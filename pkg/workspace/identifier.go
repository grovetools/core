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

	// Case 1: Project is inside an ecosystem's worktree directory.
	// Uses the WorktreeName field from the data model.
	if p.ParentEcosystemPath != "" && p.WorktreeName != "" {
		ecoName := s(filepath.Base(p.ParentEcosystemPath))
		worktreeName := s(p.WorktreeName)

		// Check if this project *is* the worktree directory itself (an ecosystem worktree)
		if p.IsWorktree && p.IsEcosystem {
			return fmt.Sprintf("%s_%s", ecoName, worktreeName)
		}

		// It's a sub-project within the worktree
		projectName := s(p.Name)
		return fmt.Sprintf("%s_%s_%s", ecoName, worktreeName, projectName)
	}

	// Case 2: Project is a worktree, but not part of an ecosystem.
	if p.IsWorktree && p.ParentPath != "" {
		parentName := s(filepath.Base(p.ParentPath))
		projectName := s(p.Name)
		return fmt.Sprintf("%s_%s", parentName, projectName)
	}

	// Case 3: A sub-project of an ecosystem (e.g., a submodule in the main repo).
	if p.ParentEcosystemPath != "" && p.Path != p.ParentEcosystemPath {
		ecoName := s(filepath.Base(p.ParentEcosystemPath))
		projectName := s(p.Name)
		return fmt.Sprintf("%s_%s", ecoName, projectName)
	}

	// Case 4: Standalone project or an ecosystem's main repository.
	return s(p.Name)
}
