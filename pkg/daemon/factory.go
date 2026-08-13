package daemon

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/grovetools/core/logging"
	"github.com/grovetools/core/pkg/paths"
	"github.com/grovetools/core/pkg/process"
	"github.com/grovetools/core/pkg/workspace"
)

// groveScopeEnv is the env-var override for scope resolution, used when
// the caller can't pass an explicit dir (e.g. subprocess of treemux).
const groveScopeEnv = "GROVE_SCOPE"

// GroveDaemonPairPIDEnv names the process any daemon auto-started from this
// environment must not outlive.
//
// It exists for sandboxes that spawn groved INDIRECTLY. A tend scenario never
// runs `groved start` — it runs `grove ...` inside a sandboxed HOME, and that
// grove auto-starts a daemon of its own accord. There is no call site for the
// harness to pass PairWith() to, so the intent has to travel in the
// environment alongside the HOME/XDG vars that define the sandbox.
//
// The effect is that an interrupted run cannot strand a daemon: the harness
// can be SIGKILLed, skipping every defer it owns, and the daemon still learns
// of its death from the kernel and shuts itself down. Callers that pass an
// explicit pairPID win over this variable.
const GroveDaemonPairPIDEnv = "GROVE_DAEMON_PAIR_PID"

// PairPIDEnv returns the KEY=VALUE assignment a fixture-spawning harness adds
// to a sandbox environment so that every daemon started inside it dies with
// the harness. Pair it with the HOME/XDG vars, not with the command: the whole
// point is to reach daemons the harness never invokes itself.
//
// Only ever hand it a PID that will outlive the sandbox. A process that
// EXPORTS this and then exits — a launcher that sets up a session and returns
// — publishes a pairing that either does nothing (the PID is already dead when
// the daemon starts, see PairPIDFromEnv) or kills the daemon seconds after it
// boots. Neither is what the launcher meant.
func PairPIDEnv(pid int) string {
	return fmt.Sprintf("%s=%d", GroveDaemonPairPIDEnv, pid)
}

// PairPIDFromEnv reads GROVE_DAEMON_PAIR_PID from the current environment,
// returning 0 when there is nothing usable to pair with.
//
// An unset, malformed, non-positive or already-dead value yields 0 (no
// pairing): pairing to a PID that is already gone would make the daemon shut
// down the instant it finished booting, which is a far more confusing failure
// than not pairing at all.
//
// It is exported because daemons OTHER than groved honor the same variable —
// the tuimux daemon reads it directly rather than being told by a flag, so a
// harness can pair a daemon binary that predates the flag.
func PairPIDFromEnv() int {
	raw := os.Getenv(GroveDaemonPairPIDEnv)
	if raw == "" {
		return 0
	}
	pid, err := strconv.Atoi(raw)
	if err != nil || pid <= 1 {
		return 0
	}
	if !process.IsProcessAlive(pid) {
		return 0
	}
	return pid
}

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
	return newAutoStart(resolvedDir, autoStartOptions{hostRedirect: true})
}

// autoStartOptions collects the tunables for an auto-starting client factory.
// Zero value = paired to nothing, default boot ordering, no host redirect.
type autoStartOptions struct {
	pairPID             int
	earlyReady          bool
	suppressStartNotice bool
	// hostRedirect lets the factory return a live UI-host daemon (env →
	// registry, see ResolveSessionHostSocket) instead of SPAWNING a scoped
	// daemon nothing would use. It only affects the spawn decision — a live
	// scoped daemon always wins the initial dial. Plain constructors default
	// it to true; callers that exist to spawn the scoped daemon itself
	// (EarlyReady, PairWith, RequireScopedDaemon) switch it off.
	hostRedirect bool
}

// Option customizes NewWithAutoStartOpts.
type Option func(*autoStartOptions)

// EarlyReady makes the spawned daemon bind its socket early (--ready-at=bind)
// and stream boot progress, so the caller unblocks in milliseconds and can
// render a loading UI while the daemon finishes booting. If the spawned
// binary is too old to understand the flag it exits immediately; the factory
// detects that and respawns once without the flag before falling back to
// LocalClient. Intended for treemux's cold-start splash.
// EarlyReady implies RequireScopedDaemon: the caller is booting the daemon it
// will pair its UI with, so redirecting to a host daemon would be a self-loop
// (the caller usually IS the host).
func EarlyReady() Option {
	return func(o *autoStartOptions) {
		o.earlyReady = true
		o.hostRedirect = false
	}
}

