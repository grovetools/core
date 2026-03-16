package daemon

import (
	"net"
	"os"
	"os/exec"
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
	socketPath := paths.SocketPath()

	// Try to connect to existing daemon
	if client := tryConnect(socketPath); client != nil {
		return client
	}

	// Fallback: daemon not running, use local client
	return NewLocalClient()
}

// NewWithAutoStart returns a Client, attempting to auto-start the daemon if not running.
// This is the recommended factory for tools that benefit from daemon features (flow, hooks).
// If auto-start fails, it falls back to LocalClient gracefully.
func NewWithAutoStart() Client {
	socketPath := paths.SocketPath()

	// Try to connect to existing daemon
	if client := tryConnect(socketPath); client != nil {
		return client
	}

	// Daemon not running, try to auto-start it
	if autoStartDaemon() {
		// Retry connection after auto-start
		if client := tryConnectWithRetry(socketPath, 5, 100*time.Millisecond); client != nil {
			return client
		}
	}

	// Auto-start failed or daemon still not responding, use local client
	return NewLocalClient()
}

// tryConnect attempts to connect to the daemon socket.
// Returns nil if connection fails.
func tryConnect(socketPath string) Client {
	if _, err := os.Stat(socketPath); err != nil {
		return nil
	}

	conn, err := net.DialTimeout("unix", socketPath, 100*time.Millisecond)
	if err != nil {
		return nil
	}
	conn.Close()

	client, err := NewRemoteClient(socketPath)
	if err != nil {
		return nil
	}
	return client
}

// tryConnectWithRetry attempts to connect with exponential backoff.
func tryConnectWithRetry(socketPath string, maxRetries int, initialDelay time.Duration) Client {
	delay := initialDelay
	for i := 0; i < maxRetries; i++ {
		time.Sleep(delay)
		if client := tryConnect(socketPath); client != nil {
			return client
		}
		delay = delay * 2 // Exponential backoff
		if delay > time.Second {
			delay = time.Second // Cap at 1 second
		}
	}
	return nil
}

// autoStartDaemon attempts to start the daemon in the background.
// Returns true if the daemon was successfully started.
func autoStartDaemon() bool {
	// Look for groved binary
	grovedPath, err := exec.LookPath("groved")
	if err != nil {
		// Try common locations
		homeDir, _ := os.UserHomeDir()
		candidates := []string{
			homeDir + "/.grove/bin/groved",
			"/usr/local/bin/groved",
		}
		for _, path := range candidates {
			if _, err := os.Stat(path); err == nil {
				grovedPath = path
				break
			}
		}
		if grovedPath == "" {
			return false
		}
	}

	// Start daemon in background
	cmd := exec.Command(grovedPath)
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return false
	}

	// Don't wait for the process - let it run in background
	go func() {
		cmd.Wait()
	}()

	return true
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
