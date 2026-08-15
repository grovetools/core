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

func TestRecoverSessionsClassifiesMissingAndUnreadablePIDLocks(t *testing.T) {
	root := sessionsRoot(t)
	missing := plantSession(t, root, "missing-lock", 0, SessionMetadata{SessionID: "job-missing", Status: "running"})
	malformed := plantSession(t, root, "malformed-lock", deadPID, SessionMetadata{SessionID: "job-malformed", Status: "pending_user"})
	if err := os.WriteFile(filepath.Join(malformed, "pid.lock"), []byte("not-a-pid"), 0o644); err != nil {
		t.Fatalf("writing malformed lock: %v", err)
	}
	terminal := plantSession(t, root, "terminal", 0, SessionMetadata{SessionID: "job-terminal", Status: "completed"})

	recovered, err := RecoverSessions()
	if err != nil {
		t.Fatalf("RecoverSessions: %v", err)
	}
	if len(recovered) != 0 {
		t.Fatalf("recovered %d sessions, want none", len(recovered))
	}
	for dir, want := range map[string]string{missing: "interrupted", malformed: "interrupted", terminal: "completed"} {
		data, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
		if err != nil {
			t.Fatalf("reading metadata: %v", err)
		}
		var metadata SessionMetadata
		if err := json.Unmarshal(data, &metadata); err != nil {
			t.Fatalf("decoding metadata: %v", err)
		}
		if metadata.Status != want {
			t.Errorf("%s status = %q, want %q", filepath.Base(dir), metadata.Status, want)
		}
	}
}

