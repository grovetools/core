package github

import (
	"context"
	"errors"
	"os/exec"
	"strings"

	"github.com/grovetools/core/pkg/forge"
)

// classify maps a failed `gh` invocation onto a forge error class.
//
// `gh` reports everything through exit status plus a human-readable stderr, so
// classification is necessarily substring matching. The ordering below is the
// contract: transport-unusable (unavailable) is checked before rate limits
// (retryable) before not-found (permanent), and anything unrecognized falls
// through to permanent — failing closed, so an unclassifiable failure cannot
// turn a poller into a hot retry loop.
func classify(op string, stderr []byte, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrGHNotFound) {
		return unavailable(op, err)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return forge.Errorf(forge.ClassRetryable, providerName, op, err, "gh call cancelled or timed out")
	}
	if errors.Is(err, exec.ErrNotFound) {
		return unavailable(op, err)
	}

	msg := strings.ToLower(string(stderr))
	trimmed := strings.TrimSpace(string(stderr))
	if trimmed == "" {
		trimmed = err.Error()
	}

	switch {
	// Transport unusable: not authenticated, or the network is not there.
	case containsAny(msg,
		"gh auth login",
		"not logged in",
		"no authentication token",
		"authentication required",
		"bad credentials",
		"requires authentication",
		"could not determine current user",
	):
		return forge.Errorf(forge.ClassUnavailable, providerName, op, err,
			"gh is not authenticated: %s", trimmed)

	case containsAny(msg,
		// This list is wider than the shapes plain `gh` emits, on purpose. The
		// pipeline-live trial (plan hosted-git-and-prs,
		// .artifacts/forge-pipeline-live/report.md finding 3) pointed the
		// provider at a dead port and got `(permanent)` in the poller cache:
		// the transport in front of gh worded its refusal differently, nothing
		// here matched, and the unclassifiable fallthrough below took it. A
		// refused, reset, unroutable or unresolvable endpoint is never a
		// permanent statement ABOUT A REPOSITORY — it says the network is
		// broken, which is what ClassUnavailable means and what makes a
		// surface render "we could not ask" instead of a verdict.
		"dial tcp",
		"no such host",
		"network is unreachable",
		"network is down",
		"connection refused",
		"econnrefused",
		"refused connection",
		"urlerror",
		"connection reset",
		"no route to host",
		"tls handshake",
		"i/o timeout",
		"connection timed out",
		"operation timed out",
		"context deadline exceeded",
		"unexpected eof",
		"proxyconnect",
		"server misbehaving",
		"temporary failure in name resolution",
	):
		return forge.Errorf(forge.ClassUnavailable, providerName, op, err,
			"github is unreachable: %s", trimmed)

	// Transient: rate limits and server errors.
	case containsAny(msg,
		"rate limit",
		"secondary rate limit",
		"was submitted too quickly",
		"abuse detection",
		"retry-after",
		"http 429",
		"http 500",
		"http 502",
		"http 503",
		"http 504",
		"server error",
	):
		return forge.Errorf(forge.ClassRetryable, providerName, op, err,
			"github rate-limited or unavailable: %s", trimmed)

	// Permanent: the thing is not there, or we may not see it.
	case containsAny(msg,
		"could not resolve to a repository",
		"could not resolve to a pullrequest",
		"http 404",
		"not found",
		"no pull requests found",
		"http 403",
		"http 422",
	):
		return forge.Errorf(forge.ClassPermanent, providerName, op, err,
			"github rejected the request: %s", trimmed)
	}

	return forge.Errorf(forge.ClassPermanent, providerName, op, err,
		"gh call failed: %s", trimmed)
}

func containsAny(haystack string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(haystack, n) {
			return true
		}
	}
	return false
}
