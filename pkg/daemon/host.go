package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/grovetools/core/logging"
	"github.com/grovetools/core/pkg/paths"
)

// HostSocketEnv names the environment variable that carries the ABSOLUTE unix
// socket path of the daemon owning the interactive host UI (treemux, or a
// tuimux host that adopts the same contract).
//
// Why a second variable instead of reusing GROVE_SCOPE: GROVE_SCOPE answers
// "which ecosystem does this work belong to?" — it is job/workspace identity
// and legitimately differs per launch (a global treemux browsing a worktree
// launches jobs whose WorkDir lives in that worktree). The host transport
// answers a different question — "which groved owns the UI that must render
// and attach this session?" — and is single-valued for the whole life of the
// host process. Overloading one variable for both made the two answers
// collide: a globally-hosted treemux launching a worktree-scoped job routed
// intent/spawn/confirm/hooks at the worktree's daemon, whose sessions the host
// never streams, so the live agent existed but could never appear in the rail
// or Agents drawer and could never be attached.
//
// The value is a socket PATH rather than a scope string on purpose: a global
// host has scope "", which is indistinguishable from "unset" once it round
// trips through an environment. paths.SocketPath("") is a concrete, non-empty
// path, so "published" and "absent" stay distinguishable.
const HostSocketEnv = "GROVE_HOST_DAEMON_SOCKET"

// PublishHostSocket records scope's daemon socket in the process environment
// as the host transport endpoint, so every descendant (shells, hosted TUIs,
// agent PTYs, hook subprocesses) can route session-lifecycle traffic back to
// the daemon that owns the UI. Returns the published path.
//
// Hosts call this once at startup, alongside pinning their scope. An empty
// scope is the global host and still publishes — the global socket path.
func PublishHostSocket(scope string) (string, error) {
	socketPath := paths.SocketPath(scope)
	if err := os.Setenv(HostSocketEnv, socketPath); err != nil {
		return "", err
	}
	return socketPath, nil
}

// HostSocketPath returns the published host daemon socket, or "" when this
// process was not launched under a host that publishes one (an ad-hoc shell,
// CI, a bare `flow plan run`). Empty means "no host transport declared" — the
// caller falls back to ordinary scope-based resolution.
func HostSocketPath() string {
	return strings.TrimSpace(os.Getenv(HostSocketEnv))
}

// withHostSocketEnv returns env with the published host transport endpoint
// appended as a KEY=VALUE entry, so a process started from this env can route
// session-lifecycle traffic back to the daemon that owns the UI.
//
// This exists because process-env publication does not survive the PTY
// boundary: panes are created BY a daemon, so they inherit the daemon's
// environ rather than the host's. Any caller that hands a command environment
// to another process on the host's behalf must pass it through here.
//
// No-ops when nothing is published (never invents an endpoint) and when env
// already carries one (an explicit caller value wins, and re-applying the
// helper is idempotent).
func withHostSocketEnv(env []string) []string {
	socketPath := HostSocketPath()
	if socketPath == "" {
		return env
	}
	prefix := HostSocketEnv + "="
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			return env
		}
	}
	return append(append([]string(nil), env...), prefix+socketPath)
}

// WithHostSocketEnv is the exported form of withHostSocketEnv, for callers
// outside this package that build a child process environment on the host's
// behalf (tuimux's direct PTY client, plugin pane launchers).
func WithHostSocketEnv(env []string) []string { return withHostSocketEnv(env) }

