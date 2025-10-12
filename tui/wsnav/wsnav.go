package wsnav

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-core/tui/components/navigator"
)

// New creates a new model for the workspace navigator TUI.
func New(projects []*workspace.WorkspaceNode) *Model {
	// Convert pointers to values for navigator
	projectValues := make([]workspace.WorkspaceNode, len(projects))
	for i, p := range projects {
		projectValues[i] = *p
	}

	// Create navigator with custom OnSelect handler
	nav := navigator.New(navigator.Config{
		Projects: projectValues,
	})

	m := &Model{
		navigator: nav,
	}

	// Set OnSelect callback to capture selection and quit
	m.navigator.OnSelect = func(selected workspace.WorkspaceNode) tea.Cmd {
		m.SelectedProject = &selected
		return tea.Quit
	}

	return m
}
