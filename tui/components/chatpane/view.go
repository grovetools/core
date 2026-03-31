package chatpane

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/grovetools/core/tui/theme"
)

// View renders the chat pane: log viewer + optional status + input box.
func (m Model) View() string {
	var sections []string

	// Log viewer takes remaining space
	sections = append(sections, m.LogViewer.View())

	// Optional status line
	if m.StatusText != "" {
		statusStyle := lipgloss.NewStyle().
			Foreground(theme.DefaultColors.MutedText).
			PaddingLeft(1)
		sections = append(sections, statusStyle.Render(m.StatusText))
	}

	// Input box
	sections = append(sections, m.renderInputBox())

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// renderInputBox renders the text input with a styled border.
func (m Model) renderInputBox() string {
	boxWidth := m.Width - 4
	if boxWidth < 20 {
		boxWidth = 20
	}

	borderColor := theme.DefaultColors.Cyan
	if m.InputActive {
		borderColor = theme.DefaultColors.Orange
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		Width(boxWidth)

	inputCopy := m.Input
	inputCopy.Width = boxWidth - 4
	if inputCopy.Width < 10 {
		inputCopy.Width = 10
	}

	return boxStyle.Render(inputCopy.View())
}
