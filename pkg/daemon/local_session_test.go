package daemon

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/grovetools/core/pkg/paths"
	"github.com/grovetools/core/pkg/sessions"
)

func TestLocalClientRegisterSessionIntentPersistsParentJobID(t *testing.T) {
	t.Setenv("GROVE_HOME", t.TempDir())

	client := NewLocalClient()
	if err := client.RegisterSessionIntent(context.Background(), SessionIntent{
		JobID:       "child-job",
		ParentJobID: "parent-job",
		Provider:    "pi",
	}); err != nil {
		t.Fatalf("RegisterSessionIntent: %v", err)
	}

	registry, err := sessions.NewFileSystemRegistry()
	if err != nil {
		t.Fatalf("NewFileSystemRegistry: %v", err)
	}
	got, err := registry.Find("child-job")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if got.ParentJobID != "parent-job" {
		t.Errorf("ParentJobID = %q, want parent-job", got.ParentJobID)
	}
}

func TestLocalClientLifecycleUsesExactAttempt(t *testing.T) {
	t.Setenv("GROVE_HOME", t.TempDir())
	client := NewLocalClient()
	ctx := context.Background()

	for _, attemptID := range []string{"attempt-old", "attempt-current"} {
		if err := client.RegisterSessionIntent(ctx, SessionIntent{
			JobID: "reused-job", AttemptID: attemptID, Provider: "pi", Type: "headless_agent",
		}); err != nil {
			t.Fatalf("RegisterSessionIntent(%s): %v", attemptID, err)
		}
	}
	if err := client.ConfirmSession(ctx, SessionConfirmation{
		JobID: "reused-job", AttemptID: "attempt-current", NativeID: "native-current", PID: os.Getpid(), TranscriptPath: "/tmp/current.jsonl",
	}); err != nil {
		t.Fatalf("ConfirmSession: %v", err)
	}
	// A late status event for the prior attempt must not mutate the current one.
	if err := client.UpdateSessionStatus(ctx, "reused-job", "attempt-old", "interrupted"); err != nil {
		t.Fatalf("late UpdateSessionStatus: %v", err)
	}

	registry, err := sessions.NewFileSystemRegistry()
	if err != nil {
		t.Fatal(err)
	}
	old, err := registry.FindAttempt("attempt-old")
	if err != nil {
		t.Fatal(err)
	}
	current, err := registry.FindAttempt("attempt-current")
	if err != nil {
		t.Fatal(err)
	}
	if old.Status != "interrupted" {
		t.Errorf("old status = %q", old.Status)
	}
	if current.Status != "running" || current.ClaudeSessionID != "native-current" || current.AttemptID != "attempt-current" {
		t.Errorf("current metadata = %+v", current)
	}
	entries, err := os.ReadDir(filepath.Join(paths.StateDir(), "hooks", "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("registry directory count = %d, want one per attempt (2)", len(entries))
	}
}

func TestLocalClientRegisterSessionIntentPersistsScope(t *testing.T) {
	t.Setenv("GROVE_HOME", t.TempDir())
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	t.Setenv("GROVE_SCOPE", cwd)
	wantScope := ResolveClientScope()
	if wantScope == "" {
		t.Fatal("test directory did not resolve to a daemon scope")
	}

	client := NewLocalClient()
	if err := client.RegisterSessionIntent(context.Background(), SessionIntent{JobID: "job-scoped", Provider: "pi"}); err != nil {
		t.Fatalf("RegisterSessionIntent: %v", err)
	}
	registry, err := sessions.NewFileSystemRegistry()
	if err != nil {
		t.Fatalf("NewFileSystemRegistry: %v", err)
	}
	got, err := registry.Find("job-scoped")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if got.Scope != wantScope {
		t.Errorf("Scope = %q, want %q", got.Scope, wantScope)
	}
}
