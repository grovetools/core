package workspace

import (
	"os"
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

// TransformToWorkspaceNodes converts a hierarchical DiscoveryResult into a flat list
// of WorkspaceNode items suitable for display in UIs.
func TransformToWorkspaceNodes(result *DiscoveryResult) []*WorkspaceNode {
	var nodes []*WorkspaceNode
	projectMap := make(map[string]*Project)
	for i := range result.Projects {
		projectMap[result.Projects[i].Path] = &result.Projects[i]
	}

	// First, add ecosystems themselves as WorkspaceNode items
	for _, eco := range result.Ecosystems {
		nodes = append(nodes, &WorkspaceNode{
			Name:                eco.Name,
			Path:                eco.Path,
			Kind:                KindEcosystemRoot,
			ParentEcosystemPath: "", // It is its own root
			RootEcosystemPath:   eco.Path,
		})
	}

	// Then process all discovered projects and their workspaces
	for _, proj := range result.Projects {
		// Check if this is a cloned repo (Type == "Cloned") - these should be NonGroveRepo
		if proj.Type == "Cloned" {
			nodes = append(nodes, &WorkspaceNode{
				Name:        proj.Name,
				Path:        proj.Path,
				Kind:        KindNonGroveRepo,
				Version:     proj.Version,
				Commit:      proj.Commit,
				AuditStatus: proj.AuditStatus,
				ReportPath:  proj.ReportPath,
			})
			continue
		}

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
			// This is an Ecosystem Worktree, treat it as a single WorkspaceNode
			nodes = append(nodes, &WorkspaceNode{
				Name:                filepath.Base(proj.Path),
				Path:                proj.Path,
				Kind:                KindEcosystemWorktree,
				ParentProjectPath:   proj.ParentEcosystemPath,
				ParentEcosystemPath: proj.ParentEcosystemPath, // An eco worktree is inside its own parent eco
				// RootEcosystemPath will be set in the hierarchy resolution pass
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
			if strings.Contains(proj.Path, ".grove-worktrees") {
				// Check if this project is itself a git worktree by examining .git
				gitPath := filepath.Join(proj.Path, ".git")
				if stat, err := os.Stat(gitPath); err == nil && !stat.IsDir() {
					// .git is a file, so this is a worktree (linked development)
					kind = KindEcosystemWorktreeSubProjectWorktree
				} else {
					// .git is a directory or doesn't exist, so it's a full checkout
					kind = KindEcosystemWorktreeSubProject
				}
			} else {
				kind = KindEcosystemSubProject
			}
		}

		nodes = append(nodes, &WorkspaceNode{
			Name:                proj.Name,
			Path:                proj.Path,
			Kind:                kind,
			ParentEcosystemPath: proj.ParentEcosystemPath,
			// RootEcosystemPath will be set in the hierarchy resolution pass
			Version:     proj.Version,
			Commit:      proj.Commit,
			AuditStatus: proj.AuditStatus,
			ReportPath:  proj.ReportPath,
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

				nodes = append(nodes, &WorkspaceNode{
					Name:                ws.Name,
					Path:                ws.Path,
					Kind:                wtKind,
					ParentProjectPath:   ws.ParentProjectPath,
					ParentEcosystemPath: proj.ParentEcosystemPath,
					// RootEcosystemPath will be set in the hierarchy resolution pass
				})
			}
		}
	}

	// Also include Non-Grove Directories
	for _, path := range result.NonGroveDirectories {
		nodes = append(nodes, &WorkspaceNode{
			Name: filepath.Base(path),
			Path: path,
			Kind: KindNonGroveRepo,
		})
	}

	// Hierarchy resolution pass: set RootEcosystemPath for all nodes
	nodeMap := make(map[string]*WorkspaceNode)
	for _, node := range nodes {
		nodeMap[node.Path] = node
	}

	for _, node := range nodes {
		if node.Kind == KindEcosystemRoot {
			// Already set during creation
			continue
		}

		// Traverse up the ParentEcosystemPath chain to find the root
		if node.ParentEcosystemPath != "" {
			rootPath := findRootEcosystem(node.ParentEcosystemPath, nodeMap)
			node.RootEcosystemPath = rootPath
		}
	}

	return nodes
}

// findRootEcosystem traverses up the parent chain to find the ultimate EcosystemRoot
func findRootEcosystem(startPath string, nodeMap map[string]*WorkspaceNode) string {
	current := startPath
	for {
		node, exists := nodeMap[current]
		if !exists {
			// Path not in our map, return what we have
			return current
		}

		if node.Kind == KindEcosystemRoot {
			// Found the root
			return node.Path
		}

		if node.ParentEcosystemPath == "" || node.ParentEcosystemPath == current {
			// No more parents, this is as far as we can go
			return current
		}

		// Move up the chain
		current = node.ParentEcosystemPath
	}
}
