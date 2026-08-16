// Package notes owns the openable note-list widget used by drawer hosts.
package notes

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/grovetools/core/tui/theme"
	"github.com/grovetools/core/tui/widget"
	"github.com/grovetools/tuimux"
)

// GroupMeta and GroupRow remain the host-side rollup vocabulary used by older
// palette surfaces. The drawer widget itself now consumes individual NoteRows.
type GroupMeta struct {
	Icon      string
	SortOrder int
}
type GroupRow struct {
	Workspace, Group, Icon   string
	Count, P0, P1, SortOrder int
}

// NoteRow is one openable note from the daemon note index.
type NoteRow struct {
	ID, Title, Path, Workspace, Group, Priority string
}

// Scope is the drawer's four-state note filter. Its zero value intentionally
// starts at the narrowest useful view.
type Scope uint8

const (
	ScopePlan Scope = iota
	ScopeNotespace
	ScopeUnlinked
	ScopeAll
)

func (s Scope) Label() string {
	switch s {
	case ScopePlan:
		return "this plan"
	case ScopeNotespace:
		return "this notespace"
	case ScopeUnlinked:
		return "unlinked"
	default:
		return "all notespaces"
	}
}
func (s Scope) Next() Scope { return (s + 1) % 4 }

// CycleScopeMsg asks the host (which owns index filtering) to advance scope.
type CycleScopeMsg struct{}

// OpenNoteMsg asks the host to open one note in its editor surface.
type OpenNoteMsg struct{ Path string }

const emptyReason = "no notes in this scope"

type Panel struct {
	tuimux.PanelBase
	viewport    viewport.Model
	notes       func() []NoteRow
	scope       func() Scope
	rows        []NoteRow
	cursor      int
	cursorKey   string
	hideHeading bool
	headerLines int
}

func New(notes func() []NoteRow, scope func() Scope) *Panel {
	return &Panel{viewport: viewport.New(0, 0), notes: notes, scope: scope}
}

func Spec(notes func() []NoteRow, scope func() Scope) widget.Spec {
	return widget.Spec{
		Name: "notes", Glyph: "note", Build: func() tuimux.Panel { return New(notes, scope) },
		EmptyReason: func() string {
			if notes == nil || len(notes()) == 0 {
				return emptyReason
			}
			return ""
		},
		Keymap: Keymap,
	}
}

func Keymap() []widget.KeyBinding {
	return []widget.KeyBinding{
		{Key: "j/k", Desc: "move between notes"},
		{Key: "g/G", Desc: "first / last note"},
		{Key: "enter", Desc: "open note"},
		{Key: "0/w", Desc: "cycle note scope"},
		{Key: "q/esc", Desc: "leave the drawer, keep it open"},
	}
}
func (p *Panel) Keymap() []widget.KeyBinding { return Keymap() }
func (p *Panel) Init() tea.Cmd               { return nil }
func (p *Panel) currentNotes() []NoteRow {
	if p.notes == nil {
		return nil
	}
	return p.notes()
}
func (p *Panel) currentScope() Scope {
	if p.scope == nil {
		return ScopePlan
	}
	return p.scope()
}
func (p *Panel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case widget.UpdateMsg:
		p.refresh()
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if p.cursor > 0 {
				p.cursor--
			}
			p.remember()
			p.refresh()
		case "down", "j":
			if p.cursor < len(p.rows)-1 {
				p.cursor++
			}
			p.remember()
			p.refresh()
		case "g", "home":
			p.cursor = 0
			p.remember()
			p.refresh()
		case "G", "end":
			p.cursor = max(len(p.rows)-1, 0)
			p.remember()
			p.refresh()
		case "0", "w":
			cmd = func() tea.Msg { return CycleScopeMsg{} }
		case "enter":
			if r, ok := p.selected(); ok && r.Path != "" {
				path := r.Path
				cmd = func() tea.Msg { return OpenNoteMsg{Path: path} }
			}
		case "q", "esc":
			cmd = func() tea.Msg { return widget.LeaveFocusMsg{} }
		default:
			p.viewport, cmd = p.viewport.Update(msg)
		}
	default:
		p.viewport, cmd = p.viewport.Update(msg)
	}
	return p, cmd
}
func (p *Panel) View() string { return p.viewport.View() }
func (p *Panel) Resize(w, h int) {
	p.PanelBase.Resize(w, h)
	p.viewport.Width = w
	p.viewport.Height = h
	p.refresh()
}
func (p *Panel) GetTitle() string { return "Notes" }
func (p *Panel) HeadingText() string {
	return fmt.Sprintf("Notes (%d) · %s", len(p.currentNotes()), p.currentScope().Label())
}
func (p *Panel) SetShowHeading(show bool) { p.hideHeading = !show; p.refresh() }
func (p *Panel) PreferredHeightHint() (int, bool) {
	if len(p.currentNotes()) > 0 {
		return 0, true
	}
	if p.hideHeading {
		return 2, false // compact scope label + empty reason
	}
	return 3, false
}

