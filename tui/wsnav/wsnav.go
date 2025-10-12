package wsnav

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattsolo1/grove-core/git"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-core/tui/components/navigator"
	"github.com/sirupsen/logrus"
)

// New creates a new model for the workspace navigator TUI.
func New(projects []*workspace.WorkspaceNode, refreshInterval int) *Model {
	// Convert pointers to values for navigator
	projectValues := make([]workspace.WorkspaceNode, len(projects))
	for i, p := range projects {
		projectValues[i] = *p
	}

	// Create navigator with custom OnSelect handler
	nav := navigator.New(navigator.Config{
		Projects: projectValues,
	})

	// Configure refresh functionality
	nav.RefreshInterval = refreshInterval
	nav.ProjectsLoader = func() ([]workspace.WorkspaceNode, error) {
		// This function will be executed in the background on every tick.
		logger := logrus.New()
		logger.SetOutput(os.Stderr) // Avoid interfering with TUI
		logger.SetLevel(logrus.WarnLevel)
		// GetProjects performs the full discovery and returns the transformed nodes.
		projectPtrs, err := workspace.GetProjects(logger)
		if err != nil {
			return nil, err
		}
		// Convert pointers to values
		projects := make([]workspace.WorkspaceNode, len(projectPtrs))
		for i, p := range projectPtrs {
			projects[i] = *p
		}
		return projects, nil
	}

	// ENRICHMENT EXAMPLE: Initialize enrichment data structures in the constructor.
	// External callers should initialize their enrichment maps here as well.
	m := &Model{
		navigator: nav,
		gitStatus: make(map[string]*git.StatusInfo),
	}

	// Set OnSelect callback to capture selection and quit
	m.navigator.OnSelect = func(selected workspace.WorkspaceNode) tea.Cmd {
		m.SelectedProject = &selected
		return tea.Quit
	}

	return m
}
