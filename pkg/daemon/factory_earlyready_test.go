package daemon

import (
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// TestMain doubles as an old-groved stand-in. When GROVE_FAKE_GROVED is set,
// the test binary — re-exec'd by autoStartDaemon as "groved" — runs
// runFakeGroved instead of the test suite. This is the classic exec-helper
// pattern: it lets the factory retry test drive a real spawn without a real
// daemon binary.
func TestMain(m *testing.M) {
	if os.Getenv("GROVE_FAKE_GROVED") == "1" {
		runFakeGroved()
		return
	}
	os.Exit(m.Run())
}

// runFakeGroved mimics an older groved: it rejects the unknown --ready-at flag
// by exiting non-zero (as cobra would during flag parsing), and otherwise
// binds the requested --socket and serves /health, so a flagless respawn
// produces a genuinely working daemon.
func runFakeGroved() {
	args := os.Args
	for _, a := range args {
		if a == "--ready-at" {
			// Old binary: unknown flag → exit before binding anything.
			os.Exit(2)
		}
	}

	flagValue := func(name string) string {
		for i := 0; i < len(args)-1; i++ {
			if args[i] == name {
				return args[i+1]
			}
		}
		return ""
	}

	// Record our PID so the test can reap us regardless of grove path layout.
	if p := os.Getenv("GROVE_FAKE_PIDFILE"); p != "" {
		_ = os.WriteFile(p, []byte(strconv.Itoa(os.Getpid())), 0o644)
	}

	sock := flagValue("--socket")
	if sock == "" {
		os.Exit(3)
	}
	_ = os.Remove(sock)
	_ = os.MkdirAll(filepath.Dir(sock), 0o755)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		os.Exit(4)
	}

	// Close the inherited ready-fd (fd 3) so the parent's waitForDaemonReady
	// unblocks on EOF, exactly as the real groved does after bind.
	if f := os.NewFile(3, "ready"); f != nil {
		_ = f.Close()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Exit promptly on signal, and self-destruct after a backstop so a stray
	// fake daemon can never outlive the test run.
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGTERM, os.Interrupt)
		<-c
		_ = ln.Close()
		os.Exit(0)
	}()
	go func() {
		time.Sleep(20 * time.Second)
		os.Exit(0)
	}()

	_ = http.Serve(ln, mux)
	os.Exit(0)
}

// TestEarlyReadyRespawnsOnOldBinary exercises the whole EarlyReady() failure
// path: the first spawn passes --ready-at=bind, the (fake) old binary rejects
// it and dies, and the factory must respawn once WITHOUT the flag and land a
// working RemoteClient — never falling back to LocalClient.
func TestEarlyReadyRespawnsOnOldBinary(t *testing.T) {
	tmp := t.TempDir()

	// Isolate all grove path resolution (socket/pidfile) under tmp.
	t.Setenv("GROVE_HOME", tmp)

	// Put a `groved` on PATH that is this test binary in fake-daemon mode.
	binDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(self, filepath.Join(binDir, "groved")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	// Children inherit these: run as the fake daemon and report their PID.
	fakePidPath := filepath.Join(tmp, "fake.pid")
	t.Setenv("GROVE_FAKE_GROVED", "1")
	t.Setenv("GROVE_FAKE_PIDFILE", fakePidPath)

	scopeDir := filepath.Join(tmp, "scope")
	if err := os.MkdirAll(scopeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	client := NewWithAutoStartOpts(scopeDir, EarlyReady())
	t.Cleanup(func() {
		_ = client.Close()
		if data, err := os.ReadFile(fakePidPath); err == nil {
			if pid, err := strconv.Atoi(string(data)); err == nil {
				if proc, err := os.FindProcess(pid); err == nil {
					_ = proc.Signal(syscall.SIGTERM)
				}
			}
		}
	})

	if _, ok := client.(*RemoteClient); !ok {
		t.Fatalf("EarlyReady() fell back to %T; the flagless respawn should have reached a working daemon", client)
	}
	if !client.IsRunning() {
		t.Fatal("EarlyReady() returned a client that is not running; the respawned fake daemon should serve /health")
	}
	// The only way a working daemon exists is via the flagless retry (the
	// first, flagged spawn exits before binding), so a live /health here is
	// proof the retry path ran end-to-end.
}
