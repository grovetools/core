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

	// Auto-detect: try tuimux daemon first, fall back to tmux.
	client := tuimux.NewApiClient(GetTuimuxSocketPath())
	if err := client.Ping(); err == nil {
		return NewTuimuxEngine()
	}

	return NewTmuxEngine()
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
