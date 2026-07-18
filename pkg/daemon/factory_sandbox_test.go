package daemon

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// TestIsPermissionDenied covers the dial-error classification that gates the
// no-spawn sandbox branch: EPERM/EACCES (as wrapped by net.DialTimeout) are
// the sandbox signature; ECONNREFUSED and friends are a dead daemon.
func TestIsPermissionDenied(t *testing.T) {
	wrap := func(errno syscall.Errno) error {
		return &net.OpError{
			Op:  "dial",
			Net: "unix",
			Err: os.NewSyscallError("connect", errno),
		}
	}

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"eperm", wrap(syscall.EPERM), true},
		{"eacces", wrap(syscall.EACCES), true},
		{"econnrefused", wrap(syscall.ECONNREFUSED), false},
		{"enoent", wrap(syscall.ENOENT), false},
		{"bare eperm", syscall.EPERM, true},
		{"opaque", errors.New("operation not permitted"), false},
	}
	for _, tc := range cases {
		if got := isPermissionDenied(tc.err); got != tc.want {
			t.Errorf("%s: isPermissionDenied(%v) = %v, want %v", tc.name, tc.err, got, tc.want)
		}
	}
}

// TestTryConnectDiag exercises the diagnosis-returning connect against real
// sockets: a missing socket is silent (nil, nil), a stale socket file whose
// daemon is dead surfaces ECONNREFUSED, and a live socket connects.
func TestTryConnectDiag(t *testing.T) {
	tmp := t.TempDir()

	t.Run("missing socket", func(t *testing.T) {
		client, err := tryConnectDiag(filepath.Join(tmp, "nope.sock"))
		if client != nil || err != nil {
			t.Fatalf("got (%v, %v), want (nil, nil)", client, err)
		}
	})

	t.Run("stale socket", func(t *testing.T) {
		sock := filepath.Join(tmp, "stale.sock")
		ln, err := net.Listen("unix", sock)
		if err != nil {
			t.Fatal(err)
		}
		// Keep the socket file around after Close so it is genuinely stale.
		ln.(*net.UnixListener).SetUnlinkOnClose(false)
		ln.Close()

		client, dialErr := tryConnectDiag(sock)
		if client != nil {
			t.Fatalf("got client %v for stale socket, want nil", client)
		}
		if !errors.Is(dialErr, syscall.ECONNREFUSED) {
			t.Fatalf("dial error = %v, want ECONNREFUSED", dialErr)
		}
		if isPermissionDenied(dialErr) {
			t.Fatalf("stale-socket ECONNREFUSED misclassified as permission denied")
		}
	})
}

// shortTempDir returns a freshly created temp dir with a short path. The
// autostart tests can't use t.TempDir() because their long test names push
// the derived socket path past the unix sun_path limit (~104 bytes on
// macOS), making bind/connect fail with EINVAL.
func shortTempDir(t *testing.T) string {
	t.Helper()
	tmp, err := os.MkdirTemp("", "grvd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmp) })
	return tmp
}

// setupFakeGroved isolates grove paths under tmp and puts this test binary on
// PATH as `groved` in fake-daemon mode (see TestMain). It returns the pidfile
// the fake daemon writes on startup — its existence is the spawn spy.
func setupFakeGroved(t *testing.T, tmp string) (fakePidPath string) {
	t.Helper()
	t.Setenv("GROVE_HOME", tmp)

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

	fakePidPath = filepath.Join(tmp, "fake.pid")
	t.Setenv("GROVE_FAKE_GROVED", "1")
	t.Setenv("GROVE_FAKE_PIDFILE", fakePidPath)
	return fakePidPath
}

// reapFakeGroved terminates a fake daemon recorded in pidPath, if any.
func reapFakeGroved(pidPath string) {
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return
	}
	if pid, err := strconv.Atoi(string(data)); err == nil {
		if proc, err := os.FindProcess(pid); err == nil {
			_ = proc.Signal(syscall.SIGTERM)
		}
	}
}

// TestAutoStartDeniedSocketDoesNotSpawn is the respawn-storm regression test:
// when the socket file exists but connect() is denied (the Claude Code
// sandbox signature, simulated here with a mode-000 socket), the factory must
// NOT spawn a new groved — it falls back to LocalClient and records a
// permission-denied ConnectDiagnosis for callers to surface.
func TestAutoStartDeniedSocketDoesNotSpawn(t *testing.T) {
	tmp := shortTempDir(t)
	fakePidPath := setupFakeGroved(t, tmp)

	scopeDir := filepath.Join(tmp, "scope")
	if err := os.MkdirAll(scopeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Bind a live listener on the exact socket the factory will resolve,
	// then strip its permissions so connect() is denied while Stat succeeds.
	_, socketPath, _ := resolveScopedTargets(scopeDir)
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(socketPath)
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	if err := os.Chmod(socketPath, 0o000); err != nil {
		t.Fatal(err)
	}

	// chmod-based denial doesn't bite for root — verify the simulation holds.
	if _, dialErr := net.DialTimeout("unix", socketPath, 100*time.Millisecond); !isPermissionDenied(dialErr) {
		t.Skipf("cannot simulate denied connect (dial err: %v); likely running as root", dialErr)
	}

	client := NewWithAutoStart(scopeDir)
	t.Cleanup(func() {
		_ = client.Close()
		reapFakeGroved(fakePidPath)
	})

	if _, ok := client.(*LocalClient); !ok {
		t.Fatalf("got %T, want *LocalClient fallback for denied socket", client)
	}
	if _, err := os.Stat(fakePidPath); !os.IsNotExist(err) {
		t.Fatalf("groved was spawned despite the denied (sandbox-signature) socket; stat err: %v", err)
	}

	diag := LastConnectDiagnosis()
	if diag == nil {
		t.Fatal("LastConnectDiagnosis() = nil, want a recorded permission-denied diagnosis")
	}
	if !diag.PermissionDenied {
		t.Errorf("diagnosis not marked PermissionDenied: %+v", diag)
	}
	if diag.SocketPath != socketPath {
		t.Errorf("diagnosis socket = %q, want %q", diag.SocketPath, socketPath)
	}
	if diag.Err == nil {
		t.Error("diagnosis Err is nil, want the dial error")
	}
}

// TestAutoStartStaleSocketStillSpawns pins today's behavior for a genuinely
// dead daemon: a stale socket file (ECONNREFUSED) must still take the spawn
// path, and no connect diagnosis is recorded for it.
func TestAutoStartStaleSocketStillSpawns(t *testing.T) {
	tmp := shortTempDir(t)
	fakePidPath := setupFakeGroved(t, tmp)

	scopeDir := filepath.Join(tmp, "scope")
	if err := os.MkdirAll(scopeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Leave a stale socket file behind: listener closed, file kept.
	_, socketPath, _ := resolveScopedTargets(scopeDir)
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(socketPath)
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	ln.(*net.UnixListener).SetUnlinkOnClose(false)
	ln.Close()

	client := NewWithAutoStart(scopeDir)
	t.Cleanup(func() {
		_ = client.Close()
		reapFakeGroved(fakePidPath)
	})

	// The spawn spy: the fake groved writes its pidfile before binding, and
	// newAutoStart doesn't return until the ready handshake resolves, so a
	// missing pidfile here means the factory never spawned.
	if _, err := os.Stat(fakePidPath); err != nil {
		t.Fatalf("stale socket did not trigger a daemon spawn: %v", err)
	}
	if diag := LastConnectDiagnosis(); diag != nil {
		t.Errorf("LastConnectDiagnosis() = %+v, want nil for the stale-socket spawn path", diag)
	}
}
