package daemon

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// recordingDaemon is a stub groved: a unix-socket HTTP server that records the
// session-lifecycle requests it receives. Two of them stand in for the global
// (host) daemon and a worktree-scoped daemon.
type recordingDaemon struct {
	socketPath string
	intents    []SessionIntent
	confirms   []SessionConfirmation
	spawns     []SpawnAgentRequest
}

func newRecordingDaemon(t *testing.T, name string) *recordingDaemon {
	t.Helper()
	// macOS caps sun_path, so keep the socket path short (mirrors
	// shortTempSocket in remote_test.go).
	dir, err := os.MkdirTemp("/tmp", "hostrt")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	d := &recordingDaemon{socketPath: filepath.Join(dir, name)}
	ln, err := net.Listen("unix", d.socketPath)
	if err != nil {
		t.Fatalf("listen unix %s: %v", d.socketPath, err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/sessions/intent", func(w http.ResponseWriter, r *http.Request) {
		var intent SessionIntent
		_ = json.NewDecoder(r.Body).Decode(&intent)
		d.intents = append(d.intents, intent)
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/sessions/confirm", func(w http.ResponseWriter, r *http.Request) {
		var confirm SessionConfirmation
		_ = json.NewDecoder(r.Body).Decode(&confirm)
		d.confirms = append(d.confirms, confirm)
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/agents/spawn", func(w http.ResponseWriter, r *http.Request) {
		var spawn SpawnAgentRequest
		_ = json.NewDecoder(r.Body).Decode(&spawn)
		d.spawns = append(d.spawns, spawn)
		w.WriteHeader(http.StatusOK)
	})
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close(); ln.Close() })
	return d
}

// isolateHostEnv clears both routing variables and pins the state dir under a
// temp GROVE_HOME, so neither a developer's ambient treemux session (env) nor
// their live UI-host registry (StateDir()/hosts) can leak into these tests.
func isolateHostEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GROVE_HOME", t.TempDir())
	t.Setenv(HostSocketEnv, "")
	_ = os.Unsetenv(HostSocketEnv)
	t.Setenv(groveScopeEnv, "")
	_ = os.Unsetenv(groveScopeEnv)
}

// TestSessionHostRoutingPrefersPublishedHost is the dual-socket regression: a
// GLOBAL host publishes its socket, the job's scope resolves to a SCOPED
// worktree socket, and every session-lifecycle call must land on the host
// socket while the scoped daemon receives nothing. This is the exact topology
// that made a live native agent invisible in treemux.
func TestSessionHostRoutingPrefersPublishedHost(t *testing.T) {
	isolateHostEnv(t)
	host := newRecordingDaemon(t, "global.sock")
	scoped := newRecordingDaemon(t, "scoped.sock")

	t.Setenv(HostSocketEnv, host.socketPath)

	client := NewSessionHostClient("/some/worktree/scope")
	defer client.Close()

	rc, ok := client.(*RemoteClient)
	if !ok {
		t.Fatalf("NewSessionHostClient returned %T, want *RemoteClient", client)
	}
	if rc.SocketPath() != host.socketPath {
		t.Fatalf("client socket = %q, want host socket %q", rc.SocketPath(), host.socketPath)
	}

	ctx := context.Background()
	// The working directory stays the real worktree — host routing must move
	// the TRANSPORT, never rewrite the session's identity data, because the
	// rail and Agents drawer filter on WorkingDirectory.
	const workDir = "/some/worktree/scope/markdown-toc"
	if err := client.RegisterSessionIntent(ctx, SessionIntent{JobID: "job-1", WorkDir: workDir}); err != nil {
		t.Fatalf("RegisterSessionIntent: %v", err)
	}
	if err := client.SpawnAgentPane(ctx, SpawnAgentRequest{JobID: "job-1", WorkDir: workDir}); err != nil {
		t.Fatalf("SpawnAgentPane: %v", err)
	}
	if err := client.ConfirmSession(ctx, SessionConfirmation{JobID: "job-1", PID: 4242}); err != nil {
		t.Fatalf("ConfirmSession: %v", err)
	}

	if len(host.intents) != 1 || len(host.spawns) != 1 || len(host.confirms) != 1 {
		t.Fatalf("host daemon received intents=%d spawns=%d confirms=%d, want 1/1/1",
			len(host.intents), len(host.spawns), len(host.confirms))
	}
	if len(scoped.intents)+len(scoped.spawns)+len(scoped.confirms) != 0 {
		t.Fatalf("scoped daemon received traffic it must not see: intents=%d spawns=%d confirms=%d",
			len(scoped.intents), len(scoped.spawns), len(scoped.confirms))
	}
	if got := host.intents[0].WorkDir; got != workDir {
		t.Fatalf("intent WorkDir = %q, want the real worktree %q", got, workDir)
	}
	if got := host.spawns[0].WorkDir; got != workDir {
		t.Fatalf("spawn WorkDir = %q, want the real worktree %q", got, workDir)
	}
}

