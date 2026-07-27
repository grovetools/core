package sessions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// deadPID is far above any live process id, so IsProcessAlive reports false
// without the test having to spawn and kill anything.
const deadPID = 4194303

func sessionsRoot(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root := filepath.Join(os.Getenv("XDG_STATE_HOME"), "grove", "hooks", "sessions")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("creating sessions root: %v", err)
	}
	return root
}

func plantSession(t *testing.T, root, nativeID string, pid int, metadata SessionMetadata) string {
	t.Helper()
	dir := filepath.Join(root, nativeID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("creating session dir: %v", err)
	}
	if pid > 0 {
		if err := os.WriteFile(filepath.Join(dir, "pid.lock"), []byte(strconv.Itoa(pid)), 0o644); err != nil {
			t.Fatalf("writing pid.lock: %v", err)
		}
	}
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatalf("marshalling metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), data, 0o644); err != nil {
		t.Fatalf("writing metadata.json: %v", err)
	}
	return dir
}

// A dead PID means the process is gone. It does not mean the session never
// happened: metadata.json is the only index binding a Flow job to its native
// session and transcript, and consumers (completion evidence, transcript
// resolution, archival) read it long after the process exits. Reaping used to
// RemoveAll the directory, which is how a job's transcript became unreachable
// while the agent that wrote it was still running.
func TestRecoverSessionsReapsPIDLockButKeepsMetadata(t *testing.T) {
	root := sessionsRoot(t)
	dir := plantSession(t, root, "native-1", deadPID, SessionMetadata{
		SessionID:       "job-1",
		JobID:           "job-1",
		ClaudeSessionID: "native-1",
		TranscriptPath:  "/plans/p/.artifacts/job-1/sessions/x.jsonl",
		JobFilePath:     "/plans/p/1-job.md",
	})

	if _, err := RecoverSessions(); err != nil {
		t.Fatalf("RecoverSessions: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "pid.lock")); !os.IsNotExist(err) {
		t.Fatalf("expected pid.lock to be removed, stat error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatalf("metadata.json must survive a dead-PID sweep: %v", err)
	}
	var recovered SessionMetadata
	if err := json.Unmarshal(data, &recovered); err != nil {
		t.Fatalf("surviving metadata.json must still parse: %v", err)
	}
	if recovered.TranscriptPath == "" || recovered.JobID != "job-1" {
		t.Fatalf("surviving metadata lost its job→transcript binding: %+v", recovered)
	}
}

// Without pid.lock the session is no longer live, so it must not come back as a
// running session on the next recovery pass.
func TestRecoverSessionsSkipsReapedSession(t *testing.T) {
	root := sessionsRoot(t)
	plantSession(t, root, "native-1", 0, SessionMetadata{SessionID: "job-1", ClaudeSessionID: "native-1"})

	recovered, err := RecoverSessions()
	if err != nil {
		t.Fatalf("RecoverSessions: %v", err)
	}
	if len(recovered) != 0 {
		t.Fatalf("expected no live sessions, got %d", len(recovered))
	}
}

func TestPurgeStaleSessionsRespectsRetentionAndLiveness(t *testing.T) {
	root := sessionsRoot(t)

	stale := plantSession(t, root, "stale", 0, SessionMetadata{SessionID: "job-stale"})
	fresh := plantSession(t, root, "fresh", 0, SessionMetadata{SessionID: "job-fresh"})
	claimed := plantSession(t, root, "claimed", deadPID, SessionMetadata{SessionID: "job-claimed"})

	old := time.Now().Add(-72 * time.Hour)
	for _, name := range []string{"metadata.json"} {
		if err := os.Chtimes(filepath.Join(stale, name), old, old); err != nil {
			t.Fatalf("chtimes: %v", err)
		}
	}
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatalf("chtimes dir: %v", err)
	}

	purged, err := PurgeStaleSessions(24 * time.Hour)
	if err != nil {
		t.Fatalf("PurgeStaleSessions: %v", err)
	}
	if purged != 1 {
		t.Fatalf("expected exactly the stale record to be purged, purged=%d", purged)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale record should be gone, stat error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(fresh, "metadata.json")); err != nil {
		t.Fatalf("record inside the retention window must survive: %v", err)
	}
	// A pid.lock means something still claims this session is live. Judging
	// that claim is the liveness sweep's job, not the GC's.
	if _, err := os.Stat(filepath.Join(claimed, "metadata.json")); err != nil {
		t.Fatalf("record with a pid.lock must never be purged by the GC: %v", err)
	}
}

func TestPurgeRemovesEverythingForExplicitGC(t *testing.T) {
	root := sessionsRoot(t)
	dir := plantSession(t, root, "native-1", 0, SessionMetadata{SessionID: "job-1"})

	registry, err := NewFileSystemRegistry()
	if err != nil {
		t.Fatalf("NewFileSystemRegistry: %v", err)
	}
	if err := registry.Purge("native-1"); err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("Purge must remove the whole record, stat error = %v", err)
	}
}
