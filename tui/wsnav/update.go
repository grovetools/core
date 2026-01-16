package wsnav

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/grovetools/core/tui/components/navigator"
)

// Update handles messages and updates the model accordingly.
// Delegates to the navigator component for all state management.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// ENRICHMENT EXAMPLE: Handle enrichment messages before delegating to navigator.
	// This demonstrates the pattern for receiving and storing enrichment data:
	//   1. Match on custom message types
	//   2. Use mutex locks for thread-safe map updates
	//   3. React to ProjectsLoadedMsg to refresh enrichment when projects change
	//
	// External callers should add similar case statements for their own
	// enrichment message types (e.g., sessionInfoLoadedMsg, noteCountsLoadedMsg)
	switch msg := msg.(type) {
	case gitStatusLoadedMsg:
		// Store the git status in our map (thread-safe)
		m.gitStatusMutex.Lock()
		m.gitStatus[msg.path] = msg.status
		m.gitStatusMutex.Unlock()

	case navigator.ProjectsLoadedMsg:
		// When projects are refreshed, dispatch new enrichment commands.
		// This ensures enrichment data stays fresh as the project list changes.
		for _, p := range msg.Projects {
			cmds = append(cmds, m.fetchGitStatusCmd(p.Path))
		}
	}

	// Delegate to navigator for all messages
	updatedModel, cmd := m.navigator.Update(msg)
	m.navigator = updatedModel.(navigator.Model)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}
