package daemon

import (
	"net"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/grovetools/core/pkg/paths"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/sirupsen/logrus"
)

// groveScopeEnv is the env-var override for scope resolution, used when
// the caller can't pass an explicit dir (e.g. subprocess of treemux).
const groveScopeEnv = "GROVE_SCOPE"

// resolveDir picks the input directory for scope resolution.
// Order: explicit arg > GROVE_SCOPE env > os.Getwd().
func resolveDir(dirs []string) string {
	if len(dirs) > 0 && dirs[0] != "" {
		return dirs[0]
	}
	if scope := os.Getenv(groveScopeEnv); scope != "" {
		return scope
	}
	cwd, _ := os.Getwd()
	return cwd
}

// resolveScopedTargets returns the scope, socket path, and pidfile path for
// the given caller directory, logging the decision at INFO so mis-routing
// is visible in logs.
func resolveScopedTargets(dir string) (scope, socketPath, pidPath string) {
	scope = workspace.ResolveScope(dir)
	socketPath = paths.SocketPath(scope)
	pidPath = paths.PidFilePath(scope)
	logrus.Debugf("daemon client: scope=%s socket=%s", scope, socketPath)
	return scope, socketPath, pidPath
}

// New returns a Client that will use the daemon if available,
// otherwise falls back to LocalClient.
//
// With no argument, the scope is resolved from GROVE_SCOPE env var or the
// current working directory. Pass an explicit dir when the caller cannot
// rely on cwd (e.g. operating on a specific plan directory).
//
// This implements the "transparent daemon" pattern: callers don't need
// to know whether the daemon is running or not. The same API works
// in both modes.
func New(dir ...string) Client {
	resolvedDir := resolveDir(dir)
	_, socketPath, _ := resolveScopedTargets(resolvedDir)

	// Try to connect to existing scoped daemon
	if client := tryConnect(socketPath); client != nil {
		return client
	}

	// Fallback: daemon not running, use local client.
	// Intentionally no global-socket fallback: one scope → one socket,
	// keeping the "which daemon am I talking to?" question unambiguous.
	return NewLocalClient()
}

// NewWithAutoStart returns a Client, attempting to auto-start the daemon if not running.
// This is the recommended factory for tools that benefit from daemon features (flow, hooks).
// If auto-start fails, it falls back to LocalClient gracefully.
func NewWithAutoStart(dir ...string) Client {
	resolvedDir := resolveDir(dir)
	scope, socketPath, pidPath := resolveScopedTargets(resolvedDir)

	// Try to connect to existing daemon
	if client := tryConnect(socketPath); client != nil {
		return client
	}

	// Daemon not running, try to auto-start it for this scope
	if autoStartDaemon(scope, socketPath, pidPath) {
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

// autoStartDaemon attempts to start the daemon in the background for the
// given scope. Returns true if the daemon was successfully started.
//
// NOTE (P1): the --scope/--socket/--pidfile/--auto-shutdown flags are added
// to `groved start` in P2. Until P2 lands, we spawn the daemon WITHOUT those
// flags and accept that the auto-started daemon will bind the global
// (unscoped) socket. The scope/socket/pidPath args are recorded here so
// P2's agent can flip a single line and get the scoped dispatch path.
func autoStartDaemon(scope, socketPath, pidPath string) bool {
	_ = scope
	_ = socketPath
	_ = pidPath

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

	// Start daemon in background, detached into its own session so it survives
	// the parent terminal's exit. Without Setsid, groved shares the terminal's
	// process group and receives SIGHUP when the terminal closes, which triggers
	// ptyManager.Shutdown() and kills every agent PTY the daemon owns.
	cmd := exec.Command(grovedPath)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

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
func MustConnect(dir ...string) Client {
	client := New(dir...)
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
func NewWithFallback(dir ...string) *WithFallback {
	return &WithFallback{
		Primary:  New(dir...),
		Fallback: NewLocalClient(),
	}
}
