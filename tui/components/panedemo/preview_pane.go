package panedemo

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/grovetools/core/tui/theme"
)

// previewPane shows detail about the currently selected item.
// It starts hidden and can be toggled with the "p" key.
type previewPane struct {
	title   string
	desc    string
	width   int
	height  int
	focused bool
}

func newPreviewPane() *previewPane {
	return &previewPane{
		title: "(none)",
		desc:  "Press Enter on a list item to preview it here.",
	}
}

func (p *previewPane) Init() tea.Cmd { return nil }

func (p *previewPane) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width = msg.Width
		p.height = msg.Height
	case itemSelectedMsg:
		p.title = msg.Title
		p.desc = msg.Desc
	}
	return p, nil
}

func (p *previewPane) View() string {
	t := theme.DefaultTheme

	titleStyle := lipgloss.NewStyle().Bold(true).Padding(0, 1)
	if p.focused {
		titleStyle = titleStyle.Foreground(theme.DefaultColors.Orange)
	} else {
		titleStyle = titleStyle.Foreground(theme.DefaultColors.Blue)
	}

	header := titleStyle.Render("Preview")

	body := fmt.Sprintf("\n  %s\n  %s",
		t.Highlight.Render(p.title),
		t.Muted.Render(p.desc),
	)

	// Pad remaining height
	usedLines := 3 // header + 2 body lines
	remaining := max(p.height-usedLines, 0)
	body += strings.Repeat("\n", remaining)

	return lipgloss.JoinVertical(lipgloss.Left, header, body)
}

func (p *previewPane) Focus() tea.Cmd {
	p.focused = true
	return nil
}

func (p *previewPane) Blur() {
	p.focused = false
}
