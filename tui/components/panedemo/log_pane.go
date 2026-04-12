package panedemo

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/grovetools/core/tui/theme"
)

// tickMsg triggers a new simulated log line.
type tickMsg time.Time

// logPane simulates streaming log output in a viewport.
type logPane struct {
	viewport viewport.Model
	lines    []string
	width    int
	height   int
	focused  bool
	lineNum  int
}

func newLogPane() *logPane {
	vp := viewport.New(0, 0)
	vp.MouseWheelEnabled = true

	initialLines := []string{
		"[INFO]  Starting build pipeline...",
		"[INFO]  Resolving dependencies",
		"[DEBUG] Found 42 packages in workspace",
		"[INFO]  Compiling core/tui/panes",
		"[WARN]  Unused import in layout.go",
		"[INFO]  Compiling core/cmd/pane-demo",
		"[DEBUG] Linking binary...",
		"[INFO]  Build succeeded in 1.2s",
		"[INFO]  Running test suite",
		"[DEBUG] Test: TestLayout... PASS",
		"[DEBUG] Test: TestFocusCycle... PASS",
		"[INFO]  12/12 tests passed",
	}

	return &logPane{
		viewport: vp,
		lines:    initialLines,
		lineNum:  len(initialLines),
	}
}

func (p *logPane) Init() tea.Cmd {
	return tickCmd()
}

func tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (p *logPane) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width = msg.Width
		p.height = msg.Height
		headerH := 1
		p.viewport.Width = msg.Width
		p.viewport.Height = max(msg.Height-headerH, 0)
		p.updateContent()

	case itemSelectedMsg:
		// React to selection from the list pane — reset log content
		p.lines = []string{
			fmt.Sprintf("[INFO]  === Selected: %s ===", msg.Title),
			fmt.Sprintf("[INFO]  %s", msg.Desc),
			"[INFO]  Streaming logs for this job...",
		}
		p.lineNum = len(p.lines)
		p.updateContent()
		return p, nil

	case tickMsg:
		p.lineNum++
		levels := []string{"INFO", "DEBUG", "WARN", "INFO", "DEBUG", "INFO"}
		msgs := []string{
			"Processing batch",
			"Cache hit ratio: 94%",
			"Slow query detected",
			"Health check OK",
			"GC pause: 1.2ms",
			"Request completed",
		}
		level := levels[p.lineNum%len(levels)]
		text := msgs[p.lineNum%len(msgs)]
		line := fmt.Sprintf("[%-5s] %s #%d", level, text, p.lineNum)
		p.lines = append(p.lines, line)
		// Keep last 200 lines
		if len(p.lines) > 200 {
			p.lines = p.lines[len(p.lines)-200:]
		}
		p.updateContent()
		cmds = append(cmds, tickCmd())
	}

	var cmd tea.Cmd
	p.viewport, cmd = p.viewport.Update(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	return p, tea.Batch(cmds...)
}

func (p *logPane) updateContent() {
	t := theme.DefaultTheme
	var styled []string
	for _, line := range p.lines {
		switch {
		case strings.Contains(line, "[WARN"):
			styled = append(styled, t.Warning.Render(line))
		case strings.Contains(line, "[ERROR"):
			styled = append(styled, t.Error.Render(line))
		case strings.Contains(line, "[DEBUG"):
			styled = append(styled, t.Muted.Render(line))
		default:
			styled = append(styled, t.Normal.Render(line))
		}
	}
	p.viewport.SetContent(strings.Join(styled, "\n"))
	p.viewport.GotoBottom()
}

func (p *logPane) View() string {
	t := theme.DefaultTheme
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Padding(0, 1)
	if p.focused {
		titleStyle = titleStyle.Foreground(theme.DefaultColors.Orange)
	} else {
		titleStyle = titleStyle.Foreground(theme.DefaultColors.Blue)
	}

	header := titleStyle.Render("Logs") +
		t.Muted.Render(fmt.Sprintf(" (%d lines)", len(p.lines)))

	return lipgloss.JoinVertical(lipgloss.Left, header, p.viewport.View())
}

// StatusLine implements panes.StatusProvider.
func (p *logPane) StatusLine() string {
	follow := "ON"
	if p.viewport.AtBottom() {
		follow = "ON"
	} else {
		follow = "OFF"
	}
	return fmt.Sprintf(" Logs • %d lines • Follow: %s ", len(p.lines), follow)
}

func (p *logPane) Focus() tea.Cmd {
	p.focused = true
	return nil
}

func (p *logPane) Blur() {
	p.focused = false
}
