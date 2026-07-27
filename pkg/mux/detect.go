package mux

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"sync"

	"github.com/grovetools/tuimux"
)

var (
	detectOnce   sync.Once
	cachedEngine MuxEngine
	cachedErr    error
)

// Constructor seams. Production always uses the real constructors; tests
// replace these to exercise failure paths without starting a multiplexer.
var (
	tuimuxConstructor = NewTuimuxEngine
	tmuxConstructor   = NewTmuxEngine
)

// IsNilEngine reports whether a MuxEngine interface value is unusable — either a
// plain nil interface or an interface holding a nil concrete pointer.
//
// Constructors here return concrete pointers (*TuimuxEngine, *TmuxEngine).
// Returning such a result straight into a MuxEngine-typed return wraps a nil
// pointer in a non-nil interface, so a plain `engine != nil` guard passes and
// the next method call panics on a nil receiver. Callers that cannot prove
// their engine came from a normalized constructor should guard with this.
func IsNilEngine(engine MuxEngine) bool {
	if engine == nil {
		return true
	}
	v := reflect.ValueOf(engine)
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan, reflect.UnsafePointer:
		return v.IsNil()
	default:
		return false
	}
}

// normalizeEngine enforces the package invariant for every constructor that is
// forwarded into a MuxEngine-typed return:
//
//	err != nil  =>  returned MuxEngine is nil
//	err == nil  =>  returned MuxEngine is non-nil and safe to call
//
// Call it as normalizeEngine(tuimuxConstructor()) so the typed-nil never escapes.
func normalizeEngine(engine MuxEngine, err error) (MuxEngine, error) {
	if err != nil {
		return nil, err
	}
	if IsNilEngine(engine) {
		return nil, fmt.Errorf("mux: engine constructor returned a nil engine without an error")
	}
	return engine, nil
}

// DetectMuxEngine returns a MuxEngine based on GROVE_MUX env var or auto-detection.
// The result is cached for the lifetime of the process.
//
// A non-nil error always comes with a nil engine.
func DetectMuxEngine(ctx context.Context) (MuxEngine, error) {
	detectOnce.Do(func() {
		cachedEngine, cachedErr = detectMuxEngine()
	})
	return cachedEngine, cachedErr
}

func detectMuxEngine() (MuxEngine, error) {
	switch os.Getenv(EnvGroveMux) {
	case "tuimux":
		return normalizeEngine(tuimuxConstructor())
	case "tmux":
		return normalizeEngine(tmuxConstructor())
	}

	// If GROVE_TMUX_SOCKET is set, we're in an isolated tmux environment (tend harness).
	// Use tmux directly — don't auto-detect tuimux.
	if os.Getenv(EnvGroveTmuxSocket) != "" {
		return normalizeEngine(tmuxConstructor())
	}

	// Respect the active mux environment.
	switch ActiveMux() {
	case MuxTuimux:
		return normalizeEngine(tuimuxConstructor())
	case MuxTmux:
		return normalizeEngine(tmuxConstructor())
	}

	// Not inside either mux — auto-detect by pinging tuimux daemon.
	client := tuimux.NewApiClient(GetTuimuxSocketPath())
	if err := client.Ping(); err == nil {
		return normalizeEngine(tuimuxConstructor())
	}

	return normalizeEngine(tmuxConstructor())
}

// DetectExistingMuxEngine is DetectMuxEngine restricted to multiplexers that
// are already running. NewTuimuxEngine starts a daemon when none is reachable,
// which is right for launching work and wrong for best-effort teardown: a
// caller that only wants to close a window it may not even own should not
// bring up a multiplexer to do it. Cleanup paths use this instead.
//
// It is not cached: reachability changes over a process's lifetime, and the
// check is a single socket ping.
//
// A non-nil error always comes with a nil engine.
func DetectExistingMuxEngine(ctx context.Context) (MuxEngine, error) {
	// tmux's constructor only resolves the binary; it never starts a server,
	// so it needs no reachability guard.
	tmux := func() (MuxEngine, error) { return normalizeEngine(tmuxConstructor()) }

	// Selection order mirrors detectMuxEngine; only the tuimux branch differs.
	switch os.Getenv(EnvGroveMux) {
	case "tuimux":
		return runningTuimuxEngine()
	case "tmux":
		return tmux()
	}
	if os.Getenv(EnvGroveTmuxSocket) != "" {
		return tmux()
	}
	switch ActiveMux() {
	case MuxTuimux:
		return runningTuimuxEngine()
	case MuxTmux:
		return tmux()
	}
	if engine, err := runningTuimuxEngine(); err == nil {
		return engine, nil
	}
	return tmux()
}

// runningTuimuxEngine connects to an already-running tuimux daemon, and fails
// rather than starting one.
func runningTuimuxEngine() (MuxEngine, error) {
	socketPath := GetTuimuxSocketPath()
	if err := PingTuimuxSocket(socketPath); err != nil {
		return nil, fmt.Errorf("tuimux daemon not running at %s: %w", socketPath, err)
	}
	return normalizeEngine(NewTuimuxEngineWithSocket(socketPath))
}

// GetEngine returns a specific mux engine by name, bypassing cached auto-detection.
// Use this in daemon-side code where the caller knows the target mux from the
// submission path (agent_target field).
//
// A non-nil error always comes with a nil engine.
func GetEngine(name string) (MuxEngine, error) {
	switch name {
	case "tuimux":
		return normalizeEngine(tuimuxConstructor())
	case "tmux":
		return normalizeEngine(tmuxConstructor())
	default:
		return DetectMuxEngine(context.Background())
	}
}

// IsAvailable returns true if any mux engine can be detected.
func IsAvailable(ctx context.Context) bool {
	engine, err := DetectMuxEngine(ctx)
	return err == nil && !IsNilEngine(engine)
}

// ResetDetection resets the cached engine detection. Intended for testing only.
func ResetDetection() {
	detectOnce = sync.Once{}
	cachedEngine = nil
	cachedErr = nil
}

// ErrNotImplemented is returned by TUI methods that are not yet supported.
var ErrNotImplemented = fmt.Errorf("not implemented for this mux engine")
