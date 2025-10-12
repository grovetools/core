package wsnav

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// Update handles messages and updates the model accordingly.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.SetSize(m.width, m.height)
		return m, nil

	case tea.KeyMsg:
		if m.help.ShowAll {
			m.help.Toggle()
			return m, nil
		}

		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit

		case key.Matches(msg, m.keys.Help):
			m.help.Toggle()
			return m, nil

		case key.Matches(msg, m.keys.Top):
			// Handle 'gg' - go to top
			if m.lastKeyWasG {
				m.cursor = 0
				m.ensureCursorVisible()
				m.lastKeyWasG = false
			} else {
				m.lastKeyWasG = true
			}

		case key.Matches(msg, m.keys.Bottom):
			// Handle 'G' - go to bottom
			if len(m.viewProjects) > 0 {
				m.cursor = len(m.viewProjects) - 1
				m.ensureCursorVisible()
			}
			m.lastKeyWasG = false

		case key.Matches(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
				m.ensureCursorVisible()
			}
			m.lastKeyWasG = false

		case key.Matches(msg, m.keys.Down):
			if m.cursor < len(m.viewProjects)-1 {
				m.cursor++
				m.ensureCursorVisible()
			}
			m.lastKeyWasG = false

		case key.Matches(msg, m.keys.PageUp):
			m.cursor -= m.height / 2
			if m.cursor < 0 {
				m.cursor = 0
			}
			m.ensureCursorVisible()
			m.lastKeyWasG = false

		case key.Matches(msg, m.keys.PageDown):
			m.cursor += m.height / 2
			if m.cursor >= len(m.viewProjects) {
				m.cursor = len(m.viewProjects) - 1
			}
			m.ensureCursorVisible()
			m.lastKeyWasG = false

		default:
			// Reset lastKeyWasG for any other key
			m.lastKeyWasG = false
		}
	}

	return m, nil
}

// ensureCursorVisible adjusts the scroll offset to ensure the cursor is visible
func (m *Model) ensureCursorVisible() {
	const headerHeight = 3
	const footerHeight = 3
	const topMargin = 1
	mainAreaHeight := m.height - headerHeight - footerHeight - topMargin
	availableHeight := mainAreaHeight - 2 - 2 - 2 // borders, padding, header row + separator

	// If cursor is above the viewport, scroll up
	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	}

	// If cursor is below the viewport, scroll down
	if m.cursor >= m.scrollOffset+availableHeight {
		m.scrollOffset = m.cursor - availableHeight + 1
	}
}
