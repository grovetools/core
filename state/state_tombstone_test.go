package state

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/grovetools/core/pkg/worktreeregistry"
	"github.com/grovetools/core/util/pathutil"
)

// A finished worktree's registry entry is history, and tombstoning strips its
// SessionState on purpose. The registry half of state.Save's dual-write must
// not put an ephemeral session payload back into a record that now outlives
// the worktree; the in-worktree .grove/state.yml half still writes, since that
// file dies with the worktree.
func TestSaveDoesNotResurrectSessionStateOnATombstone(t *testing.T) {
	t.Setenv("GROVE_HOME", t.TempDir())

	base := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(base); err == nil {
		base = resolved
	}
	// Legacy worktree layout: <repo>/.grove-worktrees/<name> is recognized as
	// a worktree root without needing an XDG base.
	root := filepath.Join(base, "repo", ".grove-worktrees", "feature")
	if err := os.MkdirAll(filepath.Join(root, ".grove"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "grove.toml"), []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := worktreeregistry.Save(&worktreeregistry.Entry{AbsPath: root, Plan: "p"}); err != nil {
		t.Fatal(err)
	}
	id := pathutil.WorktreeID(root)
	if _, err := worktreeregistry.Tombstone(id, nil); err != nil {
		t.Fatal(err)
	}

	if err := Set(root, "flow.active_plan", "should-not-fossilize"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	entry, err := worktreeregistry.Load(id)
	if err != nil {
		t.Fatalf("tombstone disappeared: %v", err)
	}
	if !entry.IsFinished() {
		t.Error("the entry stopped being a tombstone")
	}
	if len(entry.SessionState) != 0 {
		t.Errorf("session state was resurrected onto a tombstone: %v", entry.SessionState)
	}

	// The in-worktree file still got written — only the registry half is skipped.
	if _, err := os.Stat(filepath.Join(root, ".grove", "state.yml")); err != nil {
		t.Errorf(".grove/state.yml should still be written: %v", err)
	}
}
