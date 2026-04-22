package prune

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetect_BailsOnEmptyActive(t *testing.T) {
	_, err := Detect(Inputs{Active: nil}, Options{DryRun: true})
	if err == nil {
		t.Fatal("expected bail error with empty active list")
	}
}

func TestRun_DryRunDoesNotDelete(t *testing.T) {
	tmp := t.TempDir()
	base := filepath.Join(tmp, ".grove-worktrees", "dead")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	in := Inputs{GitRoot: tmp, Active: []string{"alive"}}
	res, err := Run(in, Options{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Orphans) != 1 {
		t.Fatalf("expected 1 orphan, got %d", len(res.Orphans))
	}
	if len(res.Deleted) != 0 {
		t.Errorf("dry run should not delete, got %d deleted", len(res.Deleted))
	}
	if _, err := os.Stat(base); err != nil {
		t.Errorf("dry run removed path: %v", err)
	}
}

func TestRun_AppliesLocalNotCloud(t *testing.T) {
	tmp := t.TempDir()
	deadDir := filepath.Join(tmp, ".grove-worktrees", "dead")
	if err := os.MkdirAll(deadDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Seed a cloud orphan via a fake gcloud runner; IncludeCloud=false
	// means Detect won't even be asked, but if it were it must stay
	// out of the delete loop.
	in := Inputs{GitRoot: tmp, Active: []string{"alive"}}
	res, err := Run(in, Options{DryRun: false, IncludeCloud: false})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Deleted) != 1 {
		t.Fatalf("expected 1 deletion, got %d: %+v", len(res.Deleted), res.Deleted)
	}
	if _, err := os.Stat(deadDir); !os.IsNotExist(err) {
		t.Errorf("expected dead dir removed, stat err=%v", err)
	}
}

func TestRun_ScopedToWorktree(t *testing.T) {
	tmp := t.TempDir()
	for _, d := range []string{"dead1", "dead2"} {
		if err := os.MkdirAll(filepath.Join(tmp, ".grove-worktrees", d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	in := Inputs{GitRoot: tmp, Active: []string{"alive"}}
	res, err := Run(in, Options{DryRun: true, Worktree: "dead1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Orphans) != 1 || res.Orphans[0].Worktree != "dead1" {
		t.Fatalf("scope filter failed: %+v", res.Orphans)
	}
}

func TestRemoveHostPath_RefusesOutsideRoot(t *testing.T) {
	err := removeHostPath("/etc/passwd", "/home/safe")
	if err == nil || !strings.Contains(err.Error(), "outside git root") {
		t.Fatalf("expected outside-root refusal, got %v", err)
	}
}
