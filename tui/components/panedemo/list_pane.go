package panedemo

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/grovetools/core/tui/embed"
	"github.com/grovetools/core/tui/panes"
	"github.com/grovetools/core/tui/theme"
)

// listItem implements list.Item and list.DefaultItem.
type listItem struct {
	title string
	desc  string
}

func (i listItem) Title() string       { return i.title }
func (i listItem) Description() string { return i.desc }
func (i listItem) FilterValue() string { return i.title }

// listDelegate renders list items.
type listDelegate struct{}

func (d listDelegate) Height() int                             { return 2 }
func (d listDelegate) Spacing() int                            { return 0 }
func (d listDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d listDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	i, ok := item.(listItem)
	if !ok {
		return
	}

	t := theme.DefaultTheme
	var str string
	if index == m.Index() {
		str = t.Selected.Render("▸ "+i.title) + "\n" +
			t.Muted.Render("  "+i.desc)
	} else {
		str = t.Normal.Render("  "+i.title) + "\n" +
			t.Muted.Render("  "+i.desc)
	}
	fmt.Fprint(w, str)
}

// listPane wraps a bubbles/list model as a pane.
type listPane struct {
	list   list.Model
	width  int
	height int
}

func newListPane() *listPane {
	items := []list.Item{
		listItem{"Job: build-core", "Running for 2m30s"},
		listItem{"Job: test-unit", "Queued"},
		listItem{"Job: lint-check", "Completed in 45s"},
		listItem{"Job: deploy-staging", "Waiting on deps"},
		listItem{"Job: e2e-tests", "Scheduled"},
		listItem{"Job: build-docs", "Running for 1m10s"},
		listItem{"Job: security-scan", "Completed in 22s"},
		listItem{"Job: perf-bench", "Queued"},
	}

	l := list.New(items, listDelegate{}, 0, 0)
	l.Title = "Jobs"
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = lipgloss.NewStyle().
		Foreground(theme.DefaultColors.Blue).
		Bold(true).
		Padding(0, 1)

	return &listPane{list: l}
}

// itemSelectedMsg is emitted when the user presses Enter on a list item.
// The logs pane and preview pane react to this to update their content.
type itemSelectedMsg struct {
	Title string
	Desc  string
}

func (p *listPane) Init() tea.Cmd { return nil }

func (p *listPane) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width = msg.Width
		p.height = msg.Height
		p.list.SetSize(msg.Width, msg.Height)
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if item, ok := p.list.SelectedItem().(listItem); ok {
				sel := itemSelectedMsg{Title: item.title, Desc: item.desc}
				return p, tea.Batch(
					panes.SendCmd("logs", sel),
					panes.SendCmd("preview", sel),
				)
			}
		case "e":
			// Open the selected item's "file" in an editor split
			if item, ok := p.list.SelectedItem().(listItem); ok {
				return p, func() tea.Msg {
					return embed.SplitEditorRequestMsg{
						Path: "/tmp/grove-demo-" + item.title + ".md",
					}
				}
			}
		}
	}

	var cmd tea.Cmd
	p.list, cmd = p.list.Update(msg)
	return p, cmd
}

func (p *listPane) View() string {
	return p.list.View()
}

func (p *listPane) Focus() tea.Cmd {
	p.list.Styles.Title = lipgloss.NewStyle().
		Foreground(theme.DefaultColors.Orange).
		Bold(true).
		Padding(0, 1)
	return nil
}

func (p *listPane) Blur() {
	p.list.Styles.Title = lipgloss.NewStyle().
		Foreground(theme.DefaultColors.Blue).
		Bold(true).
		Padding(0, 1)
}
