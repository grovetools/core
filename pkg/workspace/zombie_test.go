package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/grovetools/core/pkg/paths"
)

func TestIsZombieWorktree_Legacy(t *testing.T) {
	sandboxXDG(t)

	repo := t.TempDir()

	// Live worktree: .git file present → not a zombie.
	live := filepath.Join(repo, ".grove-worktrees", "live")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, ".git"), []byte("gitdir: ../../.git/worktrees/live"), 0o644); err != nil {
		t.Fatal(err)
	}
	if IsZombieWorktree(live) {
		t.Error("live legacy worktree flagged as zombie")
	}

	// Deleted worktree: dir exists but .git file is gone → zombie, also for
	// paths deep inside it.
	dead := filepath.Join(repo, ".grove-worktrees", "dead")
	deep := filepath.Join(dead, ".grove", "rules")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if !IsZombieWorktree(dead) {
		t.Error("deleted legacy worktree not flagged as zombie")
	}
	if !IsZombieWorktree(deep) {
		t.Error("path inside deleted legacy worktree not flagged as zombie")
	}

	// Non-worktree paths are never zombies.
	if IsZombieWorktree(repo) {
		t.Error("repo root flagged as zombie")
	}
}

func TestIsZombieWorktree_XDG(t *testing.T) {
	sandboxXDG(t)

	repo := t.TempDir()

	// Live XDG worktree: .git file present → not a zombie.
	live := ResolveNewWorktreePath(repo, "live", true)
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	gitFile := "gitdir: " + filepath.Join(repo, ".git", "worktrees", "live")
	if err := os.WriteFile(filepath.Join(live, ".git"), []byte(gitFile), 0o644); err != nil {
		t.Fatal(err)
	}
	if IsZombieWorktree(live) {
		t.Error("live XDG worktree flagged as zombie")
	}

	// Deleted XDG worktree: dir (or a recreated path inside it) exists but
	// .git is gone → zombie.
	dead := ResolveNewWorktreePath(repo, "dead", true)
	deep := filepath.Join(dead, ".grove", "rules")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if !IsZombieWorktree(dead) {
		t.Error("deleted XDG worktree not flagged as zombie")
	}
	if !IsZombieWorktree(deep) {
		t.Error("path inside deleted XDG worktree not flagged as zombie")
	}

	// The XDG base and identifier dirs themselves are containers — never
	// zombies, no matter what they contain.
	if IsZombieWorktree(paths.WorktreesDir()) {
		t.Error("XDG worktree base flagged as zombie")
	}
	if IsZombieWorktree(filepath.Dir(dead)) {
		t.Error("XDG identifier dir flagged as zombie")
	}
}

func TestIsZombieWorktree_CxCarveOut(t *testing.T) {
	sandboxXDG(t)

	// cx commit-keyed checkouts contain the legacy literal but are not
	// grove worktrees — even without a .git file they are never zombies.
	checkout := filepath.Join(paths.DataDir(), "cx", "repos", "my-repo", ".grove-worktrees", "abc123def456")
	if err := os.MkdirAll(checkout, 0o755); err != nil {
		t.Fatal(err)
	}
	if IsZombieWorktree(checkout) {
		t.Error("cx checkout flagged as zombie")
	}
}
