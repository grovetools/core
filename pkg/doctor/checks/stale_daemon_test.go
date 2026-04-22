package checks

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"

	"github.com/grovetools/core/pkg/doctor"
	"github.com/grovetools/core/pkg/paths"
)

func setupStaleDaemonFixture(t *testing.T, scope string, sameInode bool) *staleDaemonCheck {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GROVE_HOME", home)

	state := paths.StateDir()
	if err := os.MkdirAll(state, 0o755); err != nil {
		t.Fatal(err)
	}
	binDir := paths.BinDir()
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write target binary and either the same file or a different one for
	// the "running" binary.
	target := filepath.Join(binDir, "groved")
	if err := os.WriteFile(target, []byte("current"), 0o755); err != nil {
		t.Fatal(err)
	}
	running := target
	if !sameInode {
		running = filepath.Join(t.TempDir(), "groved-old")
		if err := os.WriteFile(running, []byte("old"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	pidPath := paths.PidFilePath(scope)
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}

	c := &staleDaemonCheck{
		stateDir:     paths.StateDir,
		binDir:       paths.BinDir,
		getenv:       func(string) string { return scope },
		getwd:        func() (string, error) { return "/unused", nil },
		resolveScope: func(string) string { return scope },
		lsofBinaryPath: func(int) (string, error) {
			return running, nil
		},
		statInode:   defaultStatInode,
		killProcess: func(int, syscall.Signal) error { return nil },
	}
	return c
}

func TestStaleDaemon_InodeMatchReturnsOK(t *testing.T) {
	c := setupStaleDaemonFixture(t, "/tmp/testscope", true)
	res := c.Run(context.Background(), doctor.RunOptions{})
	if res.Status != doctor.StatusOK {
		t.Fatalf("expected OK when inodes match, got %s: %s", res.Status, res.Message)
	}
}

func TestStaleDaemon_InodeMismatchReturnsFail(t *testing.T) {
	c := setupStaleDaemonFixture(t, "/tmp/testscope-stale", false)
	res := c.Run(context.Background(), doctor.RunOptions{})
	if res.Status != doctor.StatusFail || !res.Fixable {
		t.Fatalf("expected Fail+Fixable on mismatch, got %+v", res)
	}
	if c.stalePID != os.Getpid() {
		t.Fatalf("expected stalePID recorded, got %d", c.stalePID)
	}
}

func TestStaleDaemon_NoScopeReturnsOK(t *testing.T) {
	c := &staleDaemonCheck{
		getenv:       func(string) string { return "" },
		getwd:        func() (string, error) { return "/tmp", nil },
		resolveScope: func(string) string { return "" },
	}
	res := c.Run(context.Background(), doctor.RunOptions{})
	if res.Status != doctor.StatusOK {
		t.Fatalf("expected OK when no scope, got %s", res.Status)
	}
}

func TestStaleDaemon_AutoFixRefusesProtectedScope(t *testing.T) {
	c := &staleDaemonCheck{
		stalePID:    12345,
		staleScope:  "/home/u/Code/grovetools",
		killProcess: func(int, syscall.Signal) error { return nil },
	}
	if err := c.AutoFix(context.Background()); err == nil {
		t.Fatalf("expected refusal for protected scope")
	}
}