func noteKey(r NoteRow) string {
	if r.ID != "" {
		return r.ID
	}
	return r.Path
}
func (p *Panel) selected() (NoteRow, bool) {
	if p.cursor < 0 || p.cursor >= len(p.rows) {
		return NoteRow{}, false
	}
	return p.rows[p.cursor], true
}
func (p *Panel) remember() {
	if len(p.rows) == 0 {
		p.cursor = 0
		p.cursorKey = ""
		return
	}
	p.cursor = min(max(p.cursor, 0), len(p.rows)-1)
	p.cursorKey = noteKey(p.rows[p.cursor])
}
func (p *Panel) restore() {
	if p.cursorKey != "" {
		for i, r := range p.rows {
			if noteKey(r) == p.cursorKey {
				p.cursor = i
				return
			}
		}
	}
	p.remember()
}
func (p *Panel) buildRows() {
	p.rows = append(p.rows[:0], p.currentNotes()...)
	sort.SliceStable(p.rows, func(i, j int) bool {
		rank := func(v string) int {
			switch v {
			case "p0":
				return 0
			case "p1":
				return 1
			case "p2":
				return 2
			case "p3":
				return 3
			default:
				return 4
			}
		}
		if rank(p.rows[i].Priority) != rank(p.rows[j].Priority) {
			return rank(p.rows[i].Priority) < rank(p.rows[j].Priority)
		}
		if p.rows[i].Workspace != p.rows[j].Workspace {
			return p.rows[i].Workspace < p.rows[j].Workspace
		}
		return strings.ToLower(p.rows[i].Title) < strings.ToLower(p.rows[j].Title)
	})
}
func (p *Panel) refresh() {
	if p.viewport.Width == 0 {
		return
	}
	p.buildRows()
	p.restore()
	var b strings.Builder
	lines := 0
	write := func(s string) {
		if lines > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(s)
		lines++
	}
	if !p.hideHeading {
		write(widget.HighlightStyle().Bold(true).Render(p.HeadingText()))
		write("")
	} else {
		// The host's title-style chrome already says "Notes", but the active
		// scope still has to remain visible for a four-state cycle to be usable.
		write(theme.DefaultTheme.Muted.Render("scope: " + p.currentScope().Label()))
	}
	p.headerLines = lines
	if len(p.rows) == 0 {
		write(widget.RenderEmptyReason(emptyReason))
		p.viewport.SetContent(b.String())
		return
	}
	lastWS := ""
	for i, r := range p.rows {
		if r.Workspace != lastWS {
			lastWS = r.Workspace
			write(theme.DefaultTheme.Info.Render(widget.Truncate(theme.IconRepo+" "+lastWS, p.viewport.Width)))
		}
		write(p.renderRow(r, i == p.cursor))
	}
	p.viewport.SetContent(b.String())
	p.scrollToCursor()
}
func (p *Panel) renderRow(r NoteRow, selected bool) string {
	prefix := "  "
	style := lipgloss.NewStyle()
	if r.Priority != "" {
		prefix = strings.ToUpper(r.Priority) + " "
		if r.Priority == "p0" {
			style = theme.DefaultTheme.Error
		} else {
			style = theme.DefaultTheme.WarningLight
		}
	}
	line := widget.Truncate(prefix+r.Title, p.viewport.Width)
	if selected {
		return widget.HighlightStyle().Render(line)
	}
	return style.Render(line)
}
func (p *Panel) scrollToCursor() {
	if p.viewport.Height <= 0 || len(p.rows) == 0 {
		return
	}
	line := p.headerLines
	last := ""
	for i, r := range p.rows {
		if r.Workspace != last {
			last = r.Workspace
			line++
		}
		if i == p.cursor {
			break
		}
		line++
	}
	if line < p.viewport.YOffset {
		p.viewport.SetYOffset(line)
	} else if line >= p.viewport.YOffset+p.viewport.Height {
		p.viewport.SetYOffset(line - p.viewport.Height + 1)
	}
}
