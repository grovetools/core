package wsnav

import (
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-core/pkg/workspace/filter"
	"github.com/mattsolo1/grove-core/tui/components/help"
)

// New creates a new model for the workspace navigator TUI.
func New(projects []*workspace.WorkspaceNode) Model {
	// Group projects hierarchically for display. This returns a flat list
	// but in a parent-then-children order.
	groupedProjects := filter.GroupByParent(projects, false)

	return Model{
		allProjects:  projects,
		viewProjects: groupedProjects,
		keys:         DefaultKeyMap,
		help:         help.New(DefaultKeyMap),
		cursor:       0,
	}
}
