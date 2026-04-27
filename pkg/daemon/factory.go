package daemon

import (
	"context"
	"net"
	"os"
	"os/exec"
	"runtime/debug"
	"strconv"
	"syscall"
	"time"

	"github.com/grovetools/core/logging"
	"github.com/grovetools/core/pkg/paths"
	"github.com/grovetools/core/pkg/workspace"
)

// groveScopeEnv is the env-var override for scope resolution, used when
// the caller can't pass an explicit dir (e.g. subprocess of treemux).
const groveScopeEnv = "GROVE_SCOPE"

// resolveDir picks the input directory for scope resolution.
//
// Order: explicit arg > GROVE_SCOPE env > empty.
//
// We intentionally do NOT fall through to os.Getwd(). Clients that run
// in arbitrary directories (ad-hoc CLI invocations from random shells,
// hook subprocesses, etc) should default to the global/unscoped daemon
// rather than spawning per-cwd daemons keyed to wherever they happened
// to launch. To opt in to a scoped daemon, callers must either pass a
// dir explicitly or export GROVE_SCOPE — and the only places that do
// so are the explicit scope-aware boundaries (treemux startup, flow
// agent launchers).
func resolveDir(dirs []string) string {
	if len(dirs) > 0 && dirs[0] != "" {
		return dirs[0]
	}
	return os.Getenv(groveScopeEnv)
}

// ResolveClientScope returns the effective scope a daemon client would
// use right now — applying the same precedence as New(): explicit arg
// > GROVE_SCOPE env > empty (global). Exposed for direct-socket
// callers (treemux's WebSocket connect, inspector panel) that bypass
// the Client abstraction but still need the scoped socket path. Empty
// return means "use the global/unscoped socket."
func ResolveClientScope() string {
	dir := resolveDir(nil)
	if dir == "" {
		return ""
	}
	return workspace.ResolveScope(dir)
}

// resolveScopedTargets returns the scope, socket path, and pidfile path for
// the given caller directory.
//
// Empty dir means "no scope intended" — resolves to the global/unscoped
// socket. We do NOT call workspace.ResolveScope("") here, because that
// function falls back to os.Getwd() when given empty input, which would
// reintroduce the very cwd inference we removed from resolveDir.
func resolveScopedTargets(dir string) (scope, socketPath, pidPath string) {
	ulog := logging.NewUnifiedLogger("daemon.factory")
	if dir != "" {
		scope = workspace.ResolveScope(dir)
	}
	socketPath = paths.SocketPath(scope)
	pidPath = paths.PidFilePath(scope)
	ulog.Debug("resolved daemon scope").
		Field("scope", scope).
		Field("socket", socketPath).
		Field("input_dir", dir).
		StructuredOnly().
		Log(context.Background())
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
	return newAutoStart(resolvedDir, 0)
}

// NewGlobalClient returns a Client targeted at the global/unscoped daemon,
// auto-starting it if not running. The global daemon hosts the shared
// proxy (port 8443) and serves proxy RegisterProxyRoute / UnregisterProxyRoutes
// RPCs from every scoped daemon on the host. Unlike NewWithAutoStart(""),
// the daemon started here never self-terminates via --auto-shutdown because
// autoStartDaemon omits that flag when scope is empty (see autoStartDaemon).
func NewGlobalClient() Client {
	return newAutoStart("", 0)
}

// NewPaired works like NewWithAutoStart but instructs the spawned daemon to
// shut down when pairPID exits. See DaemonConfig.PairWithTreemux.
//
// If the daemon is already running for this scope (same socket), the existing
// daemon is returned unchanged — pairing only takes effect on a fresh spawn.
// Callers that need to guarantee pairing semantics must ensure no stale daemon
// is running for the scope before invoking NewPaired.
func NewPaired(dir string, pairPID int) Client {
	return newAutoStart(dir, pairPID)
}

func newAutoStart(resolvedDir string, pairPID int) Client {
	scope, socketPath, pidPath := resolveScopedTargets(resolvedDir)

	// Try to connect to existing daemon
	if client := tryConnect(socketPath); client != nil {
		return client
	}

	// Daemon not running, try to auto-start it for this scope. autoStartDaemon
	// returns the read end of a pipe whose write end is inherited by groved
	// (via --ready-fd); groved closes it after the socket is bound, giving us
	// a deterministic EOF to wait on instead of polling with a guessed window.
	// On pipe-setup failure readyPipe is nil and we fall back to plain polling.
	readyPipe, ok := autoStartDaemon(scope, socketPath, pidPath, pairPID)
	if !ok {
		return NewLocalClient()
	}
	if client := waitForDaemonReady(readyPipe, socketPath, readyHandshakeTimeout); client != nil {
		return client
	}

	// Auto-start succeeded but daemon never signaled ready (or the short
	// connect cushion that follows still didn't land us a RemoteClient).
	// Fall back to LocalClient rather than blocking the caller indefinitely.
	return NewLocalClient()
}

