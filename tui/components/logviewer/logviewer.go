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
)

// LogLineMsg is sent when a new log line is received.
type LogLineMsg struct {
	Workspace string
	Line      string
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
	case LogLineMsg:
		formattedLine := formatLogLine(msg.Workspace, msg.Line)
		m.lines = append(m.lines, formattedLine)
		m.viewport.SetContent(strings.Join(m.lines, "\n"))
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

// View renders the log viewer.
func (m Model) View() string {
	if !m.ready {
		return "Initializing log viewer..."
	}
	status := "Follow: ON"
	if !m.follow {
		status = "Follow: OFF"
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		m.viewport.View(),
		theme.DefaultTheme.Muted.Render(status),
	)
}

// formatLogLine is a simplified log formatter for the TUI component.
func formatLogLine(workspace, line string) string {
	var logMap map[string]interface{}
	if err := json.Unmarshal([]byte(line), &logMap); err != nil {
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

	if timeStr != "" {
		return fmt.Sprintf("%s [%s] %s: %s",
			timeStr,
			theme.DefaultTheme.Accent.Render(workspace),
			levelStyle.Render(strings.ToUpper(level)),
			msg,
		)
	}

	return fmt.Sprintf("[%s] %s: %s",
		theme.DefaultTheme.Accent.Render(workspace),
		levelStyle.Render(strings.ToUpper(level)),
		msg,
	)
}
