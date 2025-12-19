package logviewer

import (
	"encoding/json"
	"fmt"
	"io"
	stdlog "log"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/hpcloud/tail"
	"github.com/mattsolo1/grove-core/tui/theme"
	"github.com/mattsolo1/grove-core/tui/utils/scrollbar"
)

// LogLineMsg is sent when a new log line is received.
type LogLineMsg struct {
	Workspace string
	Line      string
	NoPrefix  bool // If true, formatLogLine will not add a workspace prefix
}

// Model is the TUI component for viewing logs.
type Model struct {
	viewport   viewport.Model
	tails      []*tail.Tail
	mu         sync.Mutex
	follow     bool
	ready      bool
	width      int
	height     int
	logChannel chan LogLineMsg
	lines      []string
}

// New creates a new log viewer model.
func New(width, height int) Model {
	vp := viewport.New(width, height-1) // Leave space for a status bar
	return Model{
		viewport:   vp,
		follow:     true,
		width:      width,
		height:     height,
		logChannel: make(chan LogLineMsg, 100),
		lines:      []string{},
	}
}

// Start begins tailing the specified log files.
func (m *Model) Start(files map[string]string) tea.Cmd {
	m.Stop() // Stop any existing tails
	m.mu.Lock()
	defer m.mu.Unlock()

	for workspace, path := range files {
		config := tail.Config{
			Follow:   true,
			ReOpen:   true,
			Location: &tail.SeekInfo{Offset: 0, Whence: io.SeekStart},
			Logger:   stdlog.New(io.Discard, "", 0),
		}
		t, err := tail.TailFile(path, config)
		if err != nil {
			// In a real app, we might send an error message
			continue
		}
		m.tails = append(m.tails, t)

		go func(ws string, t *tail.Tail) {
			for line := range t.Lines {
				m.logChannel <- LogLineMsg{Workspace: ws, Line: line.Text}
			}
		}(workspace, t)
	}

	return m.waitForLogLine()
}

// Stop halts all tailing operations.
func (m *Model) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.tails {
		t.Stop()
	}
	m.tails = nil
}

// setWrappedContent wraps the content to the viewport's current width.
func (m *Model) setWrappedContent() {
	if !m.ready {
		return
	}

	// Wrap to viewport width - 1 for scrollbar.
	// The viewport itself is set to m.width, but we need room for the scrollbar.
	wrapWidth := m.viewport.Width - 1
	if wrapWidth < 1 {
		wrapWidth = 1
	}

	wrapStyle := lipgloss.NewStyle().Width(wrapWidth)

	var wrappedLines []string
	for _, line := range m.lines {
		wrappedLines = append(wrappedLines, wrapStyle.Render(line))
	}

	m.viewport.SetContent(strings.Join(wrappedLines, "\n"))
}

// generateScrollbar creates a visual scrollbar based on viewport position.
// Deprecated: Use scrollbar.Generate directly instead.
func (m *Model) generateScrollbar(height int) []string {
	if !m.ready {
		return []string{}
	}
	return scrollbar.Generate(&m.viewport, height)
}

// SetContent displays static content, stopping any live tailing.
func (m *Model) SetContent(content string) {
	m.Stop()
	m.lines = strings.Split(content, "\n")
	m.setWrappedContent()
	m.viewport.GotoBottom()
}

// Clear stops tailing and clears the viewer's content.
func (m *Model) Clear() {
	m.Stop()
	m.lines = []string{}
	m.viewport.SetContent("")
}

// GotoTop scrolls to the top of the log content.
func (m *Model) GotoTop() {
	m.viewport.GotoTop()
}

// GotoBottom scrolls to the bottom of the log content.
func (m *Model) GotoBottom() {
	m.viewport.GotoBottom()
}

// GetScrollInfo returns current scroll position information.
func (m *Model) GetScrollInfo() (currentLine, totalLines int) {
	totalLines = len(m.lines)
	if totalLines == 0 {
		return 0, 0
	}
	// YOffset is the top line being displayed
	currentLine = m.viewport.YOffset + 1 // +1 for human-readable (1-indexed)
	return currentLine, totalLines
}

// GetScrollPercent returns the scroll position as a percentage.
func (m *Model) GetScrollPercent() float64 {
	if len(m.lines) == 0 {
		return 0
	}
	return m.viewport.ScrollPercent()
}

func (m *Model) waitForLogLine() tea.Cmd {
	return func() tea.Msg {
		return <-m.logChannel
	}
}

// Init initializes the component.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 1
		m.ready = true
		m.setWrappedContent()
	case LogLineMsg:
		formattedLine := formatLogLine(msg.Workspace, msg.Line, msg.NoPrefix)
		m.lines = append(m.lines, formattedLine)
		m.setWrappedContent()
		if m.follow {
			m.viewport.GotoBottom()
		}
		cmds = append(cmds, m.waitForLogLine())
	case tea.KeyMsg:
		switch msg.String() {
		case "f":
			m.follow = !m.follow
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// View renders the log viewer with scrollbar.
func (m Model) View() string {
	if !m.ready {
		return "Initializing log viewer..."
	}

	// Get the viewport content (already wrapped to width - 1 in setWrappedContent)
	content := m.viewport.View()

	// Split content into lines
	lines := strings.Split(content, "\n")

	// Generate scrollbar for the actual number of lines displayed
	scrollbar := m.generateScrollbar(len(lines))

	// Overlay scrollbar on the right side of each line
	var result []string
	for i := 0; i < len(lines); i++ {
		line := lines[i]

		scrollbarChar := " "
		if i < len(scrollbar) {
			scrollbarChar = scrollbar[i]
		}

		// Simply append scrollbar - content is already wrapped to the correct width
		result = append(result, line+scrollbarChar)
	}

	return strings.Join(result, "\n")
}

// IsFollowing returns whether the log viewer is in follow mode.
func (m Model) IsFollowing() bool {
	return m.follow
}

// formatLogLine is a simplified log formatter for the TUI component.
func formatLogLine(workspace, line string, noPrefix bool) string {
	var logMap map[string]interface{}
	if err := json.Unmarshal([]byte(line), &logMap); err != nil {
		// If no prefix is requested, just return the raw line.
		if noPrefix {
			return line
		}
		// Pass through raw output without adding prefixes for Job Output and System
		if workspace == "Job Output" || workspace == "System" {
			return line
		}
		return fmt.Sprintf("[%s] %s", theme.DefaultTheme.Accent.Render(workspace), line)
	}

	msg, _ := logMap["msg"].(string)
	level, _ := logMap["level"].(string)
	ts, _ := logMap["time"].(string)

	// Parse time for formatting
	var timeStr string
	parsedTime, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		parsedTime, _ = time.Parse(time.RFC3339, ts)
	}
	if !parsedTime.IsZero() {
		timeStr = parsedTime.Format("15:04:05")
	}

	var levelStyle lipgloss.Style
	switch strings.ToLower(level) {
	case "error", "fatal":
		levelStyle = theme.DefaultTheme.Error
	case "warning":
		levelStyle = theme.DefaultTheme.Warning
	default:
		levelStyle = theme.DefaultTheme.Info
	}

	// Build the final log line parts
	var parts []string
	if timeStr != "" {
		parts = append(parts, timeStr)
	}
	if !noPrefix {
		parts = append(parts, fmt.Sprintf("[%s]", theme.DefaultTheme.Accent.Render(workspace)))
	}
	parts = append(parts, levelStyle.Render(strings.ToUpper(level))+":")
	parts = append(parts, msg)

	return strings.Join(parts, " ")
}
