package wsnav

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-core/tui/components/navigator"
)

// Model wraps the navigator component and provides custom table-based rendering.
type Model struct {
	navigator       navigator.Model
	scrollOffset    int
	SelectedProject *workspace.WorkspaceNode // The project selected when Enter is pressed
}

// Init is the first command that will be executed.
func (m *Model) Init() tea.Cmd {
	return m.navigator.Init()
}
