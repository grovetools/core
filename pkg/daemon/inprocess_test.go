package daemon

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

// gitScopeDir returns a fresh git repository directory, which is what
// workspace.ResolveScope needs to answer with a non-empty scope (a bare path
// with no git or ecosystem context resolves to the global scope instead).
func gitScopeDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", "init", "--quiet", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	return dir
}

// TestInProcessDaemonNeverSpawnsScopedSibling is the fleet-restart regression:
// groved runs flow's orchestration code in-process, that code reaches for
// NewWithAutoStart, and with no host registered yet the factory used to spawn
// one scoped groved per active worktree — daemons with no clients that idle
// until --auto-shutdown reaps them.
//
// A process that has declared itself a daemon must instead be handed its OWN
// client. The test asserts the routing, not the absence of a fork, because a
// fork is only observable as a real groved on the machine; the spawn path is
// unreachable once we return early here.
func TestInProcessDaemonNeverSpawnsScopedSibling(t *testing.T) {
	isolateHostEnv(t)
	t.Cleanup(resetInProcessDaemon)

	self := newRecordingDaemon(t, "self.sock")
	MarkInProcessDaemon(self.socketPath)

	// A worktree scope with no daemon of its own — the exact shape of an
	// adopted job's worktree during a fleet restart. It has to be a real git
	// root: ResolveScope returns "" for a directory with no git or ecosystem
	// context, which would make this the global case instead of the scoped one.
	client := NewWithAutoStart(gitScopeDir(t))
	remote, ok := client.(*RemoteClient)
	if !ok {
		t.Fatalf("in-daemon client for an absent scope = %T, want this daemon's own RemoteClient", client)
	}
	if remote.SocketPath() != self.socketPath {
		t.Fatalf("in-daemon client routed to %q, want this daemon's own socket %q",
			remote.SocketPath(), self.socketPath)
	}
}

// TestInProcessDaemonNeverSpawnsItself covers the boot window: a daemon whose
// own socket is not bound yet must degrade to a LocalClient rather than fork a
// duplicate of itself.
func TestInProcessDaemonNeverSpawnsItself(t *testing.T) {
	isolateHostEnv(t)
	t.Cleanup(resetInProcessDaemon)

	// A path that will never be dialable — nothing is listening on it.
	selfSocket := scopedSocketFor(t, "")
	MarkInProcessDaemon(selfSocket)

	client := NewWithAutoStart("")
	if _, ok := client.(*LocalClient); !ok {
		t.Fatalf("a daemon dialing its own unbound socket got %T, want a LocalClient (no self-spawn)", client)
	}
}

// TestInDaemonSpawnSuppressionRules pins the policy directly, including the one
// spawn that must survive: a SCOPED groved auto-starting the GLOBAL daemon,
// which owns the shared proxy and the host-wide route table (NewGlobalClient
// from the env and channel managers). Asserted on the predicate rather than
// through newAutoStart because the allowed case ends in a real fork.
func TestInDaemonSpawnSuppressionRules(t *testing.T) {
	t.Cleanup(resetInProcessDaemon)

	const scopedSelf = "/run/groved-perf-audit.sock"
	const globalSock = "/run/groved.sock"

	// Not a daemon: nothing is suppressed, whatever the scope.
	resetInProcessDaemon()
	if _, suppressed := inDaemonSpawnSuppressed("/wt/other", "/run/groved-other.sock"); suppressed {
		t.Fatal("a plain CLI had its scoped spawn suppressed")
	}

	MarkInProcessDaemon(scopedSelf)

	if self, suppressed := inDaemonSpawnSuppressed("/wt/other", "/run/groved-other.sock"); !suppressed {
		t.Fatal("a daemon was allowed to spawn a scoped sibling")
	} else if self != scopedSelf {
		t.Fatalf("fallback socket = %q, want this daemon's own %q", self, scopedSelf)
	}
	if _, suppressed := inDaemonSpawnSuppressed("/wt/perf-audit", scopedSelf); !suppressed {
		t.Fatal("a daemon was allowed to spawn a duplicate of itself")
	}
	if _, suppressed := inDaemonSpawnSuppressed("", globalSock); suppressed {
		t.Fatal("a scoped daemon was blocked from auto-starting the GLOBAL daemon")
	}

	// The global daemon itself must not fork a second copy of itself.
	MarkInProcessDaemon(globalSock)
	if _, suppressed := inDaemonSpawnSuppressed("", globalSock); !suppressed {
		t.Fatal("the global daemon was allowed to spawn a duplicate of itself")
	}
}

