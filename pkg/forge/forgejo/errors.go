package forgejo

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/grovetools/core/pkg/forge"
)

// maxErrBodyChars bounds how much of an error body is echoed into a message,
// so a hostile instance cannot flood logs — and so a body that happens to
// contain a token fragment is not reproduced wholesale.
const maxErrBodyChars = 256

// classifyStatus maps a non-2xx HTTP response onto a forge error class.
//
//	401                        → unavailable (we cannot talk to this instance)
//	403 with rate-limit signal → retryable
//	403 otherwise              → permanent (we may not see this)
//	404                        → permanent
//	408, 425, 429, 5xx         → retryable
//	501                        → unsupported
//	other 4xx                  → permanent
//
// Note the split on 403: GitHub-lineage APIs return 403 for both "forbidden"
// and "rate limited", and conflating them turns a transient throttle into a
// permanent "you have no access".
func classifyStatus(op string, resp *http.Response, body []byte) error {
	snippet := strings.TrimSpace(string(body))
	if len(snippet) > maxErrBodyChars {
		snippet = snippet[:maxErrBodyChars] + "…"
	}
	status := resp.StatusCode

	class := forge.ClassPermanent
	switch {
	case status == http.StatusUnauthorized:
		class = forge.ClassUnavailable
	case status == http.StatusForbidden:
		if isRateLimited(resp, snippet) {
			class = forge.ClassRetryable
		} else {
			class = forge.ClassPermanent
		}
	case status == http.StatusNotImplemented:
		class = forge.ClassUnsupported
	case status == http.StatusRequestTimeout,
		status == http.StatusTooEarly,
		status == http.StatusTooManyRequests:
		class = forge.ClassRetryable
	case status >= 500:
		class = forge.ClassRetryable
	}

	return forge.Errorf(class, providerName, op, nil,
		"forge returned HTTP %d: %s", status, snippet)
}

// isRateLimited detects a throttle from the standard headers or the body text.
func isRateLimited(resp *http.Response, body string) bool {
	if resp.Header.Get("Retry-After") != "" {
		return true
	}
	if v := resp.Header.Get("X-RateLimit-Remaining"); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n <= 0 {
			return true
		}
	}
	lower := strings.ToLower(body)
	return strings.Contains(lower, "rate limit") || strings.Contains(lower, "too many requests")
}
