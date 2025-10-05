package workspace

import (
	"path/filepath"
	"strings"
)

// TransformToProjectInfo converts a hierarchical DiscoveryResult into a flat list
// of ProjectInfo items suitable for display in UIs.
func TransformToProjectInfo(result *DiscoveryResult) []*ProjectInfo {
	var projects []*ProjectInfo

	// First, add ecosystems themselves as discoverable projects
	for _, eco := range result.Ecosystems {
		projects = append(projects, &ProjectInfo{
			Name:        eco.Name,
			Path:        eco.Path,
			IsWorktree:  false,
			IsEcosystem: true,
		})
	}

	// Then process all discovered projects
	for _, proj := range result.Projects {
		// Check if this is an ecosystem worktree (has ParentEcosystemPath and is in .grove-worktrees)
		isEcosystemWorktree := proj.ParentEcosystemPath != "" &&
			strings.Contains(proj.Path, filepath.Join(proj.ParentEcosystemPath, ".grove-worktrees"))

		if isEcosystemWorktree {
			// Ecosystem worktrees should be treated as worktrees of the ecosystem
			// Use the directory name as the display name
			wtName := filepath.Base(proj.Path)
			projects = append(projects, &ProjectInfo{
				Name:                wtName,
				Path:                proj.Path,
				ParentPath:          proj.ParentEcosystemPath,
				IsWorktree:          true,
				ParentEcosystemPath: proj.ParentEcosystemPath,
				IsEcosystem:         true, // Ecosystem worktrees are also ecosystems
			})
		} else {
			// Regular projects (including those within ecosystems)
			projects = append(projects, &ProjectInfo{
				Name:                proj.Name,
				Path:                proj.Path,
				IsWorktree:          false,
				ParentEcosystemPath: proj.ParentEcosystemPath,
			})

			// Add all associated Worktree Workspaces
			for _, ws := range proj.Workspaces {
				if ws.Type == WorkspaceTypeWorktree {
					projects = append(projects, &ProjectInfo{
						Name:                ws.Name,
						Path:                ws.Path,
						ParentPath:          ws.ParentProjectPath,
						IsWorktree:          true,
						ParentEcosystemPath: proj.ParentEcosystemPath,
					})
				}
			}
		}
	}

	// Also include Non-Grove Directories
	for _, path := range result.NonGroveDirectories {
		projects = append(projects, &ProjectInfo{
			Name:       filepath.Base(path),
			Path:       path,
			IsWorktree: false,
		})
	}

	return projects
}
