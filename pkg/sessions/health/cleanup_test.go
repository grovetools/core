package health

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/grovetools/core/pkg/models"
	"github.com/grovetools/core/pkg/sessions"
)

func TestCleanerRejectsSyntheticSession(t *testing.T) {
	cleaner := Cleaner{StateDir: t.TempDir()}
	out, err := cleaner.Clean(context.Background(), &Probe{Session: &models.Session{
		ID: "projected-job", Synthetic: true, Provenance: "flow_job_projection", PID: os.Getpid(),
	}})
	if err == nil {
		t.Fatal("Clean accepted a synthetic row")
	}
	if out.DaemonKilled || out.SignalledPID != 0 || out.RemovedRecovery {
		t.Fatalf("synthetic cleanup had side effects: %+v", out)
	}
}

func TestCleanerLocalFallbackClearsAllRegistryAliases(t *testing.T) {
	stateDir := t.TempDir()
	registry, err := sessions.NewFileSystemRegistryAt(stateDir)
	if err != nil {
		t.Fatalf("NewFileSystemRegistryAt: %v", err)
	}
	metadata := sessions.SessionMetadata{
		SessionID: "job-1", JobID: "job-1", ClaudeSessionID: "native-1", PID: 4194303,
	}
	if err := registry.Register(sessions.SessionMetadata{SessionID: "job-1", JobID: "job-1", PID: 4194303}); err != nil {
		t.Fatalf("register intent: %v", err)
	}
	if err := registry.Register(metadata); err != nil {
		t.Fatalf("register native alias: %v", err)
	}

	cleaner := Cleaner{StateDir: stateDir}
	out, err := cleaner.Clean(context.Background(), &Probe{
		Session:  &models.Session{ID: "job-1", ClaudeSessionID: "native-1"},
		Evidence: Evidence{RegistryDir: "native-1"},
	})
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if !out.RemovedRecovery {
		t.Fatal("Clean did not report recovery removal")
	}
	for _, dir := range []string{"job-1", "native-1"} {
		root := filepath.Join(stateDir, "hooks", "sessions", dir)
		if _, err := os.Stat(filepath.Join(root, "pid.lock")); !os.IsNotExist(err) {
			t.Errorf("%s pid.lock should be removed, stat error = %v", dir, err)
		}
		if _, err := os.Stat(filepath.Join(root, "metadata.json")); err != nil {
			t.Errorf("%s metadata should remain: %v", dir, err)
		}
	}
}
