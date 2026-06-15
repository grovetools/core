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

func TestIsZombieWorktree_XDGContainer(t *testing.T) {
	sandboxXDG(t)

	repo := t.TempDir()

	// Unified-container worktree: the container (<base>/<identifier>/<name>)
	// holds repo checkouts as <container>/<repo>/ subdirs and carries a
	// synthetic grove.toml but NO .git of its own. The .git reference lives in
	// each child repo.
	container := ResolveNewWorktreePath(repo, "live", true)
	child := filepath.Join(container, "myrepo")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(container, "grove.toml"), []byte("workspaces = [\"*\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitFile := "gitdir: " + filepath.Join(repo, ".git", "worktrees", "live")
	if err := os.WriteFile(filepath.Join(child, ".git"), []byte(gitFile), 0o644); err != nil {
		t.Fatal(err)
	}

	// Neither the container nor the live child repo is a zombie.
	if IsZombieWorktree(container) {
		t.Error("live XDG container flagged as zombie")
	}
	if IsZombieWorktree(child) {
		t.Error("live XDG container child repo flagged as zombie")
	}
	// A deep path inside the live child must also resolve as non-zombie so
	// default context rules can be written into <child>/.grove/rules.
	if IsZombieWorktree(filepath.Join(child, ".grove", "rules")) {
		t.Error("path inside live XDG container child flagged as zombie")
	}

	// Deleted container: child repos exist as bare dirs but their .git
	// references are gone → zombie.
	dead := ResolveNewWorktreePath(repo, "dead", true)
	deadChild := filepath.Join(dead, "myrepo", ".grove")
	if err := os.MkdirAll(deadChild, 0o755); err != nil {
		t.Fatal(err)
	}
	if !IsZombieWorktree(dead) {
		t.Error("deleted XDG container not flagged as zombie")
	}
	if !IsZombieWorktree(filepath.Join(dead, "myrepo")) {
		t.Error("child of deleted XDG container not flagged as zombie")
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
