package wsnav

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattsolo1/grove-core/pkg/workspace/filter"
)

// Update handles messages and updates the model accordingly.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

		// Handle filter input when it's focused
		if m.filterInput.Focused() {
			switch {
			case key.Matches(msg, m.keys.Quit): // Esc or Ctrl+C
				m.filterInput.Blur()
				m.applyFiltersAndSort()
				return m, nil
			case key.Matches(msg, m.keys.Up):
				if m.cursor > 0 {
					m.cursor--
				}
				return m, nil
			case key.Matches(msg, m.keys.Down):
				if m.cursor < len(m.filteredProjects)-1 {
					m.cursor++
				}
				return m, nil
			case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
				// Selection logic is handled below
				if len(m.filteredProjects) > 0 && m.cursor < len(m.filteredProjects) {
					m.SelectedProject = m.filteredProjects[m.cursor]
					return m, tea.Quit
				}
			default:
				var cmd tea.Cmd
				m.filterInput, cmd = m.filterInput.Update(msg)
				m.applyFiltersAndSort()
				m.cursor = 0
				return m, cmd
			}
		}

		switch {
		case key.Matches(msg, m.keys.Search):
			m.filterInput.Focus()
			return m, textinput.Blink

		case key.Matches(msg, m.keys.Focus):
			if len(m.filteredProjects) > 0 && m.cursor < len(m.filteredProjects) {
				selected := m.filteredProjects[m.cursor]
				m.focusedProject = selected
				m.applyFiltersAndSort()
				m.cursor = 0
			}

		case key.Matches(msg, m.keys.ClearFocus):
			m.focusedProject = nil
			m.applyFiltersAndSort()
			m.cursor = 0

		case key.Matches(msg, m.keys.ToggleWorktrees):
			m.worktreesFolded = !m.worktreesFolded
			m.applyFiltersAndSort()

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			if len(m.filteredProjects) > 0 && m.cursor < len(m.filteredProjects) {
				m.SelectedProject = m.filteredProjects[m.cursor]
				return m, tea.Quit
			}

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
			if len(m.filteredProjects) > 0 {
				m.cursor = len(m.filteredProjects) - 1
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
			if m.cursor < len(m.filteredProjects)-1 {
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
			if m.cursor >= len(m.filteredProjects) {
				m.cursor = len(m.filteredProjects) - 1
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

// applyFiltersAndSort updates the filteredProjects list based on the current model state.
func (m *Model) applyFiltersAndSort() {
	// Start with the full list of projects
	projects := m.allProjects

	// 1. Apply focus
	projects = filter.FilterByFocus(projects, m.focusedProject)

	// 2. Fold worktrees if enabled
	if m.worktreesFolded {
		projects = filter.FoldWorktrees(projects)
	}

	// 3. Apply text filter
	filterText := m.filterInput.Value()
	projects = filter.FilterByText(projects, filterText)

	// 4. Sort by match quality
	projects = filter.SortByMatchQuality(projects, filterText)

	// 5. Group hierarchically to get the correct display order with full ecosystem hierarchy
	m.filteredProjects = filter.GroupHierarchically(projects, m.worktreesFolded)

	// Reset cursor and ensure it's valid
	if m.cursor >= len(m.filteredProjects) {
		m.cursor = len(m.filteredProjects) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}