// ---------------------------------------------------------------------------
// UI host registry
// ---------------------------------------------------------------------------
//
// Environment publication above only reaches DESCENDANTS of the host process,
// and the processes that actually launch sessions are usually not descendants
// of anything the host owns: flow's agent providers execute inside groved's
// jobrunner (the daemon imports flow and runs jobs in-process), and panes are
// forked by tuimux daemons that may predate the host entirely. No amount of
// env stamping can reach a daemon chain the host neither spawned nor owns.
//
// The registry is the env-independent half of the contract. A host records
// itself in StateDir()/hosts/ — one small JSON file per host process — and
// ANY process (a groved running a job, a hook subprocess, a bare CLI) can
// consult it. Filesystem registration is the established federation idiom
// here: daemon discovery reads pidfile+.scope sidecars, and channels persist
// their cross-daemon routes in StateDir()/channels/state.json. It survives
// every start order, needs no parent-daemon RPC, and is sandbox-isolated for
// free because the path is XDG-derived.
//
// Staleness is handled by liveness, not protocol: entries whose PID is gone
// are skipped on lookup and pruned on the next registration; a live entry
// whose daemon socket has died is skipped by the dial check in
// sessionHostClient.
//
// The registry holds two KINDS of host (see hostKind). A "ui" host is a real
// interactive UI (treemux) — the thing this contract was built for. A "daemon"
// host is the global groved registering ITSELF as the machine-wide fallback
// endpoint. Without that second tier, "no live registered UI host" and "no host
// at all" are indistinguishable, which is exactly the fleet-restart window: kill
// every daemon while agents are running, and for the seconds treemux takes to
// come back the registry is empty, so every scope-resolving client in a worktree
// concludes "no host" and SPAWNS a scoped groved nothing will ever use. The
// global daemon is up long before its UI is, and it is where the jobs and
// sessions actually live, so letting it answer "route here" closes that window.
// A live UI host always beats a daemon host, so nothing about treemux's routing
// changes while it is running.

// hostKind classifies a registered host. It decides precedence only: a UI host
// always wins over a daemon host, because only a UI can render and attach a
// session.
type hostKind string

const (
	// hostKindUI is an interactive host UI (treemux). Also the meaning of an
	// empty Kind, so records written before this field existed keep working.
	hostKindUI hostKind = "ui"
	// hostKindDaemon is the global groved acting as its own last-resort
	// routing endpoint. It renders nothing; it exists so clients stop
	// spawning scoped daemons when no UI happens to be up.
	hostKindDaemon hostKind = "daemon"
)

// uiHostRegistration is the on-disk record of one host.
type uiHostRegistration struct {
	// PID of the host process (treemux, or the global groved for a daemon
	// host). Liveness of this pid — not of the daemon — decides whether the
	// entry is honored: for a UI host the daemon can outlive the UI, and
	// routing sessions at a UI that is gone helps nobody.
	PID int `json:"pid"`
	// Program is a human-readable tag ("treemux", "groved") for debugging.
	Program string `json:"program,omitempty"`
	// Kind is "ui" or "daemon"; empty means "ui" (pre-kind records).
	Kind hostKind `json:"kind,omitempty"`
	// Scope is the workspace subtree this host displays. "" is a global host
	// and covers every directory.
	Scope string `json:"scope"`
	// SocketPath is the groved socket the host streams sessions from.
	SocketPath string    `json:"socket_path"`
	StartedAt  time.Time `json:"started_at"`
	// Starting marks a record published BEFORE its socket can answer — a
	// daemon that has taken the pidfile and is still booting. It is what
	// distinguishes "this host is coming, wait for it" from "this host is
	// gone", which an undialable socket alone cannot say.
	//
	// Only a process that also clears the flag ever sets it, so a record from
	// a binary that predates the field reads as ready — the conservative
	// answer, since waiting on a host that will never announce itself would
	// stall every client for the full grace period.
	Starting bool `json:"starting,omitempty"`
}

// kind returns the record's kind with the pre-kind default applied.
func (r uiHostRegistration) kind() hostKind {
	if r.Kind == "" {
		return hostKindUI
	}
	return r.Kind
}

func uiHostsDir() string { return filepath.Join(paths.StateDir(), "hosts") }

func uiHostRegistrationPath(pid int) string {
	return filepath.Join(uiHostsDir(), fmt.Sprintf("host-%d.json", pid))
}

