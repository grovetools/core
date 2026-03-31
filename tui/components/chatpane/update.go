package chatpane

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/grovetools/core/tui/components/logviewer"
)

// Update handles messages for the chat pane.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Resize(msg.Width, msg.Height)
		return m, nil

	case logviewer.LogLineMsg:
		var cmd tea.Cmd
		m.LogViewer, cmd = m.LogViewer.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		if m.InputActive {
			switch msg.String() {
			case "enter":
				text := m.Input.Value()
				if text != "" {
					m.Input.SetValue("")
					return m, func() tea.Msg {
						return InputSubmittedMsg{Text: text}
					}
				}
				return m, nil
			case "esc":
				m.BlurInput()
				return m, nil
			default:
				var cmd tea.Cmd
				m.Input, cmd = m.Input.Update(msg)
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
				return m, tea.Batch(cmds...)
			}
		}

		// When input is not active, pass navigation keys to log viewer
		switch msg.String() {
		case "i":
			cmd := m.FocusInput()
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)
		default:
			var cmd tea.Cmd
			m.LogViewer, cmd = m.LogViewer.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)
		}
	}

	// Pass other messages to logviewer
	var cmd tea.Cmd
	m.LogViewer, cmd = m.LogViewer.Update(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// Init initializes the component.
func (m Model) Init() tea.Cmd {
	return nil
}
