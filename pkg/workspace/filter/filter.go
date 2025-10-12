package filter

import (
	"sort"
	"strings"

	"github.com/mattsolo1/grove-core/pkg/workspace"
)

// FilterByFocus returns only the focused project and its children/worktrees
func FilterByFocus(projects []*workspace.WorkspaceNode, focus *workspace.WorkspaceNode) []*workspace.WorkspaceNode {
	if focus == nil {
		return projects
	}

	var result []*workspace.WorkspaceNode

	// Include the focused project
	for _, p := range projects {
		if p.Path == focus.Path {
			result = append(result, p)
			break
		}
	}

	// Include all children of the focused ecosystem
	for _, p := range projects {
		if p.ParentEcosystemPath == focus.Path && p.Path != focus.Path {
			result = append(result, p)
		}
	}

	// If the focused project is an ecosystem worktree, also include projects
	// that are children of this worktree
	if focus.Kind == workspace.KindEcosystemWorktree {
		for _, p := range projects {
			// Include sub-projects that are inside this ecosystem worktree
			if p.ParentEcosystemPath == focus.Path && p.Path != focus.Path {
				result = append(result, p)
			}
		}
	}

	return result
}

// FoldWorktrees returns a new slice with all worktrees removed
func FoldWorktrees(projects []*workspace.WorkspaceNode) []*workspace.WorkspaceNode {
	var result []*workspace.WorkspaceNode
	for _, p := range projects {
		switch p.Kind {
		case workspace.KindStandaloneProjectWorktree,
			workspace.KindEcosystemWorktree,
			workspace.KindEcosystemSubProjectWorktree,
			workspace.KindEcosystemWorktreeSubProjectWorktree:
			// This is a worktree, so skip it.
			continue
		default:
			result = append(result, p)
		}
	}
	return result
}

// FilterByText returns projects where the name or path contains the text
func FilterByText(projects []*workspace.WorkspaceNode, text string) []*workspace.WorkspaceNode {
	if text == "" {
		return projects
	}

	lowerText := strings.ToLower(text)
	var result []*workspace.WorkspaceNode

	for _, p := range projects {
		lowerName := strings.ToLower(p.Name)
		lowerPath := strings.ToLower(p.Path)

		if strings.Contains(lowerName, lowerText) || strings.Contains(lowerPath, lowerText) {
			result = append(result, p)
		}
	}

	return result
}

// SortByMatchQuality sorts projects based on how well their name matches the filter text
// Exact matches come first, then prefix matches, then substring matches
func SortByMatchQuality(projects []*workspace.WorkspaceNode, filterText string) []*workspace.WorkspaceNode {
	if filterText == "" {
		return projects
	}

	lowerFilter := strings.ToLower(filterText)

	// Create a copy to avoid modifying the original slice
	result := make([]*workspace.WorkspaceNode, len(projects))
	copy(result, projects)

	// Helper function to get match quality score (higher is better)
	getMatchQuality := func(p *workspace.WorkspaceNode) int {
		lowerName := strings.ToLower(p.Name)

		if lowerName == lowerFilter {
			return 3 // Exact match
		} else if strings.HasPrefix(lowerName, lowerFilter) {
			return 2 // Prefix match
		} else if strings.Contains(lowerName, lowerFilter) {
			return 1 // Contains in name
		}
		return 0 // No direct match (included because path matched)
	}

	sort.SliceStable(result, func(i, j int) bool {
		return getMatchQuality(result[i]) > getMatchQuality(result[j])
	})

	return result
}

// SortByActivity sorts projects to show groups with active sessions first
// The runningSessions map should have session names (derived from path) as keys
func SortByActivity(projects []*workspace.WorkspaceNode, runningSessions map[string]bool) []*workspace.WorkspaceNode {
	if runningSessions == nil || len(runningSessions) == 0 {
		return projects
	}

	// Create a copy to avoid modifying the original slice
	result := make([]*workspace.WorkspaceNode, len(projects))
	copy(result, projects)

	// Helper to determine if a project's group is active
	isGroupActive := func(p *workspace.WorkspaceNode) bool {
		groupKey := p.Path
		if p.ParentProjectPath != "" {
			groupKey = p.ParentProjectPath
		}
		return runningSessions[groupKey]
	}

	sort.SliceStable(result, func(i, j int) bool {
		isIActive := isGroupActive(result[i])
		isJActive := isGroupActive(result[j])

		if isIActive && !isJActive {
			return true
		}
		if !isIActive && isJActive {
			return false
		}
		return false // Maintain original order for groups of same activity status
	})

	return result
}

// GroupByParent groups projects with their worktrees hierarchically
// Returns a flat list where each parent is followed by its worktrees
func GroupByParent(projects []*workspace.WorkspaceNode, folded bool) []*workspace.WorkspaceNode {
	// Build a map of parents to their worktrees
	parentWorktrees := make(map[string][]*workspace.WorkspaceNode)
	var parents []*workspace.WorkspaceNode

	for _, p := range projects {
		isWorktree := p.ParentProjectPath != ""
		if isWorktree {
			parentWorktrees[p.ParentProjectPath] = append(parentWorktrees[p.ParentProjectPath], p)
		} else {
			parents = append(parents, p)
		}
	}

	// Build result with parents followed by their worktrees (if not folded)
	var result []*workspace.WorkspaceNode
	for _, parent := range parents {
		result = append(result, parent)

		if !folded {
			if worktrees, exists := parentWorktrees[parent.Path]; exists {
				result = append(result, worktrees...)
			}
		}
	}

	return result
}

// GroupHierarchically organizes projects in a tree structure considering full ecosystem hierarchy.
// Returns a flat list ordered depth-first where each node is followed by its children.
// This function considers:
// - Ecosystem roots as top-level
// - Ecosystem worktrees as children of root
// - Sub-projects as children of their ecosystem
// - Sub-project worktrees as children of their project
func GroupHierarchically(projects []*workspace.WorkspaceNode, folded bool) []*workspace.WorkspaceNode {
	// Build a map of parent path -> children
	childrenMap := make(map[string][]*workspace.WorkspaceNode)

	// Build a map for quick node lookup by path
	nodeMap := make(map[string]*workspace.WorkspaceNode)
	for _, p := range projects {
		nodeMap[p.Path] = p
	}

	// Populate children map based on hierarchical parent
	var roots []*workspace.WorkspaceNode
	for _, p := range projects {
		parent := p.GetHierarchicalParent()
		if parent == "" {
			// This is a root node
			roots = append(roots, p)
		} else {
			childrenMap[parent] = append(childrenMap[parent], p)
		}
	}

	// Recursively build the result in depth-first order
	var result []*workspace.WorkspaceNode
	var addNodeAndChildren func(node *workspace.WorkspaceNode, depth int)

	addNodeAndChildren = func(node *workspace.WorkspaceNode, depth int) {
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