// RegisterUIHost records the calling process as the interactive host UI for
// scope ("" = global host, covers everything). The returned unregister func
// removes the record and must run when the UI exits; a crash instead leaves a
// stale file that liveness checks skip and the next registration prunes.
//
// Registration complements PublishHostSocket: the env var serves descendants
// of the host, the registry serves everyone else — most importantly the
// groved jobrunner, which executes flow's providers in a process the host
// never spawned.
func RegisterUIHost(scope, program string) (unregister func(), err error) {
	reg, err := registerHost(scope, program, hostKindUI, false)
	if err != nil {
		return nil, err
	}
	return reg.Unregister, nil
}

// RegisterDaemonHost records the calling groved as the routing endpoint of last
// resort for scope. It is the same contract as RegisterUIHost with strictly
// lower precedence: any live UI host covering the same directory wins, and this
// entry is consulted only when none does.
//
// Only the GLOBAL daemon should call this. A scoped groved registering itself
// would make its socket the most-specific host for its own subtree and could
// steal session traffic from a global treemux that is streaming a different
// daemon. The global daemon has scope "" — it covers everything, is never
// auto-shutdown, and is where in-process jobs and sessions already live — so it
// is the only safe answer to "there is no UI, now what?".
func RegisterDaemonHost(scope, program string) (unregister func(), err error) {
	reg, err := registerHost(scope, program, hostKindDaemon, false)
	if err != nil {
		return nil, err
	}
	return reg.Unregister, nil
}

// RegisterDaemonHostStarting is RegisterDaemonHost published EARLY — before the
// daemon's socket can answer — so the window between "groved is coming back" and
// "groved is reachable" stops reading as "there is no host at all".
//
// That window is the whole fleet-restart bug: a client in a worktree that finds
// no covering host legitimately auto-starts a scoped groved, and during a
// restart every client in every active worktree finds none for as long as the
// global daemon takes to boot. A record marked Starting lets those clients WAIT
// for the daemon that is demonstrably on its way (see newAutoStart's grace)
// instead of racing it.
//
// The caller MUST call MarkReady on the returned handle once its socket is
// bound, and Unregister when it exits. Registering early is only safe because
// the flag is honored as a reason to wait, never as a reason to dial: a
// Starting record whose daemon never binds costs a bounded wait, not a
// misrouted request.
func RegisterDaemonHostStarting(scope, program string) (*HostRegistration, error) {
	return registerHost(scope, program, hostKindDaemon, true)
}

// HostRegistration is a live host record, held by the process that wrote it.
// Its zero value is inert: every method no-ops on a nil receiver, so callers
// that failed to register (or never tried) need no nil checks.
type HostRegistration struct {
	mu   sync.Mutex
	path string
	reg  uiHostRegistration
}

// MarkReady clears the Starting flag, publishing that this host's socket now
// answers. Idempotent; a no-op on a record that was never marked starting.
func (h *HostRegistration) MarkReady() error {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.reg.Starting {
		return nil
	}
	h.reg.Starting = false
	return writeUIHostRegistration(h.path, h.reg)
}

// Unregister removes the record. A crash instead leaves a stale file that
// liveness checks skip and the next registration prunes.
func (h *HostRegistration) Unregister() {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	os.Remove(h.path)
}

func registerHost(scope, program string, kind hostKind, starting bool) (*HostRegistration, error) {
	dir := uiHostsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	pruneDeadUIHostRegistrations(dir)

	reg := uiHostRegistration{
		PID:        os.Getpid(),
		Program:    program,
		Kind:       kind,
		Scope:      scope,
		SocketPath: paths.SocketPath(scope),
		StartedAt:  time.Now().UTC(),
		Starting:   starting,
	}
	path := uiHostRegistrationPath(reg.PID)
	if err := writeUIHostRegistration(path, reg); err != nil {
		return nil, err
	}
	return &HostRegistration{path: path, reg: reg}, nil
}

