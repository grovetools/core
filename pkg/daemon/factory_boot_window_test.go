package daemon

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/grovetools/core/pkg/paths"
)

// The fleet-restart boot window: every daemon has just been killed, the global
// groved is coming back, and for the seconds it takes to bind, a client in a
// worktree finds no host and spawns a scoped groved nothing will ever use. The
// tests below pin the two halves of the fix — a client waits for a daemon that
// is demonstrably starting, and only for one that is.

// writeGlobalPidfile plants a pidfile for the GLOBAL daemon naming a live
// process. os.Getppid() is the go test runner: alive for the whole test, and
// not this process (which livePidFromFile deliberately ignores, since a daemon
// cannot be waiting for itself).
func writeGlobalPidfile(t *testing.T) {
	t.Helper()
	path := paths.PidFilePath("")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getppid())), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestAutoStartWaitsForStartingHostDaemon is the fix itself: a host record
// marked Starting means "a daemon is on its way", and the client rides it
// instead of racing it to a spawn.
func TestAutoStartWaitsForStartingHostDaemon(t *testing.T) {
	f := newRedirectFixture(t)

	hostSock := filepath.Join(shortTmpDir(t, "gbw-host"), "host.sock")
	writeUIHostRegistrationFile(t, "booting.json", uiHostRegistration{
		PID: os.Getpid(), Program: "groved", Kind: hostKindDaemon,
		Scope: "", SocketPath: hostSock, StartedAt: time.Now().UTC(),
		Starting: true,
	})

	// The daemon binds a little later, as a real one does.
	bound := make(chan struct{})
	go func() {
		time.Sleep(300 * time.Millisecond)
		listenStub(t, hostSock)
		close(bound)
	}()

	client := NewWithAutoStart(f.scopeDir)
	defer client.Close()
	<-bound

	f.assertRemoteOn(t, client, hostSock)
	if f.spawnAttempted() {
		t.Fatal("a scoped groved was spawned while the host daemon was still booting")
	}
}

// TestAutoStartWaitsForGlobalDaemonPidfile covers the window the registry
// cannot describe: groved has taken the pidfile but has not published a record
// yet (or is too old to). The live pidfile plus a socket that does not answer
// is the same statement, and the client rides that daemon once it binds.
func TestAutoStartWaitsForGlobalDaemonPidfile(t *testing.T) {
	f := newRedirectFixture(t)
	writeGlobalPidfile(t)
	globalSock := paths.SocketPath("")

	bound := make(chan struct{})
	go func() {
		time.Sleep(300 * time.Millisecond)
		listenStub(t, globalSock)
		close(bound)
	}()

	client := NewWithAutoStart(f.scopeDir)
	defer client.Close()
	<-bound

	f.assertRemoteOn(t, client, globalSock)
	if f.spawnAttempted() {
		t.Fatal("a scoped groved was spawned while the global daemon was still booting")
	}
}

// TestAutoStartSpawnsWhenNothingIsBooting keeps the ordinary path free of the
// grace: with no starting record and no live pidfile, the client must reach the
// spawn path immediately rather than pay the window for nothing.
func TestAutoStartSpawnsWhenNothingIsBooting(t *testing.T) {
	f := newRedirectFixture(t)
	// A window far longer than the spawn path's own ~2s (the sentinel groved
	// dies instantly and waitForDaemonReady keeps its exitedGracePeriod), so
	// "the grace did not apply" is unambiguous in the elapsed time.
	t.Setenv(HostBootGraceEnv, "30s")

	start := time.Now()
	client := NewWithAutoStart(f.scopeDir)
	defer client.Close()
	elapsed := time.Since(start)

	if !f.spawnAttempted() {
		t.Fatal("spawn path not reached with nothing booting anywhere")
	}
	if elapsed > 10*time.Second {
		t.Fatalf("waited %v before spawning; nothing was booting, so the grace must not apply", elapsed)
	}
}

// TestAutoStartGivesUpOnStartingHostThatNeverBinds bounds the wait: a daemon
// that announces itself and then never binds must not stall the client past the
// grace window. It is a spawn, late, not a hang.
func TestAutoStartGivesUpOnStartingHostThatNeverBinds(t *testing.T) {
	f := newRedirectFixture(t)
	t.Setenv(HostBootGraceEnv, "400ms")

	writeUIHostRegistrationFile(t, "stuck.json", uiHostRegistration{
		PID: os.Getpid(), Program: "groved", Kind: hostKindDaemon,
		Scope: "", SocketPath: "/tmp/gbw-never-binds.sock", StartedAt: time.Now().UTC(),
		Starting: true,
	})

	start := time.Now()
	client := NewWithAutoStart(f.scopeDir)
	defer client.Close()
	elapsed := time.Since(start)

	if !f.spawnAttempted() {
		t.Fatal("a host that never binds must fall through to the spawn path")
	}
	if elapsed < 400*time.Millisecond {
		t.Fatalf("gave up after %v; the grace window should have been waited out", elapsed)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("waited %v; the grace window is meant to bound this", elapsed)
	}
}