func TestRecoverSessionsForScopeDoesNotClassifyForeignMissingLock(t *testing.T) {
	root := sessionsRoot(t)
	foreign := plantSession(t, root, "foreign", 0, SessionMetadata{
		SessionID: "job-foreign", Status: "running", Scope: "/other/scope",
	})

	if _, err := RecoverSessionsForScope("/owned/scope"); err != nil {
		t.Fatalf("RecoverSessionsForScope: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(foreign, "metadata.json"))
	if err != nil {
		t.Fatalf("reading metadata: %v", err)
	}
	var metadata SessionMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		t.Fatalf("decoding metadata: %v", err)
	}
	if metadata.Status != "running" {
		t.Fatalf("foreign status = %q, want running", metadata.Status)
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

func TestPurgeStaleSessionsWithCorroboration(t *testing.T) {
	root := sessionsRoot(t)
	old := time.Now().Add(-72 * time.Hour)

	deadTerminal := plantSession(t, root, "dead-terminal", deadPID, SessionMetadata{
		SessionID: "job-terminal", JobID: "job-terminal", ClaudeSessionID: "dead-terminal",
	})
	deadActive := plantSession(t, root, "dead-active", deadPID, SessionMetadata{
		SessionID: "job-active", JobID: "job-active", ClaudeSessionID: "dead-active",
	})
	alive := plantSession(t, root, "alive", os.Getpid(), SessionMetadata{
		SessionID: "job-alive", JobID: "job-alive", ClaudeSessionID: "alive",
	})
	for _, dir := range []string{deadTerminal, deadActive, alive} {
		for _, name := range []string{"metadata.json", "pid.lock"} {
			if err := os.Chtimes(filepath.Join(dir, name), old, old); err != nil {
				t.Fatalf("chtimes %s: %v", name, err)
			}
		}
		if err := os.Chtimes(dir, old, old); err != nil {
			t.Fatalf("chtimes dir: %v", err)
		}
	}

	purged, err := PurgeStaleSessionsWithCorroboration(24*time.Hour, func(_ string, metadata SessionMetadata) bool {
		return metadata.JobID == "job-terminal" // absent/terminal daemon row
	})
	if err != nil {
		t.Fatalf("PurgeStaleSessionsWithCorroboration: %v", err)
	}
	if purged != 1 {
		t.Fatalf("purged = %d, want 1", purged)
	}
	if _, err := os.Stat(deadTerminal); !os.IsNotExist(err) {
		t.Fatalf("corroborated dead record should be purged, stat error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(deadActive, "pid.lock")); err != nil {
		t.Fatalf("dead lock with active daemon row must remain: %v", err)
	}
	if _, err := os.Stat(filepath.Join(alive, "pid.lock")); err != nil {
		t.Fatalf("live lock must remain regardless of corroboration: %v", err)
	}
}

func TestPurgeCorroboratedFreshDeadLockClearsRecoveryButKeepsMetadata(t *testing.T) {
	root := sessionsRoot(t)
	dir := plantSession(t, root, "native-fresh", deadPID, SessionMetadata{
		SessionID: "job-fresh", JobID: "job-fresh", ClaudeSessionID: "native-fresh",
	})

	purged, err := PurgeStaleSessionsWithCorroboration(24*time.Hour, func(_ string, _ SessionMetadata) bool { return true })
	if err != nil {
		t.Fatalf("PurgeStaleSessionsWithCorroboration: %v", err)
	}
	if purged != 0 {
		t.Fatalf("fresh record purged = %d, want 0", purged)
	}
	if _, err := os.Stat(filepath.Join(dir, "pid.lock")); !os.IsNotExist(err) {
		t.Fatalf("dead corroborated pid.lock should be removed, stat error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "metadata.json")); err != nil {
		t.Fatalf("fresh metadata should remain: %v", err)
	}
}

func TestRemoveRecoveryFilesForJobClearsLegacyAliases(t *testing.T) {
	root := sessionsRoot(t)
	metadata := SessionMetadata{SessionID: "job-1", JobID: "job-1", ClaudeSessionID: "native-1"}
	jobDir := plantSession(t, root, "job-1", deadPID, metadata)
	nativeDir := plantSession(t, root, "native-1", deadPID, metadata)
	otherDir := plantSession(t, root, "other", deadPID, SessionMetadata{SessionID: "job-other"})

	registry, err := NewFileSystemRegistry()
	if err != nil {
		t.Fatalf("NewFileSystemRegistry: %v", err)
	}
	removed, err := registry.RemoveRecoveryFilesForJob("job-1", "native-1")
	if err != nil {
		t.Fatalf("RemoveRecoveryFilesForJob: %v", err)
	}
	if removed != 2 {
		t.Fatalf("removed = %d, want 2", removed)
	}
	for _, dir := range []string{jobDir, nativeDir} {
		if _, err := os.Stat(filepath.Join(dir, "pid.lock")); !os.IsNotExist(err) {
			t.Errorf("alias lock should be gone in %s: %v", dir, err)
		}
		if _, err := os.Stat(filepath.Join(dir, "metadata.json")); err != nil {
			t.Errorf("alias metadata should remain in %s: %v", dir, err)
		}
	}
	if _, err := os.Stat(filepath.Join(otherDir, "pid.lock")); err != nil {
		t.Fatalf("unrelated lock should remain: %v", err)
	}
}

func TestRemoveRecoveryFilesForJobInScopePreservesOtherScope(t *testing.T) {
	root := sessionsRoot(t)
	owned := plantSession(t, root, "owned-native", deadPID, SessionMetadata{
		SessionID: "shared-job", JobID: "shared-job", ClaudeSessionID: "owned-native", Scope: "/owned",
	})
	foreign := plantSession(t, root, "foreign-native", deadPID, SessionMetadata{
		SessionID: "shared-job", JobID: "shared-job", ClaudeSessionID: "foreign-native", Scope: "/foreign",
	})
	registry, err := NewFileSystemRegistry()
	if err != nil {
		t.Fatalf("NewFileSystemRegistry: %v", err)
	}
	removed, err := registry.RemoveRecoveryFilesForJobInScope("shared-job", "owned-native", "/owned")
	if err != nil {
		t.Fatalf("RemoveRecoveryFilesForJobInScope: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if _, err := os.Stat(filepath.Join(owned, "pid.lock")); !os.IsNotExist(err) {
		t.Fatalf("owned lock should be gone, stat error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(foreign, "pid.lock")); err != nil {
		t.Fatalf("foreign lock should remain: %v", err)
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
