package wsnav

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattsolo1/grove-core/tui/components/navigator"
)

// Update handles messages and updates the model accordingly.
// Delegates to the navigator component for all state management.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	updatedModel, cmd := m.navigator.Update(msg)
	m.navigator = updatedModel.(navigator.Model)
	return m, cmd
}