func writeUIHostRegistration(path string, reg uiHostRegistration) error {
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil { //nolint:gosec // G306: world-readable like the pidfile beside it
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// pruneDeadUIHostRegistrations removes records whose host process is gone.
// Called on registration only, so lookups stay read-only.
func pruneDeadUIHostRegistrations(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		reg, err := readUIHostRegistration(path)
		if err != nil || !uiHostAlive(reg.PID) {
			os.Remove(path)
		}
	}
}

func readUIHostRegistration(path string) (uiHostRegistration, error) {
	var reg uiHostRegistration
	data, err := os.ReadFile(path)
	if err != nil {
		return reg, err
	}
	if err := json.Unmarshal(data, &reg); err != nil {
		return reg, err
	}
	if reg.SocketPath == "" {
		return reg, errors.New("registration missing socket_path")
	}
	return reg, nil
}

// uiHostAlive reports whether pid names a live process. EPERM still means
// alive (just not ours to signal).
func uiHostAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// uiHostScopeCovers reports whether a host registered for scope displays dir.
// A global host (scope "") covers everything, including the empty dir of an
// ad-hoc scopeless launch; a scoped host covers exactly its subtree.
func uiHostScopeCovers(scope, dir string) bool {
	if scope == "" {
		return true
	}
	if dir == "" {
		return false
	}
	scope, dir = filepath.Clean(scope), filepath.Clean(dir)
	return dir == scope || strings.HasPrefix(dir, scope+string(filepath.Separator))
}

// lookupHostSocket returns the daemon socket of the registered live host whose
// scope covers dir, or "" when no such host exists.
// See LookupHost for the precedence among competing hosts.
func lookupHostSocket(dir string) string {
	rec, ok := LookupHost(dir)
	if !ok {
		return ""
	}
	return rec.SocketPath
}

// HostRecord is the live registered host covering a directory, as reported by
// LookupHost.
type HostRecord struct {
	// SocketPath is the groved socket this host's traffic goes to.
	SocketPath string
	// PID of the host process, known live at lookup time.
	PID int
	// Program is the host's self-declared tag ("treemux", "groved").
	Program string
	// Starting is true while the host has published its record but cannot yet
	// answer on SocketPath — a daemon mid-boot. Callers wait for it; they must
	// not treat it as reachable.
	Starting bool
	// Daemon is true for a groved registered as the endpoint of last resort,
	// false for a real interactive UI host.
	Daemon bool
}

// LookupHost returns the live REGISTERED host covering dir. Unlike
// ResolveSessionHostSocket it ignores the published env endpoint, so the answer
// is a fact about the machine rather than about how this process was launched —
// which is what a daemon deciding whether to yield needs, since it inherits the
// environment of whatever spawned it.
//
// Precedence, in order:
//
//  1. Kind — a UI host always beats a daemon host. A daemon host renders
//     nothing, so it must never displace a treemux that could actually show
//     and attach the session.
//  2. Scope specificity — a worktree-scoped treemux beats a global one for
//     its own subtree.
//  3. Start time — among equals, the most recently started host wins.
func LookupHost(dir string) (HostRecord, bool) {
	hostsDir := uiHostsDir()
	entries, err := os.ReadDir(hostsDir)
	if err != nil {
		return HostRecord{}, false
	}
	var best *uiHostRegistration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		reg, err := readUIHostRegistration(filepath.Join(hostsDir, e.Name()))
		if err != nil || !uiHostAlive(reg.PID) || !uiHostScopeCovers(reg.Scope, dir) {
			continue
		}
		if best == nil || betterHost(reg, *best) {
			r := reg
			best = &r
		}
	}
	if best == nil {
		return HostRecord{}, false
	}
	return HostRecord{
		SocketPath: best.SocketPath,
		PID:        best.PID,
		Program:    best.Program,
		Starting:   best.Starting,
		Daemon:     best.kind() == hostKindDaemon,
	}, true
}

// betterHost reports whether candidate should displace incumbent as the host
// for a directory both cover. See lookupHostSocket for the ordering.
func betterHost(candidate, incumbent uiHostRegistration) bool {
	if candidate.kind() != incumbent.kind() {
		return candidate.kind() == hostKindUI
	}
	if len(candidate.Scope) != len(incumbent.Scope) {
		return len(candidate.Scope) > len(incumbent.Scope)
	}
	return candidate.StartedAt.After(incumbent.StartedAt)
}

