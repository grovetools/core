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
		// Extract worktree name if this project is inside .grove-worktrees
		worktreeName := ""
		if proj.ParentEcosystemPath != "" && strings.Contains(proj.Path, ".grove-worktrees") {
			// Extract worktree name from path like: /eco/.grove-worktrees/my-branch/sub-project
			relPath, err := filepath.Rel(proj.ParentEcosystemPath, proj.Path)
			if err == nil {
				parts := strings.Split(relPath, string(filepath.Separator))
				if len(parts) >= 2 && parts[0] == ".grove-worktrees" {
					worktreeName = parts[1]
				}
			}
		}

		// Check if this is an ecosystem worktree (has ParentEcosystemPath and IS the worktree directory itself)
		// A worktree directory is: /eco/.grove-worktrees/branch-name
		// A sub-project inside worktree is: /eco/.grove-worktrees/branch-name/sub-project
		isEcosystemWorktree := false
		if proj.ParentEcosystemPath != "" && strings.Contains(proj.Path, ".grove-worktrees") {
			relPath, err := filepath.Rel(proj.ParentEcosystemPath, proj.Path)
			if err == nil {
				parts := strings.Split(relPath, string(filepath.Separator))
				// It's a worktree if the path is exactly: .grove-worktrees/worktree-name
				isEcosystemWorktree = len(parts) == 2 && parts[0] == ".grove-worktrees"
			}
		}

		if isEcosystemWorktree {
			// Ecosystem worktrees should be treated as worktrees of the ecosystem
			// Use the directory name as the display name
			wtName := filepath.Base(proj.Path)
			projects = append(projects, &ProjectInfo{
				Name:                wtName,
				Path:                proj.Path,
				ParentPath:          proj.ParentEcosystemPath,
				IsWorktree:          true,
				WorktreeName:        worktreeName,
				ParentEcosystemPath: proj.ParentEcosystemPath,
				IsEcosystem:         true, // Ecosystem worktrees are also ecosystems
			})
		} else {
			// Regular projects (including those within ecosystems)
			projects = append(projects, &ProjectInfo{
				Name:                proj.Name,
				Path:                proj.Path,
				IsWorktree:          false,
				WorktreeName:        worktreeName,
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
