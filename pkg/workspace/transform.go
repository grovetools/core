package workspace

import (
	"path/filepath"
	"strings"
)

// determineKind classifies a project or workspace based on its path and context.
func determineKind(path, parentEcosystemPath, parentProjectPath string, isWorktree bool, isEcosystemRoot bool) WorkspaceKind {
	isInsideEcosystem := parentEcosystemPath != ""
	isInEcosystemWorktree := isInsideEcosystem && strings.Contains(path, filepath.Join(parentEcosystemPath, ".grove-worktrees"))

	if isWorktree {
		if isEcosystemRoot {
			return KindEcosystemWorktree
		}
		if isInEcosystemWorktree {
			// This case is tricky. It could be a worktree of a subproject inside an ecosystem worktree.
			// Let's assume for now that if it's a worktree inside an ecosystem worktree's subproject,
			// it's of the most specific kind.
			return KindEcosystemWorktreeSubProjectWorktree // This is the linked-development case
		}
		if isInsideEcosystem {
			return KindEcosystemSubProjectWorktree
		}
		return KindStandaloneProjectWorktree
	}

	// Not a worktree
	if isEcosystemRoot {
		return KindEcosystemRoot
	}
	if isInEcosystemWorktree {
		// This is a primary repo checkout inside an eco worktree (the fallback case)
		return KindEcosystemWorktreeSubProject
	}
	if isInsideEcosystem {
		return KindEcosystemSubProject
	}

	// If no grove.yml, it would be NonGroveRepo, but this function assumes a Project.
	return KindStandaloneProject
}

// TransformToProjectInfo converts a hierarchical DiscoveryResult into a flat list
// of ProjectInfo items suitable for display in UIs.
func TransformToProjectInfo(result *DiscoveryResult) []*ProjectInfo {
	var projects []*ProjectInfo
	projectMap := make(map[string]*Project)
	for i := range result.Projects {
		projectMap[result.Projects[i].Path] = &result.Projects[i]
	}

	// First, add ecosystems themselves as ProjectInfo items
	for _, eco := range result.Ecosystems {
		projects = append(projects, &ProjectInfo{
			Name:                eco.Name,
			Path:                eco.Path,
			Kind:                KindEcosystemRoot,
			ParentEcosystemPath: "", // It is its own root
		})
	}

	// Then process all discovered projects and their workspaces
	for _, proj := range result.Projects {
		// Is this project an ecosystem worktree?
		isEcoWorktree := false
		if proj.ParentEcosystemPath != "" {
			rel, err := filepath.Rel(proj.ParentEcosystemPath, proj.Path)
			if err == nil && strings.HasPrefix(rel, ".grove-worktrees"+string(filepath.Separator)) {
				// It's in a worktrees dir. If it's a direct child, it IS the worktree.
				if len(strings.Split(rel, string(filepath.Separator))) == 2 {
					isEcoWorktree = true
				}
			}
		}

		if isEcoWorktree {
			// This is an Ecosystem Worktree, treat it as a single ProjectInfo
			projects = append(projects, &ProjectInfo{
				Name:                filepath.Base(proj.Path),
				Path:                proj.Path,
				Kind:                KindEcosystemWorktree,
				ParentProjectPath:   proj.ParentEcosystemPath,
				ParentEcosystemPath: proj.ParentEcosystemPath, // An eco worktree is inside its own parent eco
			})
			// We don't process its 'workspaces' field because it's represented as a single entity.
			// Sub-projects inside it will be handled as separate items in the result.Projects loop.
			continue
		}

		// Handle the primary workspace of the project
		isSubProject := proj.ParentEcosystemPath != "" && proj.Path != proj.ParentEcosystemPath
		kind := KindStandaloneProject
		if isSubProject {
			// Check if it's inside an ecosystem worktree
			// This logic is complex and relies on careful path inspection
			if strings.Contains(proj.Path, ".grove-worktrees") {
				kind = KindEcosystemWorktreeSubProject
			} else {
				kind = KindEcosystemSubProject
			}
		}

		projects = append(projects, &ProjectInfo{
			Name:                proj.Name,
			Path:                proj.Path,
			Kind:                kind,
			ParentEcosystemPath: proj.ParentEcosystemPath,
			Version:             proj.Version,
			Commit:              proj.Commit,
			AuditStatus:         proj.AuditStatus,
			ReportPath:          proj.ReportPath,
		})

		// Add all associated Worktree Workspaces
		for _, ws := range proj.Workspaces {
			if ws.Type == WorkspaceTypeWorktree {
				wtKind := KindStandaloneProjectWorktree
				if isSubProject {
					if strings.Contains(ws.Path, ".grove-worktrees") {
						// Check if this is a worktree within an ecosystem worktree
						// by examining whether the parent project itself is in .grove-worktrees
						if strings.Contains(proj.Path, ".grove-worktrees") {
							wtKind = KindEcosystemWorktreeSubProjectWorktree
						} else {
							wtKind = KindEcosystemSubProjectWorktree
						}
					} else {
						wtKind = KindEcosystemSubProjectWorktree
					}
				}

				projects = append(projects, &ProjectInfo{
					Name:                ws.Name,
					Path:                ws.Path,
					Kind:                wtKind,
					ParentProjectPath:   ws.ParentProjectPath,
					ParentEcosystemPath: proj.ParentEcosystemPath,
				})
			}
		}
	}

	// Also include Non-Grove Directories
	for _, path := range result.NonGroveDirectories {
		projects = append(projects, &ProjectInfo{
			Name: filepath.Base(path),
			Path: path,
			Kind: KindNonGroveRepo,
		})
	}

	return projects
}
