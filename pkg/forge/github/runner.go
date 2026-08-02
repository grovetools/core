package github

import (
	"bytes"
	"context"
	"errors"
	"os/exec"

	"github.com/grovetools/core/pkg/forge"
)

// Runner executes a `gh` invocation. It exists so the conformance suite can
// drive the argv builder and JSON parsing without a GitHub account, and so
// callers can substitute a rate-limited or instrumented transport.
type Runner interface {
	// Run executes `gh` with args and returns stdout and stderr. A non-nil
	// error means the process failed to start or exited non-zero; stderr is
	// still returned so the caller can classify the failure.
	Run(ctx context.Context, args []string) (stdout, stderr []byte, err error)
}

// ErrGHNotFound is the cause wrapped when `gh` is not on PATH.
var ErrGHNotFound = errors.New("gh CLI not found on PATH")

// execRunner is the production transport: it shells out to `gh`.
//
// Credential handling is entirely the CLI's. This package neither reads nor
// forwards a token: it does not consult GH_TOKEN, GITHUB_TOKEN, or any other
// environment variable, and it does not modify the child environment. `gh`
// resolves its own auth exactly as it does for an interactive user, so an
// unauthenticated machine produces a clean ClassUnavailable rather than a
// silently half-working client.
type execRunner struct {
	// binary is the executable name; overridable for tests that put a fake
	// `gh` on PATH under a different name.
	binary string
}

func (r execRunner) Run(ctx context.Context, args []string) ([]byte, []byte, error) {
	bin := r.binary
	if bin == "" {
		bin = "gh"
	}
	path, err := exec.LookPath(bin)
	if err != nil {
		return nil, nil, ErrGHNotFound
	}

	cmd := exec.CommandContext(ctx, path, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// No cmd.Env assignment: the child inherits the parent environment so `gh`
	// can find its own config, and this package never injects credentials.
	err = cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

// Available reports whether the `gh` transport is usable at all. It is a
// cheap PATH probe, not an auth check: a present-but-unauthenticated `gh`
// still reports ClassUnavailable at call time.
func Available() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

// unavailable builds the ClassUnavailable error for a missing transport.
func unavailable(op string, cause error) error {
	return forge.Errorf(forge.ClassUnavailable, providerName, op, cause,
		"gh CLI is unavailable")
}
