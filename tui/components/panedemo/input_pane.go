package panedemo

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/grovetools/core/tui/theme"
)

// inputPane wraps a text input and shows submitted messages.
// It implements both Focusable and TextInputActive.
type inputPane struct {
	input    textinput.Model
	messages []string
	width    int
	height   int
	focused  bool
	editing  bool // when true, layout keys are suppressed
}

func newInputPane() *inputPane {
	ti := textinput.New()
	ti.Placeholder = "Type a message..."
	ti.CharLimit = 256

	return &inputPane{
		input: ti,
		messages: []string{
			"Welcome to the pane demo!",
			"Press Enter to submit, Esc to stop editing.",
		},
	}
}

func (p *inputPane) Init() tea.Cmd { return nil }

func (p *inputPane) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width = msg.Width
		p.height = msg.Height
		p.input.Width = max(msg.Width-4, 10)

	case tea.KeyMsg:
		if p.editing {
			switch msg.String() {
			case "enter":
				val := p.input.Value()
				if val != "" {
					p.messages = append(p.messages, val)
					p.input.SetValue("")
				}
				return p, nil
			case "esc":
				p.editing = false
				p.input.Blur()
				return p, nil
			}
		} else if p.focused {
			switch msg.String() {
			case "i", "enter":
				p.editing = true
				p.input.Focus()
				return p, p.input.Cursor.BlinkCmd()
			}
		}
	}

	if p.editing {
		var cmd tea.Cmd
		p.input, cmd = p.input.Update(msg)
		return p, cmd
	}

	return p, nil
}

func (p *inputPane) View() string {
	t := theme.DefaultTheme

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Padding(0, 1)
	if p.focused {
		titleStyle = titleStyle.Foreground(theme.DefaultColors.Orange)
	} else {
		titleStyle = titleStyle.Foreground(theme.DefaultColors.Blue)
	}

	title := "Input"
	var hint string
	if p.editing {
		hint = " (editing)"
	} else if p.focused {
		hint = " (i=edit)"
	}
	header := titleStyle.Render(title)
	if hint != "" {
		if p.editing {
			header += t.Warning.Render(hint)
		} else {
			header += t.Muted.Render(hint)
		}
	}

	// Messages area — chrome = header(1) + separator(1) + input(1)
	msgHeight := max(p.height-3, 1)
	var visibleMsgs []string
	start := 0
	if len(p.messages) > msgHeight {
		start = len(p.messages) - msgHeight
	}
	for _, m := range p.messages[start:] {
		line := "  " + m
		// Truncate to pane width to prevent wrapping
		if p.width > 0 && len(line) > p.width {
			line = line[:p.width]
		}
		visibleMsgs = append(visibleMsgs, t.Normal.Render(line))
	}
	// Pad to fill space
	for len(visibleMsgs) < msgHeight {
		visibleMsgs = append(visibleMsgs, "")
	}
	msgArea := strings.Join(visibleMsgs, "\n")

	// Input line
	inputLine := "  " + p.input.View()

	// Constrain separator to pane width
	sep := t.Muted.Render(strings.Repeat("─", max(p.width, 0)))

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		msgArea,
		sep,
		inputLine,
	)
}

// IsTextEntryActive implements panes.TextInputActive.
// When editing, layout keys (Tab, z, V) are suppressed.
func (p *inputPane) IsTextEntryActive() bool {
	return p.editing
}

func (p *inputPane) Focus() tea.Cmd {
	p.focused = true
	return nil
}

func (p *inputPane) Blur() {
	p.focused = false
	p.editing = false
	p.input.Blur()
}
