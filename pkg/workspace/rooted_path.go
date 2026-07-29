package workspace

import (
	"path/filepath"

	"github.com/grovetools/core/util/pathutil"
)

// RootedPath converts an absolute path to cx-style workspace-rooted form
// (<repo>/rel/path), naming a worktree file by its PARENT repo so the result is
// worktree-unrooted: the same source file reads identically whether it was
// touched in the main checkout or in one of its worktrees, which is what makes
// such a list transferable between agents working in different worktrees.
//
// Falls back to the input path whenever the file is not under any known
// workspace, or when the provider is nil (standalone/no-daemon hosts).
func (p *Provider) RootedPath(absPath string) string {
	if p == nil {
		return absPath
	}
	canonical, err := pathutil.NormalizeForLookup(absPath)
	if err != nil {
		return absPath
	}
	node := p.FindByPath(canonical)
	if node == nil {
		return absPath
	}
	name := node.Name
	if node.IsProjectWorktreeChild() {
		if parent := p.FindByPath(node.ParentProjectPath); parent != nil {
			name = parent.Name
		}
	}
	nodePath, err := pathutil.NormalizeForLookup(node.Path)
	if err != nil {
		nodePath = node.Path
	}
	rel, err := filepath.Rel(nodePath, canonical)
	if err != nil {
		return absPath
	}
	if rel == "." {
		return name
	}
	return filepath.Join(name, rel)
}
