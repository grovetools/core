package workspace

import (
	"fmt"
	"path/filepath"

	"github.com/grovetools/core/util/sanitize"
)

// Identifier generates a unique, sanitized identifier for a project.
// The delim parameter controls the separator between components:
// use "_" for tmux session names, ":" for alias/display purposes.
func (p *WorkspaceNode) Identifier(delim string) string {
	// Sanitize helper
	s := func(name string) string {
		return sanitize.SanitizeForTmuxSession(name)
	}

	ecoName := ""
	if p.ParentEcosystemPath != "" {
		ecoName = s(filepath.Base(p.ParentEcosystemPath))
	}

	switch p.Kind {
	case KindEcosystemWorktree:
		// e.g., my-ecosystem_eco-feature
		return fmt.Sprintf("%s%s%s", ecoName, delim, s(p.Name))

	case KindEcosystemWorktreeSubProject:
		// e.g., my-ecosystem_eco-feature_sub-project
		worktreeName := s(filepath.Base(p.ParentEcosystemPath))
		grandParentPath := filepath.Dir(filepath.Dir(p.ParentEcosystemPath))
		grandParentName := s(filepath.Base(grandParentPath))
		return fmt.Sprintf("%s%s%s%s%s", grandParentName, delim, worktreeName, delim, s(p.Name))

	case KindEcosystemWorktreeSubProjectWorktree:
		// e.g., my-ecosystem_eco-feature_sub-project
		rootEcoName := s(filepath.Base(p.RootEcosystemPath))
		ecoWorktreeName := s(filepath.Base(p.ParentEcosystemPath))
		subProjectName := s(p.Name)
		return fmt.Sprintf("%s%s%s%s%s", rootEcoName, delim, ecoWorktreeName, delim, subProjectName)

	case KindStandaloneProjectWorktree:
		// e.g., my-project_feature-branch
		parentName := s(filepath.Base(p.ParentProjectPath))
		return fmt.Sprintf("%s%s%s", parentName, delim, s(p.Name))

	case KindEcosystemSubProjectWorktree:
		// e.g., my-ecosystem_sub-project_feature-branch
		parentName := s(filepath.Base(p.ParentProjectPath))
		return fmt.Sprintf("%s%s%s%s%s", ecoName, delim, parentName, delim, s(p.Name))

	case KindEcosystemSubProject:
		// e.g., my-ecosystem_sub-project
		return fmt.Sprintf("%s%s%s", ecoName, delim, s(p.Name))

	case KindStandaloneProject, KindEcosystemRoot, KindNonGroveRepo:
		return s(p.Name)

	default:
		return s(p.Name)
	}
}
