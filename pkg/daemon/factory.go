package daemon

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"sync"
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
	return newAutoStart(resolvedDir, autoStartOptions{})
}

// autoStartOptions collects the tunables for an auto-starting client factory.
// Zero value = today's behavior (paired to nothing, default boot ordering).
type autoStartOptions struct {
	pairPID    int
	earlyReady bool
}

// Option customizes NewWithAutoStartOpts.
type Option func(*autoStartOptions)

// EarlyReady makes the spawned daemon bind its socket early (--ready-at=bind)
// and stream boot progress, so the caller unblocks in milliseconds and can
// render a loading UI while the daemon finishes booting. If the spawned
// binary is too old to understand the flag it exits immediately; the factory
// detects that and respawns once without the flag before falling back to
// LocalClient. Intended for treemux's cold-start splash.
func EarlyReady() Option {
	return func(o *autoStartOptions) { o.earlyReady = true }
}

// PairWith instructs the spawned daemon to shut down when pairPID exits
// (see NewPaired). No-op when the daemon is already running.
func PairWith(pairPID int) Option {
	return func(o *autoStartOptions) { o.pairPID = pairPID }
}

// NewWithAutoStartOpts is the option-taking form of NewWithAutoStart. It exists
// so treemux can request EarlyReady() (and optionally PairWith) without adding
// a new positional factory per combination.
func NewWithAutoStartOpts(dir string, opts ...Option) Client {
	var o autoStartOptions
	for _, f := range opts {
		f(&o)
	}
	return newAutoStart(resolveDir([]string{dir}), o)
}

// NewGlobalClient returns a Client targeted at the global/unscoped daemon,
// auto-starting it if not running. The global daemon hosts the shared
// proxy (port 8443) and serves proxy RegisterProxyRoute / UnregisterProxyRoutes
// RPCs from every scoped daemon on the host. Unlike NewWithAutoStart(""),
// the daemon started here never self-terminates via --auto-shutdown because
// autoStartDaemon omits that flag when scope is empty (see autoStartDaemon).
func NewGlobalClient() Client {
	return newAutoStart("", autoStartOptions{})
}

// NewPaired works like NewWithAutoStart but instructs the spawned daemon to
// shut down when pairPID exits. See DaemonConfig.PairWithTreemux.
//
// If the daemon is already running for this scope (same socket), the existing
// daemon is returned unchanged — pairing only takes effect on a fresh spawn.
// Callers that need to guarantee pairing semantics must ensure no stale daemon
// is running for the scope before invoking NewPaired.
func NewPaired(dir string, pairPID int) Client {
	return newAutoStart(dir, autoStartOptions{pairPID: pairPID})
}