// TestSessionHostRoutingIgnoresScopeEnv proves GROVE_SCOPE no longer steers the
// host transport. A scoped GROVE_SCOPE (what flow injects into a worktree job's
// environment) must not pull session traffic off the published host endpoint.
func TestSessionHostRoutingIgnoresScopeEnv(t *testing.T) {
	isolateHostEnv(t)
	host := newRecordingDaemon(t, "global.sock")

	t.Setenv(HostSocketEnv, host.socketPath)
	t.Setenv(groveScopeEnv, "/some/worktree/scope")

	socketPath, viaHost := ResolveSessionHostSocket("")
	if !viaHost {
		t.Fatal("ResolveSessionHostSocket did not use the published host endpoint")
	}
	if socketPath != host.socketPath {
		t.Fatalf("resolved socket = %q, want %q", socketPath, host.socketPath)
	}
}

// TestSessionHostRoutingFallsBackWithoutHost keeps the no-host case honest:
// with nothing published, resolution is ordinary scope-derived routing, so
// ad-hoc launches (plain shell, CI) behave exactly as before.
func TestSessionHostRoutingFallsBackWithoutHost(t *testing.T) {
	isolateHostEnv(t)

	socketPath, viaHost := ResolveSessionHostSocket("")
	if viaHost {
		t.Fatal("ResolveSessionHostSocket claimed a host endpoint with none published")
	}
	if want := scopedSocketFor(t, ""); socketPath != want {
		t.Fatalf("resolved socket = %q, want unscoped socket %q", socketPath, want)
	}
}

// TestSessionHostClientPrecedence asserts the routing policy directly, with
// connect and scoped-fallback injected, so the three branches are covered
// without dialing a socket or auto-starting a real groved.
func TestSessionHostClientPrecedence(t *testing.T) {
	hostClient := NewLocalClient()
	scopedClient := NewLocalClient()

	t.Run("published and reachable wins", func(t *testing.T) {
		isolateHostEnv(t)
		t.Setenv(HostSocketEnv, "/run/host.sock")

		var dialed string
		got := sessionHostClient("/some/worktree",
			func(s string) Client { dialed = s; return hostClient },
			func(string) Client { t.Fatal("scoped fallback used while host was reachable"); return nil })
		if got != hostClient {
			t.Fatal("did not return the host client")
		}
		if dialed != "/run/host.sock" {
			t.Fatalf("dialed %q, want the published host socket", dialed)
		}
	})

	t.Run("published but dead falls back to scope", func(t *testing.T) {
		isolateHostEnv(t)
		t.Setenv(HostSocketEnv, "/run/dead.sock")

		var fallbackDir string
		got := sessionHostClient("/some/worktree",
			func(string) Client { return nil },
			func(d string) Client { fallbackDir = d; return scopedClient })
		if got != scopedClient {
			t.Fatal("a dead host endpoint did not fall back to scope routing")
		}
		if fallbackDir != "/some/worktree" {
			t.Fatalf("fallback dir = %q, want the job scope", fallbackDir)
		}
	})

	t.Run("nothing published goes straight to scope", func(t *testing.T) {
		isolateHostEnv(t)

		got := sessionHostClient("/some/worktree",
			func(string) Client { t.Fatal("dialed a host socket with none published"); return nil },
			func(string) Client { return scopedClient })
		if got != scopedClient {
			t.Fatal("unhosted launch did not use scope routing")
		}
	})

	t.Run("published env wins over a registered host", func(t *testing.T) {
		isolateHostEnv(t)
		t.Setenv(HostSocketEnv, "/run/host.sock")
		writeUIHostRegistrationFile(t, "reg.json", uiHostRegistration{
			PID: os.Getpid(), Scope: "", SocketPath: "/run/registered.sock", StartedAt: time.Now().UTC(),
		})

		var dialed string
		got := sessionHostClient("/some/worktree",
			func(s string) Client { dialed = s; return hostClient },
			func(string) Client { t.Fatal("scoped fallback used while host was reachable"); return nil })
		if got != hostClient || dialed != "/run/host.sock" {
			t.Fatalf("dialed %q, want the published endpoint to beat the registry", dialed)
		}
	})

	t.Run("registered host serves processes with no env at all", func(t *testing.T) {
		isolateHostEnv(t)
		writeUIHostRegistrationFile(t, "reg.json", uiHostRegistration{
			PID: os.Getpid(), Scope: "", SocketPath: "/run/registered.sock", StartedAt: time.Now().UTC(),
		})

		var dialed string
		got := sessionHostClient("/some/worktree",
			func(s string) Client { dialed = s; return hostClient },
			func(string) Client { t.Fatal("scoped fallback used while a registered host was reachable"); return nil })
		if got != hostClient || dialed != "/run/registered.sock" {
			t.Fatalf("dialed %q, want the registered host socket", dialed)
		}
	})

	t.Run("registered host with a dead daemon falls back to scope", func(t *testing.T) {
		isolateHostEnv(t)
		writeUIHostRegistrationFile(t, "reg.json", uiHostRegistration{
			PID: os.Getpid(), Scope: "", SocketPath: "/run/registered-dead.sock", StartedAt: time.Now().UTC(),
		})

		var fallbackDir string
		got := sessionHostClient("/some/worktree",
			func(string) Client { return nil },
			func(d string) Client { fallbackDir = d; return scopedClient })
		if got != scopedClient || fallbackDir != "/some/worktree" {
			t.Fatalf("fallback dir = %q, want scope routing after a dead registered host", fallbackDir)
		}
	})
}

