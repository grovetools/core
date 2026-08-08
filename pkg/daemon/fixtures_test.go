package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The identity gate is the only thing standing between this sweep and the
// user's live daemon, so it gets the most tests. Everything below is really
// one assertion in two directions: fixtures are recognized, and nothing else
// ever is.

func TestFixtureNamespaceRecognizesProducers(t *testing.T) {
	cases := []struct {
		name string
		path string
		want string
	}{
		{
			name: "tend harness runtime dir",
			path: "/tmp/tend-grove-tend-treemux-panelkit-1234567/grove/groved.sock",
			want: "/tmp/tend-grove-tend-treemux-panelkit-1234567",
		},
		{
			name: "tend harness fixture tuimux",
			path: "/tmp/tend-grove-tend-flow-42/tuimux-test.sock",
			want: "/tmp/tend-grove-tend-flow-42",
		},
		{
			name: "tend debug session",
			path: "/tmp/tend-debug-987/grove/groved.sock",
			want: "/tmp/tend-debug-987",
		},
		{
			name: "tend lab",
			path: "/tmp/tendlab-55/grove/groved-lab-abc12345.sock",
			want: "/tmp/tendlab-55",
		},
		{
			name: "tuipilot bare socket file is its own namespace",
			path: "/tmp/tuipilot-20260805-gtd-phase1-tuimux.sock",
			want: "/tmp/tuipilot-20260805-gtd-phase1-tuimux.sock",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FixtureNamespace(tc.path); got != tc.want {
				t.Errorf("FixtureNamespace(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestFixtureNamespaceRefusesRealPaths(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}

	paths := []string{
		filepath.Join(home, ".local", "state", "grove", "groved.sock"),
		filepath.Join(home, ".local", "state", "tuimux", "daemon.sock"),
		filepath.Join(home, ".local", "state", "grove", "groved-core-e2435831.sock"),
		"/var/run/groved.sock",
		"/tmp/groved.sock",                 // temp root, but no fixture prefix
		"/tmp/some-other-tool/groved.sock", // temp root, unrelated tool
		"/tmp/tuimux-daemon.sock",          // "tuimux-" is not a fixture prefix
		"relative/tend-foo/groved.sock",    // not absolute
		"",
	}

	for _, p := range paths {
		if ns := FixtureNamespace(p); ns != "" {
			t.Errorf("FixtureNamespace(%q) = %q, want \"\" — this path is not a fixture", p, ns)
		}
	}
}

// A fixture prefix under the REAL grove state dir must still be refused: the
// explicit real-dir guard has to win over the prefix match, because a user who
// sets GROVE_HOME or TMPDIR somewhere odd must not lose their daemon.
func TestFixtureNamespaceRefusesRealDirsEvenWithFixturePrefix(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("GROVE_HOME", filepath.Join(tmp, "tend-looks-like-a-fixture"))
	t.Setenv("TMPDIR", tmp)

	sock := filepath.Join(tmp, "tend-looks-like-a-fixture", "run", "groved.sock")
	if ns := FixtureNamespace(sock); ns != "" {
		t.Fatalf("FixtureNamespace(%q) = %q, want \"\" — path is inside the real GROVE_HOME", sock, ns)
	}
}

func TestFixtureSocketArg(t *testing.T) {
	cases := []struct {
		name     string
		cmdline  string
		wantKind string
		wantSock string
	}{
		{
			name:     "groved with explicit socket",
			cmdline:  "/opt/grove/bin/groved start --socket /tmp/tend-x-1/grove/groved.sock --pidfile /tmp/p.pid",
			wantKind: "groved",
			wantSock: "/tmp/tend-x-1/grove/groved.sock",
		},
		{
			name:     "groved with --socket=value form",
			cmdline:  "groved start --socket=/tmp/tend-x-1/grove/groved.sock",
			wantKind: "groved",
			wantSock: "/tmp/tend-x-1/grove/groved.sock",
		},
		{
			name:     "tuimux daemon",
			cmdline:  "tuimux daemon --socket /tmp/tuipilot-20260805-x-tuimux.sock",
			wantKind: "tuimux",
			wantSock: "/tmp/tuipilot-20260805-x-tuimux.sock",
		},
		{
			// The real unscoped groved derives its socket from paths and passes
			// no flag, so it is unmatched by construction.
			name:    "groved without --socket is never matched",
			cmdline: "/opt/grove/bin/groved start",
		},
		{
			name:    "tuimux daemon stop is a client, not a daemon",
			cmdline: "tuimux daemon stop --socket /tmp/tuipilot-20260805-x-tuimux.sock",
		},
		{
			name:    "tuimux daemon status is a client, not a daemon",
			cmdline: "tuimux daemon status --socket /tmp/tuipilot-20260805-x-tuimux.sock",
		},
		{
			name:    "groved status is a client, not a daemon",
			cmdline: "groved status --socket /tmp/tend-x-1/grove/groved.sock",
		},
		{
			name:    "an unrelated binary mentioning a socket",
			cmdline: "tail -f --socket /tmp/tend-x-1/grove/groved.sock",
		},
		{
			name:    "empty",
			cmdline: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kind, sock := fixtureSocketArg(tc.cmdline)
			if kind != tc.wantKind || sock != tc.wantSock {
				t.Errorf("fixtureSocketArg(%q) = (%q, %q), want (%q, %q)",
					tc.cmdline, kind, sock, tc.wantKind, tc.wantSock)
			}
		})
	}
}

func TestFixtureLeaseRoundTrip(t *testing.T) {
	ns := t.TempDir()

	if _, ok := ReadFixtureLease(ns); ok {
		t.Fatal("read a lease from a namespace that has none")
	}

	want := FixtureLease{OwnerPID: os.Getpid(), Owner: "tend/harness"}
	if err := WriteFixtureLease(ns, want); err != nil {
		t.Fatalf("WriteFixtureLease: %v", err)
	}

	got, ok := ReadFixtureLease(ns)
	if !ok {
		t.Fatal("lease not readable after write")
	}
	if got.OwnerPID != want.OwnerPID || got.Owner != want.Owner {
		t.Errorf("lease round trip = %+v, want owner_pid=%d owner=%q", got, want.OwnerPID, want.Owner)
	}
	if got.CreatedAt.IsZero() {
		t.Error("WriteFixtureLease did not stamp CreatedAt")
	}
}

func TestFixtureLeaseAbandoned(t *testing.T) {
	t.Run("live pid is not abandoned", func(t *testing.T) {
		abandoned, _ := FixtureLease{OwnerPID: os.Getpid()}.Abandoned()
		if abandoned {
			t.Error("our own live PID reported as abandoned")
		}
	})

	t.Run("dead pid is abandoned", func(t *testing.T) {
		// PID 0 is never a valid owner; use a PID that cannot be running by
		// starting and reaping a trivial child.
		abandoned, why := FixtureLease{OwnerPID: deadPID(t)}.Abandoned()
		if !abandoned {
			t.Error("a reaped PID was not reported as abandoned")
		}
		if !strings.Contains(why, "is gone") {
			t.Errorf("reason = %q, want it to explain the dead owner", why)
		}
	})

	t.Run("missing owner dir is abandoned", func(t *testing.T) {
		abandoned, why := FixtureLease{OwnerPath: "/definitely/not/a/real/lab/dir"}.Abandoned()
		if !abandoned {
			t.Error("a vanished owner dir was not reported as abandoned")
		}
		if !strings.Contains(why, "no longer exists") {
			t.Errorf("reason = %q, want it to explain the vanished dir", why)
		}
	})

	t.Run("existing owner dir is not abandoned", func(t *testing.T) {
		abandoned, _ := FixtureLease{OwnerPath: t.TempDir()}.Abandoned()
		if abandoned {
			t.Error("an existing owner dir reported as abandoned")
		}
	})

	t.Run("empty lease proves nothing", func(t *testing.T) {
		abandoned, _ := FixtureLease{}.Abandoned()
		if abandoned {
			t.Error("an empty lease must not authorize a kill")
		}
	})
}

// deadPID returns a PID that is guaranteed not to be running: we fork a
// process that exits immediately and wait for it.
func deadPID(t *testing.T) int {
	t.Helper()
	proc, err := os.StartProcess("/usr/bin/true", []string{"true"}, &os.ProcAttr{})
	if err != nil {
		t.Skipf("cannot spawn a throwaway process: %v", err)
	}
	pid := proc.Pid
	if _, err := proc.Wait(); err != nil {
		t.Skipf("cannot reap throwaway process: %v", err)
	}
	return pid
}

func TestSweepKeepsLiveAndUnknownAgeDaemons(t *testing.T) {
	// SweepFixtureDaemons reads the live process table, so drive the decision
	// logic through the same rules with a synthetic daemon set.
	live := FixtureDaemon{
		Kind: "groved", PID: 111, Namespace: "/tmp/tend-a-1",
		Elapsed: 4 * time.Hour,
		Lease:   FixtureLease{OwnerPID: os.Getpid()}, HasLease: true,
	}
	if orphaned, _ := live.Orphaned(); orphaned {
		t.Error("a daemon whose lease owner is alive must never be orphaned, however old")
	}

	unleased := FixtureDaemon{Kind: "groved", PID: 222, Namespace: "/tmp/tend-b-1", Elapsed: 4 * time.Hour}
	if orphaned, _ := unleased.Orphaned(); orphaned {
		t.Error("a lease-less daemon is not PROVABLY orphaned; the age threshold decides it")
	}

	dead := FixtureDaemon{
		Kind: "groved", PID: 333, Namespace: "/tmp/tend-c-1",
		Lease: FixtureLease{OwnerPID: deadPID(t)}, HasLease: true,
	}
	orphaned, why := dead.Orphaned()
	if !orphaned {
		t.Error("a daemon whose lease owner died must be orphaned regardless of age")
	}
	if why == "" {
		t.Error("orphan verdicts must carry an auditable reason")
	}
}

func TestSweepDryRunSignalsNothing(t *testing.T) {
	// Against the real process table there may be zero fixtures; the contract
	// under test is that a dry run never errors and never reports a signal
	// failure, whatever it finds.
	decisions, err := SweepFixtureDaemons(SweepOptions{DryRun: true, IgnoreAge: true})
	if err != nil {
		t.Fatalf("SweepFixtureDaemons: %v", err)
	}
	for _, d := range decisions {
		if d.Err != nil {
			t.Errorf("dry run signalled pid %d: %v", d.Daemon.PID, d.Err)
		}
		if d.Reason == "" {
			t.Errorf("pid %d got no reason for its verdict", d.Daemon.PID)
		}
	}
}

// fakeDaemonEnv makes this test binary impersonate a long-lived daemon when
// re-executed, via the standard Go helper-process pattern.
const fakeDaemonEnv = "GROVE_FIXTURES_TEST_FAKE_DAEMON"

// TestHelperFakeDaemon is not a test. It is the body of the fake daemon that
// spawnFakeFixtureDaemon starts, and it does nothing but stay alive long
// enough to be found and signalled.
func TestHelperFakeDaemon(t *testing.T) {
	if os.Getenv(fakeDaemonEnv) != "1" {
		t.Skip("helper process; runs only when re-executed by spawnFakeFixtureDaemon")
	}
	time.Sleep(2 * time.Minute)
}

// spawnFakeFixtureDaemon starts a long-lived process whose argv is
// indistinguishable, to the census, from a real fixture groved: argv[0]
// basename "groved", the "start" subcommand, and a --socket inside a fixture
// namespace.
//
// The trick is that argv[0] need not name the file being executed. The
// executable is this very test binary — copying a system binary such as
// /bin/sh does not work on macOS, where the copy is SIGKILLed on exec — while
// the argv, which is all `ps` (and therefore the census) ever sees, is ours to
// compose.
func spawnFakeFixtureDaemon(t *testing.T, ns string) *os.Process {
	t.Helper()

	self, err := os.Executable()
	if err != nil {
		t.Skipf("cannot locate the test binary to re-exec: %v", err)
	}
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		t.Skipf("cannot open %s: %v", os.DevNull, err)
	}
	defer func() { _ = devNull.Close() }()

	sock := filepath.Join(ns, "groved.sock")
	proc, err := os.StartProcess(self, []string{
		filepath.Join(ns, "groved"),
		"-test.run=TestHelperFakeDaemon",
		"start", "--socket", sock,
	}, &os.ProcAttr{
		Env:   append(os.Environ(), fakeDaemonEnv+"=1"),
		Files: []*os.File{devNull, devNull, devNull},
	})
	if err != nil {
		t.Skipf("cannot spawn a fake daemon: %v", err)
	}
	t.Cleanup(func() {
		_ = proc.Kill()
		_, _ = proc.Wait()
	})

	// The census reads the process table, so wait until the kernel actually
	// shows this process rather than assuming StartProcess is enough.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		found, findErr := FindFixtureDaemons()
		if findErr == nil {
			for _, d := range found {
				if d.PID == proc.Pid {
					return proc
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("fake daemon pid %d never appeared in the fixture census", proc.Pid)
	return nil
}

// TestSweepTerminatesOrphanedFixture exercises the branch that actually
// signals — the one every other test deliberately avoids.
//
// Two things keep it from touching anything real. The namespace is created by
// this test under /tmp, and SweepOptions.Namespace confines the sweep to it,
// so a developer's live fixtures (never mind their real groved) are outside
// the sweep's reach by construction rather than by luck of timing.
func TestSweepTerminatesOrphanedFixture(t *testing.T) {
	ns, err := os.MkdirTemp("/tmp", "tend-sweeptest-")
	if err != nil {
		t.Skipf("cannot create a fixture namespace under /tmp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(ns) })

	proc := spawnFakeFixtureDaemon(t, ns)

	// A lease whose owner is already dead: the sweep must collect on the
	// PROOF, not on the age threshold, which this seconds-old process fails.
	if err := WriteFixtureLease(ns, FixtureLease{OwnerPID: deadPID(t), Owner: "fixtures_test"}); err != nil {
		t.Fatalf("WriteFixtureLease: %v", err)
	}

	decisions, err := SweepFixtureDaemons(SweepOptions{Namespace: ns})
	if err != nil {
		t.Fatalf("SweepFixtureDaemons: %v", err)
	}
	if len(decisions) != 1 {
		t.Fatalf("scoped sweep returned %d decisions, want exactly the one fixture in %s: %+v", len(decisions), ns, decisions)
	}
	d := decisions[0]
	if d.Daemon.PID != proc.Pid {
		t.Fatalf("swept pid %d, want the fake daemon %d", d.Daemon.PID, proc.Pid)
	}
	if !d.Swept {
		t.Fatalf("fixture with a dead lease owner was kept: %s", d.Reason)
	}
	if d.Err != nil {
		t.Fatalf("signalling pid %d failed: %v", d.Daemon.PID, d.Err)
	}
	if !strings.Contains(d.Reason, "lease owner pid") {
		t.Errorf("reason %q does not name the proof the sweep acted on", d.Reason)
	}

	// The signal has to have LANDED, not merely been sent.
	if _, err := proc.Wait(); err != nil {
		t.Fatalf("waiting for the swept daemon: %v", err)
	}
}

// TestSweepNamespaceScopeExcludesOtherFixtures is the safety half of the test
// above: the same live fake daemon must be invisible to a sweep scoped
// elsewhere. Without this, "the sweep only killed our process" could be true
// merely because nothing else happened to be running.
func TestSweepNamespaceScopeExcludesOtherFixtures(t *testing.T) {
	ns, err := os.MkdirTemp("/tmp", "tend-sweeptest-")
	if err != nil {
		t.Skipf("cannot create a fixture namespace under /tmp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(ns) })

	proc := spawnFakeFixtureDaemon(t, ns)

	decisions, err := SweepFixtureDaemons(SweepOptions{
		Namespace: filepath.Join("/tmp", "tend-sweeptest-nonexistent"),
		IgnoreAge: true,
	})
	if err != nil {
		t.Fatalf("SweepFixtureDaemons: %v", err)
	}
	for _, d := range decisions {
		if d.Daemon.PID == proc.Pid {
			t.Fatalf("sweep scoped to another namespace reported our fixture: %s", d.Reason)
		}
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("fixture outside the sweep's scope did not survive it: %v", err)
	}
}

func TestFormatFixtureDaemonsEmpty(t *testing.T) {
	if got := FormatFixtureDaemons(nil); !strings.Contains(got, "No fixture daemons") {
		t.Errorf("FormatFixtureDaemons(nil) = %q", got)
	}
}