// TestDaemonHostRegistrationLosesToUIHost pins the precedence that keeps
// treemux authoritative: when both a UI host and the global daemon's own
// registration cover a directory, session traffic follows the UI.
func TestDaemonHostRegistrationLosesToUIHost(t *testing.T) {
	isolateHostEnv(t)
	pid := os.Getpid()
	now := time.Now().UTC()

	writeUIHostRegistrationFile(t, "daemon.json", uiHostRegistration{
		PID: pid, Kind: hostKindDaemon, Program: "groved",
		Scope: "", SocketPath: "/run/groved.sock", StartedAt: now.Add(time.Minute),
	})
	writeUIHostRegistrationFile(t, "ui.json", uiHostRegistration{
		PID: pid, Kind: hostKindUI, Program: "treemux",
		Scope: "", SocketPath: "/run/treemux-host.sock", StartedAt: now,
	})

	// The daemon host is both newer and equally scoped; kind still decides.
	if got := lookupHostSocket("/wt/perf-audit"); got != "/run/treemux-host.sock" {
		t.Fatalf("resolved %q, want the UI host to beat the daemon host", got)
	}
}

// TestDaemonHostRegistrationAnswersWhenNoUIExists is the other half: with no UI
// anywhere — the fleet-restart window, or a headless machine — the global
// daemon's own registration is what stops a worktree client from spawning a
// scoped daemon.
func TestDaemonHostRegistrationAnswersWhenNoUIExists(t *testing.T) {
	isolateHostEnv(t)

	unregister, err := RegisterDaemonHost("", "groved")
	if err != nil {
		t.Fatalf("RegisterDaemonHost: %v", err)
	}
	defer unregister()

	socketPath, viaHost := ResolveSessionHostSocket("/wt/perf-audit")
	if !viaHost {
		t.Fatal("a registered daemon host did not answer for a worktree dir")
	}
	if want := scopedSocketFor(t, ""); socketPath != want {
		t.Fatalf("resolved socket = %q, want the global daemon socket %q", socketPath, want)
	}

	unregister()
	if _, viaHost := ResolveSessionHostSocket("/wt/perf-audit"); viaHost {
		t.Fatal("an unregistered daemon host still attracted routing")
	}
}

// TestPreKindRegistrationsCountAsUIHosts guards the on-disk compatibility of
// the new kind field: a record written by a treemux that predates it carries no
// kind and must keep its UI precedence.
func TestPreKindRegistrationsCountAsUIHosts(t *testing.T) {
	isolateHostEnv(t)
	pid := os.Getpid()
	now := time.Now().UTC()

	writeUIHostRegistrationFile(t, "legacy-ui.json", uiHostRegistration{
		PID: pid, Scope: "", SocketPath: "/run/legacy-ui.sock", StartedAt: now,
	})
	writeUIHostRegistrationFile(t, "daemon.json", uiHostRegistration{
		PID: pid, Kind: hostKindDaemon, Scope: "", SocketPath: "/run/groved.sock",
		StartedAt: now.Add(time.Minute),
	})

	if got := lookupHostSocket("/anywhere"); got != "/run/legacy-ui.sock" {
		t.Fatalf("resolved %q, want the kind-less legacy record treated as a UI host", got)
	}
}