// scopedSocketFor mirrors the factory's own scope→socket resolution so the
// fallback assertion cannot drift from the implementation it guards.
func scopedSocketFor(t *testing.T, dir string) string {
	t.Helper()
	_, socketPath, _ := resolveScopedTargets(dir)
	return socketPath
}

// writeUIHostRegistrationFile plants a registry record directly, so tests can
// model hosts other than the test process itself (foreign pids, multiple
// hosts) without forking anything.
func writeUIHostRegistrationFile(t *testing.T, name string, reg uiHostRegistration) {
	t.Helper()
	if err := os.MkdirAll(uiHostsDir(), 0o755); err != nil {
		t.Fatalf("mkdir hosts dir: %v", err)
	}
	data, err := json.Marshal(reg)
	if err != nil {
		t.Fatalf("marshal registration: %v", err)
	}
	if err := os.WriteFile(filepath.Join(uiHostsDir(), name), data, 0o644); err != nil {
		t.Fatalf("write registration: %v", err)
	}
}

// TestRegisterUIHostResolvesWithoutEnv is the registry's core contract — and
// the fix for the launch path env publication can never reach: flow's
// providers run inside groved's jobrunner, a process treemux never spawned,
// where HostSocketEnv is necessarily absent. Registration alone must route
// session traffic to the host daemon; unregistering must restore scope
// routing.
func TestRegisterUIHostResolvesWithoutEnv(t *testing.T) {
	isolateHostEnv(t)

	unregister, err := RegisterUIHost("", "treemux")
	if err != nil {
		t.Fatalf("RegisterUIHost: %v", err)
	}

	socketPath, viaHost := ResolveSessionHostSocket("/some/worktree/scope")
	if !viaHost {
		t.Fatal("a registered global host was not used for a worktree dir")
	}
	if want := scopedSocketFor(t, ""); socketPath != want {
		t.Fatalf("resolved socket = %q, want the global host socket %q", socketPath, want)
	}

	// A global host also covers scopeless ad-hoc launches.
	if _, viaHost := ResolveSessionHostSocket(""); !viaHost {
		t.Fatal("a registered global host was not used for a scopeless launch")
	}

	unregister()
	if _, viaHost := ResolveSessionHostSocket("/some/worktree/scope"); viaHost {
		t.Fatal("an unregistered host still attracted session traffic")
	}
}

