package mux

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/grovetools/tuimux"
)

var (
	detectOnce   sync.Once
	cachedEngine MuxEngine
	cachedErr    error
)

// DetectMuxEngine returns a MuxEngine based on GROVE_MUX env var or auto-detection.
// The result is cached for the lifetime of the process.
func DetectMuxEngine(ctx context.Context) (MuxEngine, error) {
	detectOnce.Do(func() {
		cachedEngine, cachedErr = detectMuxEngine()
	})
	return cachedEngine, cachedErr
}

func detectMuxEngine() (MuxEngine, error) {
	switch os.Getenv(EnvGroveMux) {
	case "tuimux":
		return NewTuimuxEngine()
	case "tmux":
		return NewTmuxEngine()
	}

	// If GROVE_TMUX_SOCKET is set, we're in an isolated tmux environment (tend harness).
	// Use tmux directly — don't auto-detect tuimux.
	if os.Getenv(EnvGroveTmuxSocket) != "" {
		return NewTmuxEngine()
	}

	// Respect the active mux environment.
	switch ActiveMux() {
	case MuxTuimux:
		return NewTuimuxEngine()
	case MuxTmux:
		return NewTmuxEngine()
	}

	// Not inside either mux — auto-detect by pinging tuimux daemon.
	client := tuimux.NewApiClient(GetTuimuxSocketPath())
	if err := client.Ping(); err == nil {
		return NewTuimuxEngine()
	}

	return NewTmuxEngine()
}

// GetEngine returns a specific mux engine by name, bypassing cached auto-detection.
// Use this in daemon-side code where the caller knows the target mux from the
// submission path (agent_target field).
func GetEngine(name string) (MuxEngine, error) {
	switch name {
	case "tuimux":
		return NewTuimuxEngine()
	case "tmux":
		return NewTmuxEngine()
	default:
		return DetectMuxEngine(context.Background())
	}
}

// IsAvailable returns true if any mux engine can be detected.
func IsAvailable(ctx context.Context) bool {
	engine, err := DetectMuxEngine(ctx)
	return err == nil && engine != nil
}

// ResetDetection resets the cached engine detection. Intended for testing only.
func ResetDetection() {
	detectOnce = sync.Once{}
	cachedEngine = nil
	cachedErr = nil
}

// ErrNotImplemented is returned by TUI methods that are not yet supported.
var ErrNotImplemented = fmt.Errorf("not implemented for this mux engine")
