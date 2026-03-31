package chatpane

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/grovetools/core/tui/components/logviewer"
)

// Model combines a scrolling log viewer with a text input box and status bar.
// It provides a reusable chat pane pattern for agent transcript viewing + input.
type Model struct {
	LogViewer   logviewer.Model
	Input       textinput.Model
	StatusText  string // Optional status line above input (e.g., "Thinking... 8s")
	InputActive bool   // Whether input is focused
	Placeholder string // Input placeholder text
	Width       int
	Height      int
}

// InputBoxHeight is the total lines taken by the input box (border + content + border).
const InputBoxHeight = 3

// StatusLineHeight is the height of the optional status line.
const StatusLineHeight = 1

// New creates a ChatPane with the given dimensions.
func New(width, height int, placeholder string) Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.CharLimit = 4096
	ti.Width = width - 6 // Account for border + padding

	logHeight := height - InputBoxHeight
	if logHeight < 1 {
		logHeight = 1
	}

	return Model{
		LogViewer:   logviewer.New(width, logHeight),
		Input:       ti,
		Placeholder: placeholder,
		Width:       width,
		Height:      height,
	}
}

// SetStatusText updates the optional status line (e.g. from agent status polling).
func (m *Model) SetStatusText(text string) {
	m.StatusText = text
}

// FocusInput activates the text input.
func (m *Model) FocusInput() tea.Cmd {
	m.InputActive = true
	return m.Input.Focus()
}

// BlurInput deactivates the text input.
func (m *Model) BlurInput() {
	m.InputActive = false
	m.Input.Blur()
}

// AppendLine adds a formatted line to the transcript viewer.
func (m *Model) AppendLine(line string) {
	// Send as a LogLineMsg with NoPrefix since these are pre-formatted agent transcript lines
	m.LogViewer.Update(logviewer.LogLineMsg{Line: line, NoPrefix: true})
}

// Resize updates the dimensions of the chat pane.
func (m *Model) Resize(width, height int) {
	m.Width = width
	m.Height = height
	m.Input.Width = width - 6

	logHeight := m.logViewerHeight()
	m.LogViewer, _ = m.LogViewer.Update(tea.WindowSizeMsg{Width: width, Height: logHeight})
}

// logViewerHeight calculates the height available for the log viewer.
func (m *Model) logViewerHeight() int {
	h := m.Height - InputBoxHeight
	if m.StatusText != "" {
		h -= StatusLineHeight
	}
	if h < 1 {
		h = 1
	}
	return h
}
