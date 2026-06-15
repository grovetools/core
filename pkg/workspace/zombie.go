package workspace

import (
	"os"
	"path/filepath"
)

// IsZombieWorktree checks if the given path is inside a deleted git worktree.
// A worktree is considered "zombie" if it's inside a worktree base directory
// but the .git reference (which links to the main repo) is missing.
//
// This is used to prevent recreating files (like .grove/rules or logs) in deleted
// worktrees, which would cause "zombie" directories to reappear after cleanup.
//
// Layout note: under the unified-container layout a worktree is a CONTAINER
// (<base>/<identifier>/<name>) holding one or more repo checkouts as
// <container>/<repo>/ subdirs. The container itself carries a synthetic
// grove.toml but NO .git file — the .git reference lives in each child repo.
// A path inside such a container (the container root or a child repo) must NOT
// be flagged as a zombie while any child repo still holds its .git reference.
func IsZombieWorktree(path string) bool {
	// Only check paths that are inside a worktree location
	if !IsWorktreePath(path) {
		return false
	}

	// Extract the worktree root from the path
	// e.g., /path/to/repo/.grove-worktrees/my-feature/.grove/rules
	//       -> /path/to/repo/.grove-worktrees/my-feature
	worktreeRoot, ok := worktreeRootForPath(path)
	if !ok {
		return false
	}

	// A live worktree (legacy single-repo or a container child) has a .git
	// reference at its root. The .git is a FILE for linked worktrees and a
	// DIRECTORY for a primary checkout; either means the worktree is live.
	if hasGitReference(worktreeRoot) {
		return false
	}

	// No .git at the root: this is a unified container whose .git references
	// live one level down, in <container>/<repo>/. It is only a zombie when NO
	// child repo retains its .git reference — i.e. the underlying git worktrees
	// were all deregistered/deleted.
	if containerHasLiveChild(worktreeRoot) {
		return false
	}

	return true // Zombie detected - no live .git reference at root or in any child
}

// hasGitReference reports whether dir is a live git worktree/repo, i.e. it has
// a .git entry (a FILE for linked worktrees, a DIRECTORY for a primary repo).
func hasGitReference(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// containerHasLiveChild reports whether any direct subdirectory of container is
// a live git worktree/repo. This distinguishes a unified-container worktree
// (whose .git references live one level down, in <container>/<repo>/) from a
// genuinely deleted worktree whose checkouts are all gone.
func containerHasLiveChild(container string) bool {
	entries, err := os.ReadDir(container)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if hasGitReference(filepath.Join(container, entry.Name())) {
			return true
		}
	}
	return false
}

// IsZombieWorktreePath is an alias for IsZombieWorktree for clarity when checking
// arbitrary paths (not necessarily the current working directory).
var IsZombieWorktreePath = IsZombieWorktree

// IsZombieWorktreeCwd checks if the current working directory is inside a zombie worktree.
func IsZombieWorktreeCwd() bool {
	cwd, err := os.Getwd()
	if err != nil {
		return false
	}
	return IsZombieWorktree(cwd)
}
