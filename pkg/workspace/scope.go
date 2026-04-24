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
// We key the scope to the **nearest ecosystem boundary**, which is either:
//   - the main ecosystem root, or
//   - an ecosystem worktree under .grove-worktrees/
//
// whichever contains the given directory most closely. This lets
// co-development worktrees (e.g. grovetools/.grove-worktrees/treemux-pt7)
// run their own daemon independently of the main ecosystem checkout —
// each one a fully isolated development environment.
//
// Resolution order:
//  1. workspace.GetProjectByPath(dir) classifies the directory.
//     - If the node IS an ecosystem (EcosystemRoot or EcosystemWorktree),
//     use its own Path — the worktree is the scope.
//     - Otherwise prefer ParentEcosystemPath (the immediate ecosystem or
//     ecosystem-worktree containing this subproject).
//     - Fall back to ParentProjectPath (standalone project worktrees share
//     the main repo daemon).
//     - Otherwise node.Path (standalone project / NonGroveRepo).
//  2. git.GetGitRoot(dir) for plain repos not yet visible to workspace
//     discovery.
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
		return ""
	}

	if node, err := GetProjectByPath(absDir); err == nil && node != nil {
		// If the node itself is an ecosystem (main or worktree), the
		// worktree IS the scope — do NOT bubble up to the parent eco.
		if node.IsEcosystem() {
			return node.Path
		}
		// Otherwise use the nearest containing ecosystem boundary.
		if node.ParentEcosystemPath != "" {
			return node.ParentEcosystemPath
		}
		// Standalone project worktrees share their main repo's scope.
		if node.ParentProjectPath != "" {
			return node.ParentProjectPath
		}
		return node.Path
	}

	if root, err := git.GetGitRoot(absDir); err == nil && root != "" {
		return root
	}

	// No ecosystem and no git context — return empty so callers know to
	// fall back to the global (unscoped) daemon rather than treating the
	// raw absolute path as a "scope". Avoids spawning ad-hoc per-dir
	// daemons when the user runs tools from unrelated directories.
	return ""
}