// TestUIHostRegistryScopeSelection pins the multi-host rules: the most
// specific covering scope wins, a scoped host never covers foreign dirs, and
// among identical scopes the most recently started host wins.
func TestUIHostRegistryScopeSelection(t *testing.T) {
	isolateHostEnv(t)
	pid := os.Getpid()
	now := time.Now().UTC()

	writeUIHostRegistrationFile(t, "global.json", uiHostRegistration{
		PID: pid, Scope: "", SocketPath: "/run/global.sock", StartedAt: now,
	})
	writeUIHostRegistrationFile(t, "scoped.json", uiHostRegistration{
		PID: pid, Scope: "/wt/perf-audit", SocketPath: "/run/scoped.sock", StartedAt: now,
	})

	if got := lookupUIHostSocket("/wt/perf-audit/core"); got != "/run/scoped.sock" {
		t.Fatalf("dir inside the scoped host resolved %q, want the scoped host", got)
	}
	if got := lookupUIHostSocket("/wt/perf-audit"); got != "/run/scoped.sock" {
		t.Fatalf("the scope root itself resolved %q, want the scoped host", got)
	}
	// Prefix similarity is not containment.
	if got := lookupUIHostSocket("/wt/perf-audit-other"); got != "/run/global.sock" {
		t.Fatalf("a sibling dir resolved %q, want the global host", got)
	}
	if got := lookupUIHostSocket("/elsewhere"); got != "/run/global.sock" {
		t.Fatalf("a foreign dir resolved %q, want the global host", got)
	}

	writeUIHostRegistrationFile(t, "global-newer.json", uiHostRegistration{
		PID: pid, Scope: "", SocketPath: "/run/global2.sock", StartedAt: now.Add(time.Minute),
	})
	if got := lookupUIHostSocket("/elsewhere"); got != "/run/global2.sock" {
		t.Fatalf("equal scopes resolved %q, want the most recently started host", got)
	}
}

// TestUIHostRegistryIgnoresDeadHosts covers crash cleanup: a registration
// whose process is gone must not attract traffic, and the next registration
// prunes it from disk.
func TestUIHostRegistryIgnoresDeadHosts(t *testing.T) {
	isolateHostEnv(t)

	// A pid far above any real pid space: guaranteed ESRCH.
	const deadPID = 1 << 30
	writeUIHostRegistrationFile(t, "dead.json", uiHostRegistration{
		PID: deadPID, Scope: "", SocketPath: "/run/dead-host.sock", StartedAt: time.Now().UTC(),
	})

	if got := lookupUIHostSocket("/any"); got != "" {
		t.Fatalf("a dead host's registration resolved %q, want nothing", got)
	}
	if _, viaHost := ResolveSessionHostSocket("/any"); viaHost {
		t.Fatal("a dead host's registration steered session routing")
	}

	unregister, err := RegisterUIHost("", "treemux")
	if err != nil {
		t.Fatalf("RegisterUIHost: %v", err)
	}
	defer unregister()
	if _, err := os.Stat(filepath.Join(uiHostsDir(), "dead.json")); !os.IsNotExist(err) {
		t.Fatal("registering did not prune the dead host's record")
	}
}

// TestWithHostSocketEnvCrossesThePtyBoundary covers the gap that made the
// first cut of this fix inert: a host publishes its endpoint with os.Setenv,
// but every pane is a PTY created BY a daemon, so pane processes inherit the
// DAEMON's environ and never see it. The endpoint must therefore ride the
// create request.
func TestWithHostSocketEnvCrossesThePtyBoundary(t *testing.T) {
	t.Run("appends the published endpoint", func(t *testing.T) {
		isolateHostEnv(t)
		t.Setenv(HostSocketEnv, "/run/host.sock")

		got := withHostSocketEnv([]string{"GROVE_THEME=dark"})
		want := []string{"GROVE_THEME=dark", HostSocketEnv + "=/run/host.sock"}
		if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Fatalf("withHostSocketEnv = %v, want %v", got, want)
		}
	})

	t.Run("never invents an endpoint", func(t *testing.T) {
		isolateHostEnv(t)
		if got := withHostSocketEnv([]string{"A=1"}); len(got) != 1 {
			t.Fatalf("withHostSocketEnv = %v, want the input untouched", got)
		}
		if got := withHostSocketEnv(nil); got != nil {
			t.Fatalf("withHostSocketEnv(nil) = %v, want nil", got)
		}
	})

	t.Run("is idempotent and lets an explicit value win", func(t *testing.T) {
		isolateHostEnv(t)
		t.Setenv(HostSocketEnv, "/run/host.sock")

		explicit := []string{HostSocketEnv + "=/run/explicit.sock"}
		got := withHostSocketEnv(withHostSocketEnv(explicit))
		if len(got) != 1 || got[0] != explicit[0] {
			t.Fatalf("withHostSocketEnv = %v, want the caller's value kept once", got)
		}
	})

	t.Run("does not mutate the caller's slice", func(t *testing.T) {
		isolateHostEnv(t)
		t.Setenv(HostSocketEnv, "/run/host.sock")

		in := []string{"A=1"}
		_ = withHostSocketEnv(in)
		if len(in) != 1 {
			t.Fatalf("caller slice mutated: %v", in)
		}
	})
}