// ResolveSessionHostSocket reports which daemon socket session-lifecycle
// traffic for dir should target, and whether that decision came from a
// declared host — the published env endpoint or a registry entry — (true) or
// from ordinary scope resolution (false).
//
// This is the dial-free half of NewSessionHostClient: it makes the routing
// decision assertable without a live daemon on either socket. It is also what
// providers use to stamp the endpoint into agent environments, so the value
// must match what the client resolution below would pick.
func ResolveSessionHostSocket(dir string) (socketPath string, viaHost bool) {
	if host := HostSocketPath(); host != "" {
		return host, true
	}
	effDir := resolveDir([]string{dir})
	if host := lookupHostSocket(effDir); host != "" {
		return host, true
	}
	_, socketPath, _ = resolveScopedTargets(effDir)
	return socketPath, false
}

// NewSessionHostClient returns the Client for session-lifecycle traffic —
// intent registration, PTY spawn/attach relay, session confirmation, hook
// status updates. These are the calls whose results must land on the daemon
// the host UI streams; everything else (workspace state, git status, plan
// queries) keeps using the ordinary scope-resolved factories.
//
// Precedence:
//
//  1. The explicit host endpoint (HostSocketEnv), when published AND
//     reachable. This is the single-valued transport answer for processes
//     that inherited it from a host.
//  2. A live registered UI host covering dir (RegisterUIHost), when its
//     daemon socket is reachable. This is how processes the host never
//     spawned — above all groved's in-process jobrunner running flow's
//     providers — find the daemon the UI streams.
//  3. Otherwise NewWithAutoStart(dir) — scope-derived behavior, which is
//     correct for launches that have no interactive host at all.
//
// A published-but-unreachable host socket deliberately does NOT auto-start a
// daemon: the endpoint names a host UI that is gone, and resurrecting its
// daemon would not resurrect the UI. Falling through to the scoped daemon
// keeps the session recorded somewhere rather than dropping it.
//
// dir is the job's scope (see flow's resolveJobScope) and is used only for
// the fallback. The caller keeps passing the real working directory as DATA
// on the request payloads — host routing must never rewrite WorkingDirectory,
// because the rail and Agents drawer filter on it.
func NewSessionHostClient(dir string) Client {
	return sessionHostClient(dir, tryConnect, func(d string) Client { return NewWithAutoStart(d) })
}

// NewSessionHostClientConnectOnly is NewSessionHostClient for callers that must
// never SPAWN a daemon: the same host precedence (env endpoint → registered UI
// host → scope resolution), but the final fallback is connect-only New(dir)
// instead of NewWithAutoStart(dir).
//
// Terminal-state writers on failure paths use this. Ending a session that no
// live daemon is tracking is a no-op worth skipping, not a reason to boot a
// daemon just to tell it about a process that already died — but when a host IS
// listening, its record must still be closed out, which plain New(dir) cannot
// guarantee because it follows GROVE_SCOPE away from the host.
func NewSessionHostClientConnectOnly(dir string) Client {
	return sessionHostClient(dir, tryConnect, func(d string) Client { return New(d) })
}

// sessionHostClient is NewSessionHostClient's policy, with its two effectful
// steps injected. Splitting them out lets the precedence be asserted without
// dialing sockets or auto-starting a real groved from a test.
func sessionHostClient(dir string, connect func(socketPath string) Client, scoped func(dir string) Client) Client {
	if host := HostSocketPath(); host != "" {
		if client := connect(host); client != nil {
			return client
		}
		logging.NewUnifiedLogger("daemon.factory").
			Debug("host daemon socket published but unreachable; trying registered hosts").
			Field("host_socket", host).
			Field("fallback_dir", dir).
			StructuredOnly().
			Log(context.Background())
	}
	if host := lookupHostSocket(resolveDir([]string{dir})); host != "" {
		if client := connect(host); client != nil {
			return client
		}
		logging.NewUnifiedLogger("daemon.factory").
			Debug("registered UI host daemon unreachable; falling back to scoped daemon").
			Field("host_socket", host).
			Field("fallback_dir", dir).
			StructuredOnly().
			Log(context.Background())
	}
	return scoped(dir)
}