func newAutoStart(resolvedDir string, opts autoStartOptions) Client {
	scope, socketPath, pidPath := resolveScopedTargets(resolvedDir)
	clearConnectDiagnosis()

	// Try to connect to existing daemon
	client, dialErr := tryConnectDiag(socketPath)
	if client != nil {
		return client
	}

	// The socket file exists but connect() was denied (EPERM/EACCES) — the
	// sandbox signature (e.g. Claude Code's Seatbelt denies unix-socket
	// connect while os.Stat succeeds). The daemon is almost certainly alive
	// but unreachable from this process, so spawning a replacement would just
	// strand a duplicate groved on every invocation. Skip auto-start, record
	// why for callers (LastConnectDiagnosis), and fall back to LocalClient.
	// A dead daemon's stale socket (ECONNREFUSED) still takes the spawn path
	// below, unchanged.
	if isPermissionDenied(dialErr) {
		recordConnectDiagnosis(socketPath, dialErr)
		return NewLocalClient()
	}

	// Daemon not running, try to auto-start it for this scope. autoStartDaemon
	// returns the read end of a pipe whose write end is inherited by groved
	// (via --ready-fd); groved closes it after the socket is bound, giving us
	// a deterministic EOF to wait on instead of polling with a guessed window.
	// On pipe-setup failure readyPipe is nil and we fall back to plain polling.
	readyPipe, exited, ok := autoStartDaemon(scope, socketPath, pidPath, opts.pairPID, opts.earlyReady)
	if !ok {
		return NewLocalClient()
	}
	if client := waitForDaemonReady(readyPipe, socketPath, readyHandshakeTimeout); client != nil {
		return client
	}

	// One-shot flagless respawn: with EarlyReady() we passed --ready-at=bind,
	// which an older installed groved rejects as an unknown flag and exits on
	// during flag parsing (before it ever binds). That looks like an instant
	// child death, so if the child has already exited and we still couldn't
	// connect, retry the spawn once WITHOUT the flag before giving up. The
	// dead child never reached pidfile.Acquire, so there's no lock/socket
	// residue to clash with. A genuinely-slow new daemon is still running at
	// this point (exited not yet closed), so it is NOT respawned.
	if opts.earlyReady {
		select {
		case <-exited:
			readyPipe2, _, ok2 := autoStartDaemon(scope, socketPath, pidPath, opts.pairPID, false)
			if ok2 {
				if client := waitForDaemonReady(readyPipe2, socketPath, readyHandshakeTimeout); client != nil {
					return client
				}
			}
		default:
		}
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
//
// Satellite note (M2 contract C4): this factory only ever resolves LOCAL unix
// sockets. Satellite-targeted clients deliberately BYPASS the factory —
// daemon-side callers (P8 collector, P9 dispatch) construct them directly via
// NewRemoteClientWithDialer with a dialer backed by
// satellite.ConnManager.DialSatelliteSocket. Do not add satellite/registry
// resolution to New()/tryConnect(); core cannot import the daemon-internal
// satellite package, and the dial-injection seam is the intended boundary.
func tryConnect(socketPath string) Client {
	client, _ := tryConnectDiag(socketPath)
	return client
}

// tryConnectDiag is tryConnect plus a diagnosis: when the socket file EXISTS
// but the dial fails, the dial error is returned alongside the nil client so
// callers can distinguish a dead daemon (ECONNREFUSED on a stale socket) from
// one that is alive but unreachable from this process (EPERM/EACCES under the
// Claude Code sandbox). A missing socket file returns (nil, nil).
func tryConnectDiag(socketPath string) (Client, error) {
	if _, err := os.Stat(socketPath); err != nil {
		return nil, nil
	}

	conn, err := net.DialTimeout("unix", socketPath, 100*time.Millisecond)
	if err != nil {
		return nil, err
	}
	conn.Close()

	client, err := NewRemoteClient(socketPath)
	if err != nil {
		return nil, nil
	}
	return client, nil
}

// isPermissionDenied reports whether a dial error carries the sandbox
// signature: connect(2) rejected with EPERM or EACCES ("operation not
// permitted") while the socket file itself stats fine. errors.Is walks the
// *net.OpError → *os.SyscallError → syscall.Errno chain.
func isPermissionDenied(err error) bool {
	return err != nil &&
		(errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES))
}

// ConnectDiagnosis describes why the most recent auto-start factory call in
// this process fell back to LocalClient without spawning a daemon.
type ConnectDiagnosis struct {
	// SocketPath is the daemon socket the factory tried to reach.
	SocketPath string
	// Err is the dial error that aborted the connect.
	Err error
	// PermissionDenied is true when Err is EPERM/EACCES — the sandbox
	// signature (daemon likely alive but unreachable from this process).
	PermissionDenied bool
}

var (
	connectDiagMu   sync.Mutex
	lastConnectDiag *ConnectDiagnosis
)

func clearConnectDiagnosis() {
	connectDiagMu.Lock()
	defer connectDiagMu.Unlock()
	lastConnectDiag = nil
}

func recordConnectDiagnosis(socketPath string, err error) {
	connectDiagMu.Lock()
	defer connectDiagMu.Unlock()
	lastConnectDiag = &ConnectDiagnosis{
		SocketPath:       socketPath,
		Err:              err,
		PermissionDenied: isPermissionDenied(err),
	}
}

// LastConnectDiagnosis returns why the most recent NewWithAutoStart-family
// call fell back to LocalClient, or nil when no diagnosis was recorded (the
// connect succeeded, or it failed for a reason the factory handles by
// spawning). Callers that got a client with IsRunning() == false consult this
// to explain the fallback — e.g. flow telling a sandboxed user that the
// daemon is alive but connect() was denied, rather than "daemon not running".
func LastConnectDiagnosis() *ConnectDiagnosis {
	connectDiagMu.Lock()
	defer connectDiagMu.Unlock()
	return lastConnectDiag
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
func autoStartDaemon(scope, socketPath, pidPath string, pairPID int, earlyReady bool) (readyPipe *os.File, exited <-chan struct{}, ok bool) {
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
		// Try common locations: the real Grove install dir first, then
		// system-wide, then the legacy ~/.grove/bin as a last resort.
		homeDir, _ := os.UserHomeDir()
		var candidates []string
		if binDir := paths.BinDir(); binDir != "" {
			candidates = append(candidates, filepath.Join(binDir, "groved"))
		}
		candidates = append(candidates,
			"/usr/local/bin/groved",
			filepath.Join(homeDir, ".grove", "bin", "groved"), // legacy fallback
		)
		for _, path := range candidates {
			if _, err := os.Stat(path); err == nil {
				grovedPath = path
				break
			}
		}
		if grovedPath == "" {
			return nil, nil, false
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
	// EarlyReady: bind the socket before the slow boot steps so this connect
	// returns in milliseconds and treemux can render its boot-progress splash.
	// An older groved that doesn't know the flag exits during flag parsing;
	// newAutoStart detects that death and respawns once without it.
	if earlyReady {
		args = append(args, "--ready-at", "bind")
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
		return nil, nil, false
	}

	// Child now holds fd 3 (its dup of readyW); parent no longer needs its
	// copy. If we don't close it here, our end of the pipe never sees EOF
	// because our own write fd stays open.
	if readyW != nil {
		readyW.Close()
	}

	// One-line spawn notice, written to stderr directly (not the logger)
	// so it survives any logging configuration. This fires only on a real
	// spawn — connecting to an already-running daemon never reaches here.
	scopeDesc := "global"
	if scope != "" {
		scopeDesc = fmt.Sprintf("scope %q", scope)
	}
	notice := fmt.Sprintf("grove: started background daemon groved (pid %d, %s)", cmd.Process.Pid, scopeDesc)
	if scope != "" {
		// Scoped daemons are spawned with --auto-shutdown (see args above).
		notice += "; exits after 2m idle"
	}
	fmt.Fprintln(os.Stderr, notice)

	// Don't wait for the process - let it run in background. Closing exitedCh
	// on Wait() lets newAutoStart distinguish "old binary rejected the flag and
	// died" (channel closed, no connection) from "new daemon still booting"
	// (channel open) so it only respawns in the former case.
	exitedCh := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(exitedCh)
	}()

	return readyR, exitedCh, true
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