// readyHandshakeTimeout bounds how long newAutoStart will wait for a freshly
// spawned daemon to finish binding its socket. Cold-scope boots can take
// several seconds — fsnotify watcher registration dominates — so we allow a
// generous ceiling before giving up.
const readyHandshakeTimeout = 30 * time.Second

// waitForDaemonReady blocks until the spawned daemon signals readiness by
// closing its end of readyPipe (EOF on Read), or timeout elapses. In either
// case, a short connect cushion runs afterward to absorb the microseconds
// between OnReady firing and Serve actually accepting.
//
// readyPipe may be nil when the caller couldn't set up a pipe; in that case
// the function falls through to the cushion directly.
func waitForDaemonReady(readyPipe *os.File, socketPath string, timeout time.Duration) Client {
	if readyPipe != nil {
		defer readyPipe.Close()
		_ = readyPipe.SetReadDeadline(time.Now().Add(timeout))
		buf := make([]byte, 1)
		_, _ = readyPipe.Read(buf)
	}
	return tryConnectWithRetry(socketPath, 5, 50*time.Millisecond)
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
// given scope. Returns the read end of a readiness pipe whose write end is
// inherited by the child as fd 3, and a bool indicating whether the spawn
// succeeded. The caller reads readyPipe to block until groved signals it has
// bound the socket (groved closes fd 3 via its --ready-fd=3 flag).
//
// readyPipe is non-nil only when both os.Pipe() succeeded and cmd.Start()
// succeeded; on any error path the pipe is torn down and readyPipe is nil
// (caller falls back to plain retry-based polling).
//
// Spawns groved with explicit --scope/--socket/--pidfile/--auto-shutdown
// so the auto-started daemon binds the scope-keyed paths and exits on
// idle. Empty scope falls through to groved's own unscoped defaults. When
// pairPID > 0, --pair-with-pid is added so the daemon exits when that
// parent process dies.
func autoStartDaemon(scope, socketPath, pidPath string, pairPID int) (*os.File, bool) {
	// Diagnostic: log the caller stack so we can trace which tool is
	// triggering a scoped-daemon auto-spawn. View with:
	//   core logs --component daemon.factory -f
	// Temporary — remove once the "unexpected scoped daemon on treemux
	// start" investigation concludes.
	ulog := logging.NewUnifiedLogger("daemon.factory")
	ulog.Debug("daemon auto-start").
		Field("scope", scope).
		Field("socket", socketPath).
		Field("pidfile", pidPath).
		Field("stack", string(debug.Stack())).
		StructuredOnly().
		Log(context.Background())

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
			return nil, false
		}
	}

	// Start daemon in background, detached into its own session so it survives
	// the parent terminal's exit. Without Setsid, groved shares the terminal's
	// process group and receives SIGHUP when the terminal closes, which triggers
	// ptyManager.Shutdown() and kills every agent PTY the daemon owns.
	//
	// Auto-shutdown is only enabled for scoped daemons. The global (unscoped)
	// daemon hosts the shared *.grove.local proxy on :8443 and the host-wide
	// route table; if it self-terminates on idle, every scoped daemon's routing
	// silently breaks. Scoped daemons stay self-reaping as before.
	args := []string{"start"}
	if scope != "" {
		args = append(args, "--auto-shutdown")
		args = append(args, "--scope", scope)
	}
	if socketPath != "" {
		args = append(args, "--socket", socketPath)
	}
	if pidPath != "" {
		args = append(args, "--pidfile", pidPath)
	}
	if pairPID > 0 {
		args = append(args, "--pair-with-pid", strconv.Itoa(pairPID))
	}

	// Readiness pipe: the write end is inherited by the child as fd 3
	// (ExtraFiles[0]). groved closes fd 3 after binding its unix socket, which
	// becomes EOF on our read end. Pipe setup failure is non-fatal — we drop
	// back to the old retry-based polling path by returning a nil readyPipe.
	readyR, readyW, pipeErr := os.Pipe()
	if pipeErr == nil {
		args = append(args, "--ready-fd", "3")
	}
	cmd := exec.Command(grovedPath, args...)
	if pipeErr == nil {
		cmd.ExtraFiles = []*os.File{readyW}
	}
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		if readyR != nil {
			readyR.Close()
		}
		if readyW != nil {
			readyW.Close()
		}
		return nil, false
	}

	// Child now holds fd 3 (its dup of readyW); parent no longer needs its
	// copy. If we don't close it here, our end of the pipe never sees EOF
	// because our own write fd stays open.
	if readyW != nil {
		readyW.Close()
	}

	// Don't wait for the process - let it run in background
	go func() {
		_ = cmd.Wait()
	}()

	return readyR, true
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
