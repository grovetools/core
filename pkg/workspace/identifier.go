package workspace

import (
	"fmt"
	"path/filepath"

	"github.com/mattsolo1/grove-core/util/sanitize"
)

// Identifier generates a unique, sanitized identifier for a project,
// suitable for use as a tmux session name or other unique identifier.
// It creates namespaced identifiers for projects within ecosystem worktrees.
func (p *ProjectInfo) Identifier() string {
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
		return fmt.Sprintf("%s_%s", ecoName, s(p.Name))

	case KindEcosystemWorktreeSubProject:
		// e.g., my-ecosystem_eco-feature_sub-project
		// The parent ecosystem path points to the ecosystem worktree
		worktreeName := s(filepath.Base(p.ParentEcosystemPath))
		// Get the grandparent ecosystem name
		grandParentPath := filepath.Dir(filepath.Dir(p.ParentEcosystemPath))
		grandParentName := s(filepath.Base(grandParentPath))
		return fmt.Sprintf("%s_%s_%s", grandParentName, worktreeName, s(p.Name))

	case KindEcosystemWorktreeSubProjectWorktree:
		// e.g., my-ecosystem_eco-feature_sub-project_sub-feature
		// The parent ecosystem path points to the ecosystem worktree
		ecoWorktreeName := s(filepath.Base(p.ParentEcosystemPath))
		// Get the root ecosystem name
		rootEcoPath := filepath.Dir(filepath.Dir(p.ParentEcosystemPath))
		rootEcoName := s(filepath.Base(rootEcoPath))
		return fmt.Sprintf("%s_%s_%s", rootEcoName, ecoWorktreeName, s(p.Name))

	case KindStandaloneProjectWorktree, KindEcosystemSubProjectWorktree:
		// e.g., my-project_feature-branch
		parentName := s(filepath.Base(p.ParentProjectPath))
		return fmt.Sprintf("%s_%s", parentName, s(p.Name))

	case KindEcosystemSubProject:
		// e.g., my-ecosystem_sub-project
		return fmt.Sprintf("%s_%s", ecoName, s(p.Name))

	case KindStandaloneProject, KindEcosystemRoot, KindNonGroveRepo:
		// e.g., my-project
		return s(p.Name)

	default:
		return s(p.Name)
	}
}
