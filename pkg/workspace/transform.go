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

	// Set ParentProjectPath for KindEcosystemWorktreeSubProjectWorktree nodes
	// These are sub-projects within ecosystem worktrees (linked development state)
	// Their ParentProjectPath should point to the corresponding sub-project in the root ecosystem
	for _, node := range nodes {
		if node.Kind == KindEcosystemWorktreeSubProjectWorktree && node.RootEcosystemPath != "" {
			// Compute ParentProjectPath as RootEcosystemPath + project name
			// e.g., /path/to/ecosystem + grove-mcp = /path/to/ecosystem/grove-mcp
			node.ParentProjectPath = filepath.Join(node.RootEcosystemPath, node.Name)
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

// BuildWorkspaceTree takes a flat slice of WorkspaceNodes and pre-calculates
// presentation data for TUI rendering. It organizes nodes hierarchically and
// populates the TreePrefix and Depth fields.
//
// The TreePrefix field contains the indentation and tree connectors (e.g., "  ├─ ", "  └─ ")
// making it trivial for views to render the tree structure without complex logic.
func BuildWorkspaceTree(nodes []*WorkspaceNode) []*WorkspaceNode {
	if len(nodes) == 0 {
		return nodes
	}

	// Import the filter package to use GroupHierarchically
	// Note: This creates a dependency, but it's appropriate since we're building on that logic
	hierarchical := groupHierarchicallyInternal(nodes, false)

	// Build a map of parent path -> list of children to know when we're on the last child
	childrenMap := make(map[string][]*WorkspaceNode)
	for _, node := range hierarchical {
		parent := node.GetHierarchicalParent()
		if parent != "" {
			childrenMap[parent] = append(childrenMap[parent], node)
		}
	}

	// Iterate through the hierarchical list and calculate prefixes
	for _, node := range hierarchical {
		// Use the pre-calculated Depth field
		depth := node.Depth

		if depth == 0 {
			// Root level nodes have no prefix
			node.TreePrefix = ""
			continue
		}

		// Build the prefix based on depth and position
		var prefix string
		parent := node.GetHierarchicalParent()

		if parent != "" {
			siblings := childrenMap[parent]
			isLastChild := false

			// Check if this is the last child of its parent
			for j, sibling := range siblings {
				if sibling.Path == node.Path && j == len(siblings)-1 {
					isLastChild = true
					break
				}
			}

			// Build indentation based on depth
			for d := 1; d < depth; d++ {
				prefix += "  "
			}

			// Add the tree connector
			if isLastChild {
				prefix += "└─ "
			} else {
				prefix += "├─ "
			}
		}

		node.TreePrefix = prefix
	}

	return hierarchical
}

// groupHierarchicallyInternal is a simplified version of filter.GroupHierarchically
// to avoid circular dependencies. It organizes nodes in depth-first order.
func groupHierarchicallyInternal(nodes []*WorkspaceNode, folded bool) []*WorkspaceNode {
	// Build a map of parent path -> children
	childrenMap := make(map[string][]*WorkspaceNode)

	// Build a map for quick node lookup by path
	nodeMap := make(map[string]*WorkspaceNode)
	for _, p := range nodes {
		nodeMap[p.Path] = p
	}

	// Populate children map based on hierarchical parent
	var roots []*WorkspaceNode
	for _, p := range nodes {
		parent := p.GetHierarchicalParent()
		if parent == "" {
			// This is a root node
			roots = append(roots, p)
		} else {
			childrenMap[parent] = append(childrenMap[parent], p)
		}
	}

	// Recursively build the result in depth-first order
	var result []*WorkspaceNode
	var addNodeAndChildren func(node *WorkspaceNode, depth int)

	addNodeAndChildren = func(node *WorkspaceNode, depth int) {
		node.Depth = depth // Set the dynamically calculated depth
		result = append(result, node)

		if !folded || !node.IsWorktree() {
			// Add children recursively
			if children, exists := childrenMap[node.Path]; exists {
				for _, child := range children {
					addNodeAndChildren(child, depth+1)
				}
			}
		}
	}

	// Add all root nodes and their children
	for _, root := range roots {
		addNodeAndChildren(root, 0)
	}

	return result
}

// BuildTree constructs a hierarchical tree from a flat slice of WorkspaceNodes.
// This is the recommended way for UIs to consume the workspace hierarchy.
func BuildTree(nodes []*WorkspaceNode) []*WorkspaceTree {
	if len(nodes) == 0 {
		return nil
	}

	nodeMap := make(map[string]*WorkspaceNode)
	for _, n := range nodes {
		nodeMap[n.Path] = n
	}

	childrenMap := make(map[string][]*WorkspaceNode)
	var roots []*WorkspaceNode
	for _, n := range nodes {
		parentPath := n.GetHierarchicalParent()
		if parentPath == "" || nodeMap[parentPath] == nil {
			roots = append(roots, n)
		} else {
			childrenMap[parentPath] = append(childrenMap[parentPath], n)
		}
	}

	var buildSubTree func(node *WorkspaceNode) *WorkspaceTree
	buildSubTree = func(node *WorkspaceNode) *WorkspaceTree {
		treeNode := &WorkspaceTree{Node: node}
		if children, exists := childrenMap[node.Path]; exists {
			for _, childNode := range children {
				treeNode.Children = append(treeNode.Children, buildSubTree(childNode))
			}
		}
		return treeNode
	}

	var treeRoots []*WorkspaceTree
	for _, rootNode := range roots {
		treeRoots = append(treeRoots, buildSubTree(rootNode))
	}

	return treeRoots
}
