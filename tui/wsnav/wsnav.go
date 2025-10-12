package wsnav

import (
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-core/pkg/workspace/filter"
	"github.com/mattsolo1/grove-core/tui/components/help"
)

// New creates a new model for the workspace navigator TUI.
func New(projects []*workspace.WorkspaceNode) *Model {
	// Group projects hierarchically for display. This returns a flat list
	// in depth-first order with proper ecosystem hierarchy.
	groupedProjects := filter.GroupHierarchically(projects, false)

	filterInput := textinput.New()
	filterInput.Placeholder = "Press / to filter..."
	filterInput.Prompt = "> "

	return &Model{
		allProjects:      projects,
		filteredProjects: groupedProjects,
		keys:             DefaultKeyMap,
		help:             help.New(DefaultKeyMap),
		filterInput:      filterInput,
		cursor:           0,
	}
}
