package daemon

import (
	"net"
	"os"
	"time"

	"github.com/grovetools/core/pkg/paths"
)

// New returns a Client that will use the daemon if available,
// otherwise falls back to LocalClient.
//
// This implements the "transparent daemon" pattern: callers don't need
// to know whether the daemon is running or not. The same API works
// in both modes.
func New() Client {
	// Check if socket exists and we can connect
	socketPath := paths.SocketPath()
	if _, err := os.Stat(socketPath); err == nil {
		// Socket file exists, try to connect
		conn, err := net.DialTimeout("unix", socketPath, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			// Return RemoteClient when daemon is available
			if client, err := NewRemoteClient(socketPath); err == nil {
				return client
			}
		}
	}

	// Fallback: daemon not running, use local client
	return NewLocalClient()
}

// MustConnect returns a DaemonClient or panics if the daemon is not available.
// Use this in contexts where the daemon is required (e.g., daemon-only tools).
func MustConnect() Client {
	client := New()
	if !client.IsRunning() {
		panic("grove daemon is not running; start it with 'grove daemon start'")
	}
	return client
}

// WithFallback wraps a Client to provide graceful degradation.
// If the primary client fails, it falls back to LocalClient.
type WithFallback struct {
	Primary  Client
	Fallback Client
}

// NewWithFallback creates a client that tries the daemon first,
// then falls back to local execution.
func NewWithFallback() *WithFallback {
	return &WithFallback{
		Primary:  New(),
		Fallback: NewLocalClient(),
	}
}