// PairWith instructs the spawned daemon to shut down when pairPID exits
// (see NewPaired). No-op when the daemon is already running. Implies
// RequireScopedDaemon: pairing semantics only exist on a fresh spawn.
func PairWith(pairPID int) Option {
	return func(o *autoStartOptions) {
		o.pairPID = pairPID
		o.hostRedirect = false
	}
}

// RequireScopedDaemon disables the UI-host redirect: even when a live host
// daemon covers the resolved scope, the factory dials/spawns the genuinely
// scoped daemon. For callers whose correctness depends on talking to the
// per-scope groved (e.g. they are about to become that scope's host).
func RequireScopedDaemon() Option {
	return func(o *autoStartOptions) { o.hostRedirect = false }
}

// SuppressStartNotice prevents an auto-start notice from being written to
// stderr. Full-screen TUI callers use this for reconnect attempts because
// out-of-band terminal output corrupts Bubble Tea's active screen. Daemon
// startup remains available through the structured daemon.factory log.
func SuppressStartNotice() Option {
	return func(o *autoStartOptions) { o.suppressStartNotice = true }
}

// NewWithAutoStartOpts is the option-taking form of NewWithAutoStart. It exists
// so treemux can request EarlyReady() (and optionally PairWith) without adding
// a new positional factory per combination.
func NewWithAutoStartOpts(dir string, opts ...Option) Client {
	o := autoStartOptions{hostRedirect: true}
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
	return newAutoStart("", autoStartOptions{hostRedirect: true})
}

