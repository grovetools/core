package checks

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/grovetools/core/pkg/doctor"
)

func TestOrphanSockets_AliveSocketIsOK(t *testing.T) {
	// macOS limits sockaddr_un paths to ~104 bytes, so don't use t.TempDir.
	runtime, err := os.MkdirTemp("/tmp", "grvd-rt-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(runtime) })
	state, err := os.MkdirTemp("/tmp", "grvd-st-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(state) })
	sock := filepath.Join(runtime, "groved-live-aaaaaaaa.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })

	c := &orphanSocketsCheck{
		runtimeDir: func() string { return runtime },
		stateDir:   func() string { return state },
		dial:       defaultUnixDial,
	}
	res := c.Run(context.Background(), doctor.RunOptions{})
	if res.Status != doctor.StatusOK {
		t.Fatalf("expected OK, got %s: %s", res.Status, res.Message)
	}
}

func TestOrphanSockets_OrphanSocketAndPidRemoved(t *testing.T) {
	runtime := t.TempDir()
	state := t.TempDir()
	sock := filepath.Join(runtime, "groved-dead-bbbbbbbb.sock")
	pid := filepath.Join(state, "groved-dead-bbbbbbbb.pid")
	if err := os.WriteFile(sock, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pid, []byte("999999\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	c := &orphanSocketsCheck{
		runtimeDir: func() string { return runtime },
		stateDir:   func() string { return state },
		dial: func(string, time.Duration) error {
			return errors.New("refused")
		},
	}

	res := c.Run(context.Background(), doctor.RunOptions{})
	if res.Status != doctor.StatusWarn || !res.Fixable {
		t.Fatalf("expected warn+fixable, got %+v", res)
	}
	if err := c.AutoFix(context.Background()); err != nil {
		t.Fatalf("autofix: %v", err)
	}
	if _, err := os.Stat(sock); !os.IsNotExist(err) {
		t.Fatalf("socket should have been removed: %v", err)
	}
	if _, err := os.Stat(pid); !os.IsNotExist(err) {
		t.Fatalf("pid file should have been removed: %v", err)
	}
}
