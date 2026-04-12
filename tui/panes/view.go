package panes

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/grovetools/core/tui/theme"
)

// View renders the pane layout. If a pane is fullscreened, it renders only
// that pane. Otherwise it renders all visible panes joined with highlighted
// separators, skipping hidden panes entirely.
func (m Manager) View() string {
	if len(m.Panes) == 0 {
		return ""
	}

	// Fullscreen mode: render only the fullscreened pane
	if m.FullscreenIdx >= 0 && m.FullscreenIdx < len(m.Panes) {
		return m.Panes[m.FullscreenIdx].Model.View()
	}

	dims := m.calculateDimensions()
	var views []string

	for i, pane := range m.Panes {
		if pane.Hidden {
			continue
		}

		// Add separator before this pane (if not the first visible pane)
		if len(views) > 0 {
			sep := m.renderSeparator(i)
			views = append(views, sep)
		}

		// Render pane content, constrained to its allocated size
		content := pane.Model.View()
		var w, h int
		if i < len(dims) {
			w = dims[i].Width
			h = dims[i].Height
		}
		if w > 0 && h > 0 {
			content = lipgloss.NewStyle().
				Width(w).
				Height(h).
				MaxWidth(w).
				MaxHeight(h).
				Render(content)
		}
		views = append(views, content)
	}

	if m.Direction == DirectionHorizontal {
		return lipgloss.JoinHorizontal(lipgloss.Top, views...)
	}
	return lipgloss.JoinVertical(lipgloss.Left, views...)
}

// renderSeparator draws the separator adjacent to pane[index].
// The separator is highlighted if it borders the active pane.
func (m Manager) renderSeparator(index int) string {
	isActive := index == m.ActivePaneIdx || m.isAdjacentToActive(index)

	t := theme.DefaultTheme
	if t == nil {
		t = &theme.Theme{}
	}

	var color lipgloss.TerminalColor
	if isActive {
		color = theme.DefaultColors.Orange
	} else {
		color = theme.DefaultColors.Border
	}

	sepStyle := lipgloss.NewStyle().Foreground(color)

	if m.Direction == DirectionHorizontal {
		line := strings.Repeat("│\n", m.Height)
		if len(line) > 0 {
			line = line[:len(line)-1] // trim trailing newline
		}
		return sepStyle.Render(line)
	}

	return sepStyle.Render(strings.Repeat("─", m.Width))
}

// isAdjacentToActive checks whether the pane at index is the visible pane
// immediately after the active pane (used for separator highlighting).
func (m Manager) isAdjacentToActive(index int) bool {
	// Walk backwards from index to find the previous visible pane
	for j := index - 1; j >= 0; j-- {
		if !m.Panes[j].Hidden {
			return j == m.ActivePaneIdx
		}
	}
	return false
}