// NewPaired works like NewWithAutoStart but instructs the spawned daemon to
// shut down when pairPID exits. See DaemonConfig.PairWithTreemux.
//
// If the daemon is already running for this scope (same socket), the existing
// daemon is returned unchanged — pairing only takes effect on a fresh spawn.
// Callers that need to guarantee pairing semantics must ensure no stale daemon
// is running for the scope before invoking NewPaired.
func NewPaired(dir string, pairPID int) Client {
	// hostRedirect stays false: the caller exists to spawn the daemon it is
	// pairing with, so a host redirect would defeat the pairing.
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

	// Scoped daemon absent. If a live UI host covers this scope, ride its
	// daemon instead of spawning one nothing will use (see plan perf-audit
	// job 27). Env beats registry, mirroring NewSessionHostClient; a dead or
	// undialable host falls through to the spawn path unchanged. The global
	// scope (scope == "") never redirects: the global daemon typically IS the
	// host daemon and must remain auto-startable (NewGlobalClient, proxy
	// infra). The hostSock != socketPath guard prevents a self-loop when the
	// resolved host socket is the scoped socket that just failed to dial.
	if opts.hostRedirect && scope != "" {
		if client := connectHostDaemon(resolvedDir, scope, socketPath); client != nil {
			return client
		}
	}

	// A daemon never spawns a scoped sibling, and never spawns a duplicate of
	// itself. This is the structural half of the host-redirect above: the
	// redirect needs a registered host to exist, and during a fleet restart
	// there is a window where none does yet — but a groved always knows it is a
	// groved. See MarkInProcessDaemon and inDaemonSpawnSuppressed.
	if selfSock, suppressed := inDaemonSpawnSuppressed(scope, socketPath); suppressed {
		ulog := logging.NewUnifiedLogger("daemon.factory")
		// Fall back to this daemon's own client. Jobs, sessions and PTYs this
		// process owns live here, so it is a strictly better answer than a
		// LocalClient — and than a freshly spawned daemon that knows nothing.
		if selfSock != "" && selfSock != socketPath {
			if client := tryConnect(selfSock); client != nil {
				ulog.Debug("in-daemon client: using own daemon instead of spawning a scoped sibling").
					Field("scope", scope).
					Field("requested_socket", socketPath).
					Field("self_socket", selfSock).
					StructuredOnly().
					Log(context.Background())
				return client
			}
		}
		ulog.Debug("in-daemon client: spawn suppressed, falling back to local client").
			Field("scope", scope).
			Field("requested_socket", socketPath).
			Field("self_socket", selfSock).
			StructuredOnly().
			Log(context.Background())
		return NewLocalClient()
	}

	// Someone is already starting the daemon for THIS socket: its pidfile names
	// a live process that has not bound yet. Spawning now would just fork a
	// duplicate that loses pidfile.Acquire and exits seconds later, so wait for
	// the one that is coming instead of racing it.
	if client := waitForStartingDaemon(pidPath, socketPath); client != nil {
		return client
	}

	// Daemon not running, try to auto-start it for this scope. autoStartDaemon
	// returns the read end of a pipe whose write end is inherited by groved
	// (via --ready-fd); groved closes it after the socket is bound, giving us
	// a deterministic EOF to wait on instead of polling with a guessed window.
	// On pipe-setup failure readyPipe is nil and we fall back to plain polling.
	readyPipe, exited, ok := autoStartDaemon(scope, socketPath, pidPath, opts.pairPID, opts.earlyReady, opts.suppressStartNotice)
	if !ok {
		return NewLocalClient()
	}
	if client := waitForDaemonReady(readyPipe, exited, socketPath, readyHandshakeTimeout); client != nil {
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
			readyPipe2, exited2, ok2 := autoStartDaemon(scope, socketPath, pidPath, opts.pairPID, false, opts.suppressStartNotice)
			if ok2 {
				if client := waitForDaemonReady(readyPipe2, exited2, socketPath, readyHandshakeTimeout); client != nil {
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

// ---------------------------------------------------------------------------
// Boot-window grace
// ---------------------------------------------------------------------------
//
// The host redirect above needs a REGISTERED host to exist. During a fleet
// restart there is a window where none does yet: every daemon has been killed,
// the global groved is booting, and until it finishes the hosts registry is
// empty. A client in a worktree that asks for daemon data during those seconds
// finds no host, correctly concludes "nothing is running", and spawns a scoped
// groved that nothing will ever use — one per active worktree, born exactly
// during the boot-sweep contention spike.
//
// The fix is to make "a daemon is on its way" observable and then wait for it.
// Two artifacts say so, and the code below trusts either:
//
//   - a host record marked Starting (RegisterDaemonHostStarting), written by
//     groved the moment it takes the pidfile — milliseconds into boot; and
//   - the global daemon's pidfile naming a live process whose socket does not
//     answer yet, which covers both the sliver before that record is written
//     and a groved too old to write one.
//
// The wait is bounded and only ever REPLACES a spawn: at worst a client is a
// few seconds later to the same LocalClient fallback it would have reached
// anyway, and in exchange the fleet stops accumulating stragglers.

// defaultHostBootGrace bounds how long a client will wait for a daemon that is
// demonstrably starting. Long enough to cover a global groved's bind (~2.5s
// under --ready-at=bind, and the pidfile is taken long before that), short
// enough that a wedged daemon cannot stall a CLI for meaningfully longer than
// the spawn path it replaces.
const defaultHostBootGrace = 5 * time.Second

// HostBootGraceEnv overrides defaultHostBootGrace. It exists for fixtures that
// need the window compressed (or widened) without rebuilding, and takes any
// time.ParseDuration value; "0" disables the wait entirely.
const HostBootGraceEnv = "GROVE_HOST_BOOT_GRACE"

// hostBootPollInterval is the cadence of re-checks while waiting out the boot
// window. Matches readyPollInterval: the wait ends the instant the daemon
// answers, so a dense poll is felt as a shorter wait, not as load.
const hostBootPollInterval = 100 * time.Millisecond

func hostBootGrace() time.Duration {
	if raw := strings.TrimSpace(os.Getenv(HostBootGraceEnv)); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d >= 0 {
			return d
		}
	}
	return defaultHostBootGrace
}

// connectHostDaemon returns the client of a live host daemon covering dir, or
// nil when there is none to ride and the caller should spawn its own.
//
// It waits, bounded, while a host is demonstrably still booting. A host that is
// registered and reachable returns immediately, and a host that is simply
// absent returns nil immediately — the wait only happens when there is
// something concrete to wait for.
func connectHostDaemon(dir, scope, socketPath string) Client {
	ulog := logging.NewUnifiedLogger("daemon.factory")

	grace := hostBootGrace()
	// A daemon never blocks on a booting sibling. Its own client (or a
	// LocalClient) is the answer inDaemonSpawnSuppressed is about to give, and
	// stalling in-process job code for seconds to reach the same place would be
	// pure latency.
	if _, isDaemon := inProcessDaemon(); isDaemon {
		grace = 0
	}
	deadline := time.Now().Add(grace)

	// booting is the socket of a daemon we have SEEN starting up. Once set, it
	// is a redirect target in its own right: having watched that daemon come up
	// inside our own grace window, riding it is the whole point of waiting.
	// This is what makes the wait pay off when the registry cannot answer —
	// the sliver before groved writes its record, or a groved too old to write
	// one at all.
	booting := ""

	for {
		if hostSock, viaHost := ResolveSessionHostSocket(dir); viaHost && hostSock != socketPath {
			if client := tryConnect(hostSock); client != nil {
				ulog.Debug("scoped daemon absent; using registered host daemon").
					Field("scope", scope).
					Field("host_socket", hostSock).
					Field("waited_for_boot", booting != "").
					StructuredOnly().
					Log(context.Background())
				return client
			}
		}
		if booting != "" && booting != socketPath {
			if client := tryConnect(booting); client != nil {
				ulog.Debug("scoped daemon absent; using the daemon we watched finish booting").
					Field("scope", scope).
					Field("host_socket", booting).
					StructuredOnly().
					Log(context.Background())
				return client
			}
		}
		if booting == "" {
			// Nothing is demonstrably on its way: spawn now, exactly as before.
			if booting = bootingHostSocket(dir); booting == "" {
				return nil
			}
			ulog.Debug("host daemon is booting; waiting instead of spawning a scoped daemon").
				Field("scope", scope).
				Field("host_socket", booting).
				Field("grace", grace.String()).
				StructuredOnly().
				Log(context.Background())
		}
		if !time.Now().Before(deadline) {
			ulog.Debug("host daemon did not finish booting within the grace window; spawning scoped daemon").
				Field("scope", scope).
				Field("host_socket", booting).
				Field("grace", grace.String()).
				StructuredOnly().
				Log(context.Background())
			return nil
		}
		time.Sleep(hostBootPollInterval)
	}
}

// bootingHostSocket returns the socket of a daemon that is demonstrably in the
// middle of starting up and would serve dir once it does, or "" when nothing
// is.
//
// Two artifacts count, in order of directness:
//
//   - a host record marked Starting, published by groved the moment it wins
//     pidfile.Acquire (RegisterDaemonHostStarting); and
//   - the global daemon's pidfile naming a live process whose socket does not
//     answer yet. The global daemon covers every scope, so this says the same
//     thing in the vocabulary a groved has always spoken: it covers the sliver
//     before the record is written, a registration that failed, and a binary
//     old enough not to write one.
//
// A socket that already answers is not booting — whatever it is or is not
// registered as, it is up, and the caller's registry lookup above is the right
// judge of whether to use it.
func bootingHostSocket(dir string) string {
	if rec, ok := LookupHost(dir); ok && rec.Starting {
		return rec.SocketPath
	}
	globalSock := paths.SocketPath("")
	if livePidFromFile(paths.PidFilePath("")) > 0 && tryConnect(globalSock) == nil {
		return globalSock
	}
	return ""
}

// waitForStartingDaemon waits out a daemon that already holds pidPath but has
// not bound socketPath yet, returning its client once it answers. Returns nil
// immediately when no live process holds the pidfile — the ordinary case, where
// the caller goes on to spawn.
func waitForStartingDaemon(pidPath, socketPath string) Client {
	grace := hostBootGrace()
	if grace <= 0 || pidPath == "" {
		return nil
	}
	pid := livePidFromFile(pidPath)
	if pid <= 0 || pid == os.Getpid() {
		return nil
	}
	logging.NewUnifiedLogger("daemon.factory").
		Debug("daemon for this socket is already starting; waiting instead of spawning a duplicate").
		Field("socket", socketPath).
		Field("pid", pid).
		Field("grace", grace.String()).
		StructuredOnly().
		Log(context.Background())
	return waitForDaemonReady(nil, nil, socketPath, grace)
}

// livePidFromFile returns the pid recorded in a daemon pidfile when that
// process is still alive, and 0 otherwise (missing, malformed, or stale).
func livePidFromFile(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 1 || !process.IsProcessAlive(pid) {
		return 0
	}
	return pid
}

// readyHandshakeTimeout bounds how long newAutoStart will wait for a freshly
// spawned daemon to finish binding its socket. Cold-scope boots can take
// several seconds — fsnotify watcher registration dominates — so we allow a
// generous ceiling before giving up.
const readyHandshakeTimeout = 30 * time.Second

// readyPollInterval is the cadence of connect attempts while a freshly
// spawned daemon boots. Startup retries must be fast and dense: the daemon's
// socket binds the instant boot completes, and every extra idle interval here
// is felt directly as TUI "connecting…" time.
const readyPollInterval = 100 * time.Millisecond

// exitedGracePeriod bounds how much longer we keep polling after the spawned
// child exits without us having connected. A child that lost the pidfile race
// exits immediately while the winning daemon may still be binding, so a short
// dense-retry window is kept; a genuinely failed spawn then falls back to
// LocalClient quickly instead of sitting out the full handshake timeout.
const exitedGracePeriod = 2 * time.Second

// waitForDaemonReady polls the socket with dense connect attempts until the
// spawned daemon accepts, the child dies (short grace period), or timeout
// elapses.
//
// The readiness pipe is deliberately only drained in the background, never
// awaited: the connect attempts themselves are the readiness signal. An older
// groved can leak its ready-fd write end into long-lived boot children (the
// scoped tuimux daemon), in which case pipe EOF NEVER arrives even though the
// socket bound seconds ago — blocking on the pipe turned every such boot into
// a full-timeout stall for the whole TUI.
//
// readyPipe may be nil when the caller couldn't set up a pipe; exited may be
// nil when the caller has no child-exit signal.
func waitForDaemonReady(readyPipe *os.File, exited <-chan struct{}, socketPath string, timeout time.Duration) Client {
	if readyPipe != nil {
		go func() {
			defer readyPipe.Close()
			_ = readyPipe.SetReadDeadline(time.Now().Add(timeout))
			buf := make([]byte, 1)
			_, _ = readyPipe.Read(buf)
		}()
	}

	deadline := time.Now().Add(timeout)
	for {
		if client := tryConnect(socketPath); client != nil {
			return client
		}
		if time.Now().After(deadline) {
			return nil
		}
		select {
		case <-exited:
			// Child is gone; clamp the remaining wait to a short grace window
			// for a racing sibling daemon that may still be binding.
			graceDeadline := time.Now().Add(exitedGracePeriod)
			if graceDeadline.Before(deadline) {
				deadline = graceDeadline
			}
			exited = nil
		case <-time.After(readyPollInterval):
		}
	}
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

// SocketReachable reports whether a daemon is accepting connections on
// socketPath right now. It is the dial half of LookupHost, exported for callers
// that must not act on a registration alone — above all a scoped daemon
// deciding whether to yield to a host, which must never exit in favor of a
// socket that answers nobody.
func SocketReachable(socketPath string) bool {
	return tryConnect(socketPath) != nil
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
func autoStartDaemon(scope, socketPath, pidPath string, pairPID int, earlyReady, suppressStartNotice bool) (readyPipe *os.File, exited <-chan struct{}, ok bool) {
	// View with: core logs --component daemon.factory -f
	ulog := logging.NewUnifiedLogger("daemon.factory")
	ulog.Debug("daemon auto-start").
		Field("scope", scope).
		Field("socket", socketPath).
		Field("pidfile", pidPath).
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
	// An explicit PairWith() beats the environment: the caller knows which
	// process it is pairing to, the env var only says "whatever spawns here
	// belongs to this run".
	if pairPID <= 0 {
		pairPID = PairPIDFromEnv()
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
	if !suppressStartNotice {
		fmt.Fprintln(os.Stderr, notice)
	}

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
