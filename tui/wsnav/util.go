package wsnav

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mattsolo1/grove-core/pkg/workspace"
)

// kindAbbreviation returns a human-readable string for a workspace kind.
func kindAbbreviation(kind workspace.WorkspaceKind) string {
	switch kind {
	case workspace.KindEcosystemRoot:
		return "Ecosystem"
	case workspace.KindEcosystemWorktree:
		return "Eco Worktree"
	case workspace.KindStandaloneProject:
		return "Project"
	case workspace.KindEcosystemSubProject:
		return "Sub-Project"
	case workspace.KindEcosystemWorktreeSubProject:
		return "Eco WT SubProj"
	case workspace.KindStandaloneProjectWorktree:
		return "Proj Worktree"
	case workspace.KindEcosystemSubProjectWorktree:
		return "SubProj WT"
	case workspace.KindEcosystemWorktreeSubProjectWorktree:
		return "Eco WT Sub WT"
	case workspace.KindNonGroveRepo:
		return "Git Repo"
	default:
		return "Unknown"
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
