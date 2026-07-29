package logging

import (
	"io"
	"os"
	"sync"
)

// globalWriter is an io.Writer that delegates to an underlying writer,
// which can be swapped at runtime in a thread-safe manner.
type globalWriter struct {
	mu sync.RWMutex
	w  io.Writer
}

// Write implements the io.Writer interface.
func (gw *globalWriter) Write(p []byte) (n int, err error) {
	gw.mu.RLock()
	defer gw.mu.RUnlock()
	return gw.w.Write(p)
}

// Set changes the underlying writer.
func (gw *globalWriter) Set(w io.Writer) {
	gw.mu.Lock()
	defer gw.mu.Unlock()
	gw.w = w
}

func (gw *globalWriter) Swap(w io.Writer) io.Writer {
	gw.mu.Lock()
	defer gw.mu.Unlock()
	previous := gw.w
	gw.w = w
	return previous
}

// Ensure the global writer is a singleton.
var defaultGlobalWriter = &globalWriter{w: os.Stdout}

// SetGlobalOutput sets the output destination for all loggers that use it.
// This is the central function for redirecting logs in the TUI.
func SetGlobalOutput(w io.Writer) {
	defaultGlobalWriter.Set(w)
}

// SwapGlobalOutput changes the global output destination and returns the
// previous destination so callers can restore scoped redirections.
func SwapGlobalOutput(w io.Writer) io.Writer {
	return defaultGlobalWriter.Swap(w)
}

// GetGlobalOutput returns the singleton instance of the global writer.
// All logging packages should be initialized with this writer.
func GetGlobalOutput() io.Writer {
	return defaultGlobalWriter
}
