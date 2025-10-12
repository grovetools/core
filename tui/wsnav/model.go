package wsnav

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-core/tui/components/help"
)

// Model represents the state of the workspace navigator TUI.
type Model struct {
	allProjects      []*workspace.WorkspaceNode // The original, full list of all discovered workspace nodes.
	filteredProjects []*workspace.WorkspaceNode // The filtered and sorted list for display.
	keys             KeyMap
	help             help.Model
	filterInput      textinput.Model
	focusedProject   *workspace.WorkspaceNode
	worktreesFolded  bool
	cursor           int
	scrollOffset     int
	width            int
	height           int
	lastKeyWasG      bool                      // Track if last key press was 'g' for 'gg' combo
	SelectedProject  *workspace.WorkspaceNode // The project selected when Enter is pressed
}

// Init is the first command that will be executed.
func (m *Model) Init() tea.Cmd {
	return nil
}
