package workspace

import (
	"os"
	"path/filepath"
	"strings"
)

// IsZombieWorktree checks if the given path is inside a deleted git worktree.
// A worktree is considered "zombie" if it's inside a .grove-worktrees directory
// but the .git file (which links to the main repo) is missing.
//
// This is used to prevent recreating files (like .grove/rules or logs) in deleted
// worktrees, which would cause "zombie" directories to reappear after cleanup.
//
// This function uses the same detection logic as zombieAwareWriter in the logging
// package, ensuring consistent behavior across the grove ecosystem.
func IsZombieWorktree(path string) bool {
	// Only check paths that are inside .grove-worktrees
	if !strings.Contains(path, ".grove-worktrees") {
		return false
	}

	// Extract worktree root from the path
	// e.g., /path/to/repo/.grove-worktrees/my-feature/.grove/rules
	//       -> /path/to/repo/.grove-worktrees/my-feature
	parts := strings.Split(path, ".grove-worktrees")
	if len(parts) < 2 {
		return false
	}

	gitRoot := parts[0]
	remaining := parts[1]

	// Extract the worktree name (first path component after .grove-worktrees/)
	remaining = strings.TrimPrefix(remaining, string(filepath.Separator))
	worktreeNameParts := strings.SplitN(remaining, string(filepath.Separator), 2)
	if len(worktreeNameParts) == 0 || worktreeNameParts[0] == "" {
		return false
	}
	worktreeName := worktreeNameParts[0]

	// Construct the worktree root path
	worktreeRoot := filepath.Join(gitRoot, ".grove-worktrees", worktreeName)

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
