package panes

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// Update handles messages for the pane manager.
// It intercepts layout keys (unless text input is active),
// distributes WindowSizeMsg to all panes, routes TargetedMsg/BroadcastMsg
// envelopes, and broadcasts other non-key messages to all panes so
// background streams (tickers, SSE, etc.) work in unfocused panes.
func (m Manager) Update(msg tea.Msg) (Manager, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		return m.distributeSize()

	case TargetedMsg:
		return m.updateTargeted(msg)

	case BroadcastMsg:
		return m.updateAllPanes(msg.Payload)

	case tea.KeyMsg:
		// If active pane has text input active, pass everything through
		if m.IsTextInputActive() {
			return m.updateActivePane(msg)
		}

		switch {
		case key.Matches(msg, m.KeyMap.CycleNext):
			return m.cycleFocus(1)
		case key.Matches(msg, m.KeyMap.CyclePrev):
			return m.cycleFocus(-1)
		case key.Matches(msg, m.KeyMap.ToggleFullscreen):
			return m.toggleFullscreen()
		case key.Matches(msg, m.KeyMap.ToggleDirection):
			return m.toggleDirection()
		}

		// Route unhandled keys to active pane only
		return m.updateActivePane(msg)
	}

	// Non-key, non-resize messages broadcast to ALL panes so background
	// tasks (tickers, streams) work in unfocused panes.
	return m.updateAllPanes(msg)
}

// distributeSize calculates dimensions and sends WindowSizeMsg to all panes.
func (m Manager) distributeSize() (Manager, tea.Cmd) {
	dims := m.calculateDimensions()
	var cmds []tea.Cmd
	for i := range m.Panes {
		if i < len(dims) {
			updated, cmd := m.Panes[i].Model.Update(dims[i])
			m.Panes[i].Model = updated
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	}
	return m, tea.Batch(cmds...)
}

// updateActivePane routes a message to the active pane.
func (m Manager) updateActivePane(msg tea.Msg) (Manager, tea.Cmd) {
	if m.ActivePaneIdx < 0 || m.ActivePaneIdx >= len(m.Panes) {
		return m, nil
	}
	updated, cmd := m.Panes[m.ActivePaneIdx].Model.Update(msg)
	m.Panes[m.ActivePaneIdx].Model = updated
	return m, cmd
}

// updateTargeted routes a TargetedMsg payload to the pane matching TargetID.
func (m Manager) updateTargeted(msg TargetedMsg) (Manager, tea.Cmd) {
	for i := range m.Panes {
		if m.Panes[i].ID == msg.TargetID {
			updated, cmd := m.Panes[i].Model.Update(msg.Payload)
			m.Panes[i].Model = updated
			return m, cmd
		}
	}
	return m, nil
}

// updateAllPanes sends a message to every pane and collects commands.
func (m Manager) updateAllPanes(msg tea.Msg) (Manager, tea.Cmd) {
	var cmds []tea.Cmd
	for i := range m.Panes {
		updated, cmd := m.Panes[i].Model.Update(msg)
		m.Panes[i].Model = updated
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return m, tea.Batch(cmds...)
}

// cycleFocus moves focus by delta (+1 or -1), skipping hidden panes.
func (m Manager) cycleFocus(delta int) (Manager, tea.Cmd) {
	if len(m.Panes) <= 1 {
		return m, nil
	}

	var cmds []tea.Cmd

	// Blur current
	if f, ok := m.Panes[m.ActivePaneIdx].Model.(Focusable); ok {
		f.Blur()
	}

	// Advance, skipping hidden panes
	n := len(m.Panes)
	for range n {
		m.ActivePaneIdx = (m.ActivePaneIdx + delta + n) % n
		if !m.Panes[m.ActivePaneIdx].Hidden {
			break
		}
	}

	// Exit fullscreen on focus change
	if m.FullscreenIdx >= 0 {
		m.FullscreenIdx = -1
		var cmd tea.Cmd
		m, cmd = m.distributeSize()
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	// Focus new
	if f, ok := m.Panes[m.ActivePaneIdx].Model.(Focusable); ok {
		if cmd := f.Focus(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

// toggleFullscreen switches between fullscreen and normal mode.
func (m Manager) toggleFullscreen() (Manager, tea.Cmd) {
	if m.FullscreenIdx >= 0 {
		m.FullscreenIdx = -1
		return m.distributeSize()
	}
	m.FullscreenIdx = m.ActivePaneIdx
	fullSize := tea.WindowSizeMsg{Width: m.Width, Height: m.Height}
	updated, cmd := m.Panes[m.ActivePaneIdx].Model.Update(fullSize)
	m.Panes[m.ActivePaneIdx].Model = updated
	return m, cmd
}

// toggleDirection swaps between horizontal and vertical splits.
func (m Manager) toggleDirection() (Manager, tea.Cmd) {
	if m.Direction == DirectionHorizontal {
		m.Direction = DirectionVertical
	} else {
		m.Direction = DirectionHorizontal
	}
	return m.distributeSize()
}
