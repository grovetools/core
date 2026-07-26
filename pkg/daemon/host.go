package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// uiHostRegistration is the on-disk record of one interactive host UI.
type uiHostRegistration struct {
	// PID of the host UI process (treemux). Liveness of this pid — not of
	// the daemon — decides whether the entry is honored: the daemon can
	// outlive the UI, and routing sessions at a UI that is gone helps nobody.
	PID int `json:"pid"`
	// Program is a human-readable tag ("treemux") for debugging.
	Program string `json:"program,omitempty"`
	// Scope is the workspace subtree this host displays. "" is a global host
	// and covers every directory.
	Scope string `json:"scope"`
	// SocketPath is the groved socket the host streams sessions from.
	SocketPath string    `json:"socket_path"`
	StartedAt  time.Time `json:"started_at"`
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
	dir := uiHostsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	pruneDeadUIHostRegistrations(dir)

	reg := uiHostRegistration{
		PID:        os.Getpid(),
		Program:    program,
		Scope:      scope,
		SocketPath: paths.SocketPath(scope),
		StartedAt:  time.Now().UTC(),
	}
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return nil, err
	}
	path := uiHostRegistrationPath(reg.PID)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return nil, err
	}
	return func() { os.Remove(path) }, nil
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

// lookupUIHostSocket returns the daemon socket of the registered live host
// whose scope covers dir, or "" when no such host exists. The most specific
// scope wins (a worktree-scoped treemux beats a global one for its subtree);
// among equals, the most recently started host wins.
func lookupUIHostSocket(dir string) string {
	hostsDir := uiHostsDir()
	entries, err := os.ReadDir(hostsDir)
	if err != nil {
		return ""
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
		if best == nil || len(reg.Scope) > len(best.Scope) ||
			(len(reg.Scope) == len(best.Scope) && reg.StartedAt.After(best.StartedAt)) {
			r := reg
			best = &r
		}
	}
	if best == nil {
		return ""
	}
	return best.SocketPath
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
	if host := lookupUIHostSocket(effDir); host != "" {
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
	if host := lookupUIHostSocket(resolveDir([]string{dir})); host != "" {
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
