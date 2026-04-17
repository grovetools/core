package workspace

import (
	"os"
	"path/filepath"

	"github.com/grovetools/core/git"
)

// ResolveScope determines the daemon scope for a given directory.
//
// The scope is the isolation boundary keyed to a single daemon instance.
// Tools and daemons sharing a scope share state; tools with different scopes
// target different daemon processes.
//
// Resolution order:
//  1. workspace.GetProjectByPath(dir) classifies the directory against
//     known workspace kinds (ecosystem, ecosystem worktree, subproject,
//     standalone project, etc). Prefer RootEcosystemPath (any ecosystem
//     context), then ParentProjectPath (standalone project worktrees share
//     the main repo daemon), else node.Path (standalone project /
//     non-grove repo).
//  2. git.GetGitRoot(dir) for repos not yet visible to workspace discovery.
//  3. Absolute(dir) as the ultimate fallback.
//
// An empty dir defaults to os.Getwd().
func ResolveScope(dir string) string {
	if dir == "" {
		if cwd, err := os.Getwd(); err == nil {
			dir = cwd
		}
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}

	if node, err := GetProjectByPath(absDir); err == nil && node != nil {
		if node.RootEcosystemPath != "" {
			return node.RootEcosystemPath
		}
		if node.ParentProjectPath != "" {
			return node.ParentProjectPath
		}
		return node.Path
	}

	if root, err := git.GetGitRoot(absDir); err == nil && root != "" {
		return root
	}

	return absDir
}
