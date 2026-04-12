package panes

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/grovetools/core/tui/theme"
)

// View renders the pane layout. If a pane is fullscreened (non-pinned), it
// renders only that pane. In pinned mode or normal mode it renders all visible
// panes joined with highlighted separators, skipping hidden panes entirely.
func (m Manager) View() string {
	if len(m.Panes) == 0 {
		return ""
	}

	// Standard fullscreen mode (not pinned): render only the fullscreened pane
	if m.FullscreenIdx >= 0 && !m.PinnedMode && m.FullscreenIdx < len(m.Panes) {
		return m.renderPaneContent(m.FullscreenIdx, m.Width, m.Height)
	}

	dims := m.CalculateDimensions()
	var views []string

	for i, pane := range m.Panes {
		if pane.Hidden || pane.Promoted {
			continue
		}

		// In pinned mode, skip Flex panes that got 0 size (not the zoomed one)
		if m.PinnedMode && m.FullscreenIdx >= 0 {
			var axisSize int
			if i < len(dims) {
				if m.Direction == DirectionHorizontal {
					axisSize = dims[i].Width
				} else {
					axisSize = dims[i].Height
				}
			}
			if axisSize == 0 && i != m.FullscreenIdx {
				continue
			}
		}

		// Add separator before this pane (if not the first visible pane)
		if len(views) > 0 {
			sep := m.renderSeparator(i)
			views = append(views, sep)
		}

		// Render pane content, constrained to its allocated size
		var w, h int
		if i < len(dims) {
			w = dims[i].Width
			h = dims[i].Height
		}

		content := m.renderPaneContent(i, w, h)
		views = append(views, content)
	}

	if m.Direction == DirectionHorizontal {
		return lipgloss.JoinHorizontal(lipgloss.Top, views...)
	}
	return lipgloss.JoinVertical(lipgloss.Left, views...)
}

// renderPaneContent renders a single pane's content, including its status line
// if the model implements StatusProvider.
func (m Manager) renderPaneContent(index, w, h int) string {
	pane := m.Panes[index]
	content := pane.Model.View()

	hasStatus := false
	var statusText string
	if sp, ok := pane.Model.(StatusProvider); ok {
		statusText = sp.StatusLine()
		if statusText != "" {
			hasStatus = true
		}
	}

	// Total height = content height + status line height
	contentH := h
	if hasStatus {
		contentH = h // already subtracted in calculateDimensions
	}

	if w > 0 && contentH > 0 {
		content = padContent(content, w, contentH)
	}

	if hasStatus {
		t := theme.DefaultTheme
		statusStyle := lipgloss.NewStyle().
			Foreground(t.Colors.DarkText).
			Background(t.Colors.SubtleBackground).
			MaxWidth(w).
			Width(w)
		rendered := statusStyle.Render(statusText)
		content = lipgloss.JoinVertical(lipgloss.Left, content, rendered)
	}

	return content
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
		// In pinned mode we need actual height based on rendered content
		sepH := m.Height
		line := strings.Repeat("│\n", sepH)
		if len(line) > 0 {
			line = line[:len(line)-1] // trim trailing newline
		}
		return sepStyle.Render(line)
	}

	return sepStyle.Render(strings.Repeat("─", m.Width))
}

// padContent pads content to exactly w×h without word-wrapping.
// Unlike lipgloss.Width(w) which triggers muesli/reflow and can break
// pre-formatted ANSI strings (e.g., scrollbar overlays), this simply
// pads short lines with spaces and truncates/pads the line count to h.
func padContent(content string, w, h int) string {
	lines := strings.Split(content, "\n")

	// Pad or truncate to exactly h lines
	for len(lines) < h {
		lines = append(lines, "")
	}
	if len(lines) > h {
		lines = lines[:h]
	}

	// Pad each line to width w without wrapping
	for i, line := range lines {
		lineW := lipgloss.Width(line)
		if lineW < w {
			lines[i] = line + strings.Repeat(" ", w-lineW)
		}
	}

	return strings.Join(lines, "\n")
}

// isAdjacentToActive checks whether the pane at index is the visible pane
// immediately after the active pane (used for separator highlighting).
func (m Manager) isAdjacentToActive(index int) bool {
	// Walk backwards from index to find the previous visible pane
	for j := index - 1; j >= 0; j-- {
		if !m.Panes[j].Hidden && !m.Panes[j].Promoted {
			return j == m.ActivePaneIdx
		}
	}
	return false
}