// TestAutoStartOptOutsIgnoreBootGrace: callers that exist to spawn the scope's
// own daemon (treemux's EarlyReady, PairWith, RequireScopedDaemon) opt out of
// the host redirect, and therefore out of its grace. A booting global daemon
// must not delay them.
func TestAutoStartOptOutsIgnoreBootGrace(t *testing.T) {
	f := newRedirectFixture(t)
	writeGlobalPidfile(t)
	t.Setenv(HostBootGraceEnv, "30s")

	start := time.Now()
	client := NewWithAutoStartOpts(f.scopeDir, RequireScopedDaemon(), SuppressStartNotice())
	defer client.Close()
	elapsed := time.Since(start)

	if !f.spawnAttempted() {
		t.Fatal("RequireScopedDaemon did not reach the spawn path")
	}
	if elapsed > 10*time.Second {
		t.Fatalf("RequireScopedDaemon waited %v on a booting host daemon", elapsed)
	}
}

// TestAutoStartWaitsForADaemonStartingOnItsOwnSocket is the other duplicate:
// something is already starting the daemon for THIS socket. Spawning would fork
// a groved that loses pidfile.Acquire and exits seconds later, so the factory
// waits for the one that is coming.
func TestAutoStartWaitsForADaemonStartingOnItsOwnSocket(t *testing.T) {
	f := newRedirectFixture(t)

	// A live process holds the SCOPED pidfile; its socket binds shortly after.
	pidPath := paths.PidFilePath(f.scope)
	if err := os.MkdirAll(filepath.Dir(pidPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getppid())), 0o644); err != nil {
		t.Fatal(err)
	}

	bound := make(chan struct{})
	go func() {
		time.Sleep(300 * time.Millisecond)
		listenStub(t, f.scopedSock)
		close(bound)
	}()

	// RequireScopedDaemon so the host redirect cannot be what answers this.
	client := NewWithAutoStartOpts(f.scopeDir, RequireScopedDaemon(), SuppressStartNotice())
	defer client.Close()
	<-bound

	f.assertRemoteOn(t, client, f.scopedSock)
	if f.spawnAttempted() {
		t.Fatal("spawned a duplicate groved for a socket another process was already starting")
	}
}

// TestLivePidFromFile pins the reader's failure modes: everything that is not a
// live process reads as 0, so a stale artifact can never make a client wait.
func TestLivePidFromFile(t *testing.T) {
	dir := shortTmpDir(t, "gbw-pid")
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	if got := livePidFromFile(filepath.Join(dir, "absent")); got != 0 {
		t.Fatalf("missing pidfile = %d, want 0", got)
	}
	if got := livePidFromFile(write("garbage", "not-a-pid")); got != 0 {
		t.Fatalf("malformed pidfile = %d, want 0", got)
	}
	if got := livePidFromFile(write("dead", strconv.Itoa(1<<30))); got != 0 {
		t.Fatalf("stale pidfile = %d, want 0", got)
	}
	if got := livePidFromFile(write("live", strconv.Itoa(os.Getppid())+"\n")); got != os.Getppid() {
		t.Fatalf("live pidfile = %d, want %d", got, os.Getppid())
	}
}

// TestHostBootGraceEnvOverride: fixtures compress or disable the window
// without a rebuild, and a malformed value falls back to the default rather
// than to zero (which would silently restore the racing behavior).
func TestHostBootGraceEnvOverride(t *testing.T) {
	if got := hostBootGrace(); got != defaultHostBootGrace {
		t.Fatalf("unset = %v, want %v", got, defaultHostBootGrace)
	}
	t.Setenv(HostBootGraceEnv, "250ms")
	if got := hostBootGrace(); got != 250*time.Millisecond {
		t.Fatalf("override = %v, want 250ms", got)
	}
	t.Setenv(HostBootGraceEnv, "0")
	if got := hostBootGrace(); got != 0 {
		t.Fatalf("zero = %v, want 0", got)
	}
	t.Setenv(HostBootGraceEnv, "soon")
	if got := hostBootGrace(); got != defaultHostBootGrace {
		t.Fatalf("malformed = %v, want the default %v", got, defaultHostBootGrace)
	}
}
