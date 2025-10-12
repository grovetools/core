package wsnav

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mattsolo1/grove-core/pkg/workspace"
)

// kindAbbreviation returns a single-character string for a workspace kind.
func kindAbbreviation(kind workspace.WorkspaceKind) string {
	switch kind {
	case workspace.KindEcosystemRoot, workspace.KindEcosystemWorktree:
		return "e" // Ecosystem
	case workspace.KindStandaloneProject, workspace.KindEcosystemSubProject, workspace.KindEcosystemWorktreeSubProject:
		return "p" // Project
	case workspace.KindStandaloneProjectWorktree, workspace.KindEcosystemSubProjectWorktree, workspace.KindEcosystemWorktreeSubProjectWorktree:
		return "w" // Worktree
	case workspace.KindNonGroveRepo:
		return "g" // Git Repo
	default:
		return "?"
	}
}

// shortenPath replaces the home directory prefix with a tilde (~).
func shortenPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path // Fallback to original path on error
	}

	if strings.HasPrefix(path, home) {
		return filepath.Join("~", strings.TrimPrefix(path, home))
	}

	return path
}
