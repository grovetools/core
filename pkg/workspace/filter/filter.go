package filter

import (
	"sort"
	"strings"

	"github.com/mattsolo1/grove-core/pkg/workspace"
)

// FilterByFocus returns only the focused project and its children/worktrees
func FilterByFocus(projects []*workspace.ProjectInfo, focus *workspace.ProjectInfo) []*workspace.ProjectInfo {
	if focus == nil {
		return projects
	}

	var result []*workspace.ProjectInfo

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

	return result
}

// FoldWorktrees returns a new slice with all worktrees removed
func FoldWorktrees(projects []*workspace.ProjectInfo) []*workspace.ProjectInfo {
	var result []*workspace.ProjectInfo
	for _, p := range projects {
		if !p.IsWorktree {
			result = append(result, p)
		}
	}
	return result
}

// FilterByText returns projects where the name or path contains the text
func FilterByText(projects []*workspace.ProjectInfo, text string) []*workspace.ProjectInfo {
	if text == "" {
		return projects
	}

	lowerText := strings.ToLower(text)
	var result []*workspace.ProjectInfo

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
func SortByMatchQuality(projects []*workspace.ProjectInfo, filterText string) []*workspace.ProjectInfo {
	if filterText == "" {
		return projects
	}

	lowerFilter := strings.ToLower(filterText)

	// Create a copy to avoid modifying the original slice
	result := make([]*workspace.ProjectInfo, len(projects))
	copy(result, projects)

	// Helper function to get match quality score (higher is better)
	getMatchQuality := func(p *workspace.ProjectInfo) int {
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
func SortByActivity(projects []*workspace.ProjectInfo, runningSessions map[string]bool) []*workspace.ProjectInfo {
	if runningSessions == nil || len(runningSessions) == 0 {
		return projects
	}

	// Create a copy to avoid modifying the original slice
	result := make([]*workspace.ProjectInfo, len(projects))
	copy(result, projects)

	// Helper to determine if a project's group is active
	isGroupActive := func(p *workspace.ProjectInfo) bool {
		groupKey := p.Path
		if p.IsWorktree {
			groupKey = p.ParentPath
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
func GroupByParent(projects []*workspace.ProjectInfo, folded bool) []*workspace.ProjectInfo {
	// Build a map of parents to their worktrees
	parentWorktrees := make(map[string][]*workspace.ProjectInfo)
	var parents []*workspace.ProjectInfo

	for _, p := range projects {
		if p.IsWorktree {
			parentWorktrees[p.ParentPath] = append(parentWorktrees[p.ParentPath], p)
		} else {
			parents = append(parents, p)
		}
	}

	// Build result with parents followed by their worktrees (if not folded)
	var result []*workspace.ProjectInfo
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
