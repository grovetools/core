package workspace

import (
	"os"
	"path/filepath"
)

// IsZombieWorktree checks if the given path is inside a deleted git worktree.
// A worktree is considered "zombie" if it's inside a worktree base directory
// but the .git file (which links to the main repo) is missing.
//
// This is used to prevent recreating files (like .grove/rules or logs) in deleted
// worktrees, which would cause "zombie" directories to reappear after cleanup.
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

	// A valid worktree must have a .git FILE (not directory).
	// The .git file contains a reference to the main repo's .git/worktrees/<name> directory.
	// If this file is missing, the worktree was deleted.
	gitPath := filepath.Join(worktreeRoot, ".git")
	if _, err := os.Stat(gitPath); os.IsNotExist(err) {
		return true // Zombie detected - .git file is missing
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
