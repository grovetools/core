package navigator

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattsolo1/grove-core/pkg/workspace"
)

// ProjectsLoadedMsg is sent when a new list of projects has been fetched.
type ProjectsLoadedMsg struct {
	Projects []workspace.WorkspaceNode
}

// tickMsg is sent periodically to trigger a refresh
type tickMsg time.Time

// tickCmd returns a command that sends a tick message after a delay
func tickCmd() tea.Cmd {
	return tea.Tick(time.Second*2, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// tick returns a command that sends a tick message after the configured RefreshInterval
func (m Model) tick() tea.Cmd {
	return tea.Tick(time.Duration(m.RefreshInterval)*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// RefreshProjectsCmd returns a command that uses the model's ProjectsLoader to fetch updated projects.
func (m *Model) RefreshProjectsCmd() tea.Cmd {
	if m.ProjectsLoader == nil {
		return nil
	}
	return func() tea.Msg {
		projects, err := m.ProjectsLoader()
		if err != nil {
			// In a real implementation, we would return an error message.
			// For now, we return nil.
			return nil
		}
		return ProjectsLoadedMsg{Projects: projects}
	}
}
