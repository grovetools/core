package sessions

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileSystemRegistryAttemptUpgradeInPlace(t *testing.T) {
	registry := &FileSystemRegistry{baseDir: t.TempDir()}
	intent := SessionMetadata{
		AttemptID: "018f-attempt-1", SessionID: "job-1", JobID: "job-1",
		Provider: "pi", Status: "pending", Scope: "/scope", PID: 0,
	}
	if err := registry.Register(intent); err != nil {
		t.Fatalf("register intent: %v", err)
	}
	confirmed := intent
	confirmed.ClaudeSessionID = "native-1"
	confirmed.PID = os.Getpid()
	confirmed.Status = "running"
	confirmed.TranscriptPath = "/tmp/transcript.jsonl"
	if err := registry.Register(confirmed); err != nil {
		t.Fatalf("register confirmation: %v", err)
	}
	enriched := confirmed
	enriched.Type = "interactive_agent"
	enriched.JobTitle = "enriched"
	if err := registry.Register(enriched); err != nil {
		t.Fatalf("register enrichment: %v", err)
	}

	entries, err := os.ReadDir(registry.baseDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != intent.AttemptID {
		t.Fatalf("registry entries = %v, want only %q", entries, intent.AttemptID)
	}
	got, err := registry.FindAttempt(intent.AttemptID)
	if err != nil {
		t.Fatalf("FindAttempt: %v", err)
	}
	if got.JobID != "job-1" || got.ClaudeSessionID != "native-1" || got.Status != "running" || got.JobTitle != "enriched" {
		t.Fatalf("upgraded metadata = %+v", got)
	}
	if _, err := os.Stat(filepath.Join(registry.baseDir, intent.AttemptID, "pid.lock")); err != nil {
		t.Fatalf("pid lock missing: %v", err)
	}
}

func TestFileSystemRegistryLegacyShapesRemainReadable(t *testing.T) {
	registry := &FileSystemRegistry{baseDir: t.TempDir()}
	legacy := []SessionMetadata{
		{SessionID: "job-session-key", JobID: "job-session-key", PID: os.Getpid()},
		{SessionID: "job-native-key", JobID: "job-native-key", ClaudeSessionID: "native-key", PID: os.Getpid()},
	}
	for _, metadata := range legacy {
		if err := registry.Register(metadata); err != nil {
			t.Fatalf("Register(%s): %v", metadata.JobID, err)
		}
		got, err := registry.Find(metadata.JobID)
		if err != nil || got.JobID != metadata.JobID {
			t.Fatalf("Find(%s) = %+v, %v", metadata.JobID, got, err)
		}
	}
	if _, err := os.Stat(filepath.Join(registry.baseDir, "job-session-key", "metadata.json")); err != nil {
		t.Fatalf("session-key legacy shape missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(registry.baseDir, "native-key", "metadata.json")); err != nil {
		t.Fatalf("native-key legacy shape missing: %v", err)
	}
}

func TestFileSystemRegistryExactAttemptUpdateRejectsWrongJob(t *testing.T) {
	registry := &FileSystemRegistry{baseDir: t.TempDir()}
	// A stale legacy alias exists at the reusable job ID, but point lookup may
	// not fall back to it when the requested attempt is absent.
	if err := registry.Register(SessionMetadata{SessionID: "job-2", JobID: "job-2", Status: "completed"}); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.FindAttempt("missing-current-attempt"); err == nil {
		t.Fatal("FindAttempt broad-fell back to a job alias")
	}
	if err := registry.Register(SessionMetadata{AttemptID: "attempt-2", SessionID: "job-2", JobID: "job-2", Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	if err := registry.UpdateStatusForAttempt("other-job", "attempt-2", "completed"); err == nil {
		t.Fatal("wrong-job exact update succeeded")
	}
	got, err := registry.FindAttempt("attempt-2")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "pending" {
		t.Fatalf("status changed to %q after rejected update", got.Status)
	}
}

func TestFileSystemRegistryIsAlive(t *testing.T) {
	registry := &FileSystemRegistry{baseDir: t.TempDir()}

	t.Run("live process", func(t *testing.T) {
		meta := SessionMetadata{
			SessionID: "live-session",
			PID:       os.Getpid(),
		}
		if err := registry.Register(meta); err != nil {
			t.Fatalf("Register failed: %v", err)
		}

		alive, err := registry.IsAlive("live-session")
		if err != nil {
			t.Fatalf("IsAlive returned error: %v", err)
		}
		if !alive {
			t.Errorf("IsAlive = false for the current process (pid %d), want true", os.Getpid())
		}
	})

	t.Run("dead process", func(t *testing.T) {
		meta := SessionMetadata{
			SessionID: "dead-session",
			PID:       99999999, // absurd PID, cannot exist
		}
		if err := registry.Register(meta); err != nil {
			t.Fatalf("Register failed: %v", err)
		}

		alive, err := registry.IsAlive("dead-session")
		if err != nil {
			t.Fatalf("IsAlive returned error: %v", err)
		}
		if alive {
			t.Error("IsAlive = true for an absurd PID, want false")
		}
	})

	t.Run("missing session", func(t *testing.T) {
		alive, err := registry.IsAlive("no-such-session")
		if err != nil {
			t.Fatalf("IsAlive returned error: %v", err)
		}
		if alive {
			t.Error("IsAlive = true for a missing session, want false")
		}
	})
}
