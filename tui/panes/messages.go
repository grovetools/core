package panes

import tea "github.com/charmbracelet/bubbletea"

// TargetedMsg delivers a payload to a specific pane by ID.
// The Manager unwraps the envelope and calls Update on the target pane.
type TargetedMsg struct {
	TargetID string
	Payload  tea.Msg
}

// BroadcastMsg delivers a payload to all panes.
// The Manager unwraps the envelope and calls Update on every pane.
type BroadcastMsg struct {
	Payload tea.Msg
}

// SendCmd returns a tea.Cmd that emits a TargetedMsg.
// Use this from a pane's Update to send a message to a sibling pane
// without coupling to it directly.
func SendCmd(targetID string, msg tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return TargetedMsg{TargetID: targetID, Payload: msg}
	}
}

// BroadcastCmd returns a tea.Cmd that emits a BroadcastMsg.
// Use this from a pane's Update to notify all sibling panes.
func BroadcastCmd(msg tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return BroadcastMsg{Payload: msg}
	}
}
