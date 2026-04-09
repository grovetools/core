package pager

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/grovetools/core/tui/embed"
)

// Update handles WindowSizeMsg (with tab-bar height deduction),
// numeric tab jumps, [/] cycling, embed.SwitchTabMsg auto-switch,
// and Focus/Blur fan-out. Anything else is forwarded to the active
// page.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if len(m.pages) == 0 {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		pageHeight := msg.Height - tabBarHeight
		if pageHeight < 1 {
			pageHeight = 1
		}
		for _, p := range m.pages {
			p.SetSize(msg.Width, pageHeight)
		}
		adjusted := tea.WindowSizeMsg{Width: msg.Width, Height: pageHeight}
		updated, cmd := m.pages[m.activePage].Update(adjusted)
		m.pages[m.activePage] = updated
		return m, cmd

	case embed.SwitchTabMsg:
		return m.switchTo(msg.TabIndex)

	case embed.FocusMsg:
		updated, cmd := m.pages[m.activePage].Update(msg)
		m.pages[m.activePage] = updated
		if cmd != nil {
			return m, cmd
		}
		return m, m.pages[m.activePage].Focus()

	case embed.BlurMsg:
		m.pages[m.activePage].Blur()
		updated, cmd := m.pages[m.activePage].Update(msg)
		m.pages[m.activePage] = updated
		return m, cmd

	case tea.KeyMsg:
		if idx, ok := m.matchTabJump(msg); ok {
			return m.switchTo(idx)
		}
		switch {
		case key.Matches(msg, m.keys.NextTab):
			return m.cycle(1)
		case key.Matches(msg, m.keys.PrevTab):
			return m.cycle(-1)
		}
	}

	updated, cmd := m.pages[m.activePage].Update(msg)
	m.pages[m.activePage] = updated
	return m, cmd
}

// matchTabJump returns the tab index (0-based) for a Tab1..Tab9
// keypress, or false. Indices past the page count are not matched.
func (m Model) matchTabJump(msg tea.KeyMsg) (int, bool) {
	bindings := []key.Binding{
		m.keys.Tab1, m.keys.Tab2, m.keys.Tab3,
		m.keys.Tab4, m.keys.Tab5, m.keys.Tab6,
		m.keys.Tab7, m.keys.Tab8, m.keys.Tab9,
	}
	for i, b := range bindings {
		if i >= len(m.pages) {
			return 0, false
		}
		if key.Matches(msg, b) {
			return i, true
		}
	}
	return 0, false
}

// switchTo activates the page at idx (Blur old, Focus new).
func (m Model) switchTo(idx int) (Model, tea.Cmd) {
	if idx < 0 || idx >= len(m.pages) || idx == m.activePage {
		return m, nil
	}
	m.pages[m.activePage].Blur()
	m.activePage = idx
	return m, m.pages[m.activePage].Focus()
}

// cycle advances the active page by delta, wrapping.
func (m Model) cycle(delta int) (Model, tea.Cmd) {
	n := len(m.pages)
	if n == 0 {
		return m, nil
	}
	next := (m.activePage + delta + n) % n
	return m.switchTo(next)
}
