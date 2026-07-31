// Package notes owns the note-summary drawer widget: the rollup of the
// nb-backed note index that the terminal multiplexer mounts as its "notes"
// pane.
//
// It lives here, beside the daemon client that serves the note index
// (core/pkg/daemon's GetNoteIndex → core/pkg/models.NoteIndexEntry), rather
// than in the host that mounts it. The host's remaining job is to fetch the
// index, summarize it into [GroupRow]s scoped to the active workspace, and hand
// this widget a reader for them — none of which requires it to know how a note
// group renders, how the cursor walks nested priority rows, or what Enter
// means.
//
// It is NOT in the nb repository, which would be the ideal home: the notebook's
// data reaches the multiplexer entirely through core's daemon client, and the
// multiplexer has no dependency on nb at all. Putting the widget in nb would
// mean creating one for the sake of a renderer. If that dependency is ever
// warranted for other reasons, this package moves wholesale — it imports
// nothing from the host.
//
// See github.com/grovetools/core/tui/widget for the contract, the terminology
// (widget / pane / page) and the message vocabulary.
package notes

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/grovetools/core/tui/theme"
	"github.com/grovetools/core/tui/widget"
	"github.com/grovetools/tuimux"
)

// GroupMeta is nb's display metadata for one note group.
type GroupMeta struct {
	Icon      string
	SortOrder int
}

// GroupRow is one row of the notes summary: a note group in the active scope,
// how many notes it holds, and how many of those are urgent. It is what the
// notes widget renders instead of a truncated list of individual notes — counts
// you can scan, with a jump into nb for the one you care about.
type GroupRow struct {
	// Workspace is the note's owning workspace NAME (nb's own identity for it),
	// which is what a jump into the notebook browser resolves against.
	Workspace string
	Group     string
	Icon      string
	Count     int
	// P0/P1 are the urgent sub-counts, rendered as nested rows so a workspace
	// with something on fire says so without being opened. Completed and
	// archived notes are excluded: they keep their old priority forever and
	// nothing is on fire about finished work.
	P0 int
	P1 int

	// SortOrder is the notebook's configured position for this group. Exported
	// because the summarizer that fills it lives in the host, on the far side
	// of a package boundary; nothing in this package reads it.
	SortOrder int
}

// OpenGroupMsg asks the host to open the notebook on one note group, with the
// rest of nb's tree folded shut. Priority, when set ("p0"/"p1"), narrows the
// jump further to that priority's bucket inside the group.
//
// It is the notes widget's only selection verb. The widget deliberately does
// NOT open individual notes: it is a summary of what is waiting in each
// section, and picking the specific note is work nb's browser already does far
// better than fifteen truncated titles in a 35-column drawer ever could.
type OpenGroupMsg struct {
	Workspace string
	Group     string
	Priority  string
}

// summaryRow is one selectable line of the widget: either a group row or one of
// its nested priority rows. Flattening them into a single cursor space is what
// lets j/k walk "inbox → its P0 → its P1 → issues" the way the tree reads.
type summaryRow struct {
	group GroupRow
	// priority is "" for the group row itself, or "p0"/"p1" for a nested
	// urgent-count row beneath it.
	priority string
}

// Panel renders a rollup of the nb-backed note index for the active workspace:
// one row per note group with its count, nested p0/p1 rows where urgent work
// exists, and Enter to jump into nb at that section.
//
// It replaced a flat list of the fifteen highest-priority notes. That list was
// the wrong shape for the space: fifteen rune-truncated titles in a narrow
// drawer told you neither what was waiting overall nor enough about any single
// note to act on it. Counts by section answer the first question at a glance,
// and the jump hands the second one to the tool built for it.
type Panel struct {
	tuimux.PanelBase
	viewport viewport.Model
	// groups reads the host's current summary. A reader rather than a snapshot:
	// the host re-summarizes whenever the note index or the active workspace
	// moves, and announces it with widget.UpdateMsg rather than a payload.
	groups func() []GroupRow
	rows   []summaryRow
	cursor int
	// cursorKey is the selected row's stable identity, so the selection follows
	// its group across a refresh that reorders or resizes the list rather than
	// staying pinned to a row number that now means something else.
	cursorKey string
	// hideHeading suppresses the in-body heading line while the host's
	// "title" focus style already draws the pane name in a title bar.
	// Zero value = heading shown, so direct construction stays correct.
	hideHeading bool
	// headerLines is how many lines the last refresh drew above the first row,
	// so cursor-follow scrolling can map a row index to a viewport line.
	headerLines int
}

// New builds the notes widget over a reader for the host's group summary. A nil
// reader is treated as an empty summary, so a directly constructed widget (a
// test, a standalone host) renders its empty state rather than panicking.
func New(groups func() []GroupRow) *Panel {
	return &Panel{
		viewport: viewport.New(0, 0),
		groups:   groups,
	}
}

// Spec is the widget's registration entry. The host supplies only the group
// reader; everything else about the widget — its glyph, its keys, what it says
// when there is nothing to show — is declared here.
func Spec(groups func() []GroupRow) widget.Spec {
	return widget.Spec{
		Name:  "notes",
		Glyph: "note",
		Build: func() tuimux.Panel { return New(groups) },
		EmptyReason: func() string {
			if groups == nil || len(groups()) == 0 {
				return "no notes in this workspace's notebook"
			}
			return ""
		},
		Keymap: Keymap,
	}
}

// Keymap declares the widget's bindings. Enter is the only verb: the summary
// hands off to nb rather than trying to be a note browser in 35 columns.
func Keymap() []widget.KeyBinding {
	return []widget.KeyBinding{
		{Key: "j/k", Desc: "move between groups and their urgent rows"},
		{Key: "g/G", Desc: "first / last row"},
		{Key: "enter", Desc: "open the notebook at this group", When: "on a group row"},
		{Key: "enter", Desc: "open the notebook at this priority", When: "on a P0/P1 row"},
		{Key: "q/esc", Desc: "leave the drawer, keep it open"},
	}
}

// Keymap answers for the LIVE pane, implementing [widget.Keymapper]: Enter
// means two different things and which one is live depends entirely on the row
// under the cursor, so only the mounted panel can say.
func (p *Panel) Keymap() []widget.KeyBinding {
	bindings := Keymap()
	for i := range bindings {
		switch bindings[i].When {
		case "on a group row":
			bindings[i].Active = func() bool {
				row, ok := p.selected()
				return ok && row.priority == ""
			}
		case "on a P0/P1 row":
			bindings[i].Active = func() bool {
				row, ok := p.selected()
				return ok && row.priority != ""
			}
		}
	}
	return bindings
}

func (p *Panel) currentGroups() []GroupRow {
	if p.groups == nil {
		return nil
	}
	return p.groups()
}

func (p *Panel) Init() tea.Cmd { return nil }

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
			p.rememberCursor()
			p.refresh()
		case "down", "j":
			if p.cursor < len(p.rows)-1 {
				p.cursor++
			}
			p.rememberCursor()
			p.refresh()
		case "g", "home":
			p.cursor = 0
			p.rememberCursor()
			p.refresh()
		case "G", "end":
			p.cursor = max(len(p.rows)-1, 0)
			p.rememberCursor()
			p.refresh()
		case "enter":
			if row, ok := p.selected(); ok {
				open := OpenGroupMsg{
					Workspace: row.group.Workspace,
					Group:     row.group.Group,
					Priority:  row.priority,
				}
				cmd = func() tea.Msg { return open }
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

// GetTitle returns the bare pane name — what the "title" focus-style bar
// and any tab UI shows. The note count lives in headingText, not here.
func (p *Panel) GetTitle() string {
	return "Notes"
}

// HeadingText is the in-body heading: the bare title plus the total note
// count across the summarized groups. [Panel.GetTitle] is the bare name that
// the host's title bar draws; this is the line the body draws when the host is
// not drawing one.
//
// No scope label, because there is no scope to choose: notes are always the
// active worktree's own notebook. Carrying the agents pane's "all" here was
// what made a 400-note rollup of every notebook in the grove look like this
// worktree's backlog — which is a claim worth keeping a host-side test on, and
// why this is exported.
func (p *Panel) HeadingText() string {
	total := 0
	for _, g := range p.currentGroups() {
		total += g.Count
	}
	return fmt.Sprintf("%s (%d)", p.GetTitle(), total)
}

// SetShowHeading toggles the in-body heading line. The host turns it off
// while the "title" focus style draws a per-pane title bar, so the name
// never appears twice.
func (p *Panel) SetShowHeading(show bool) {
	p.hideHeading = !show
	p.refresh()
}

// rowKey is a row's stable identity across refreshes.
func rowKey(r summaryRow) string {
	return r.group.Workspace + "\x00" + r.group.Group + "\x00" + r.priority
}

func (p *Panel) selected() (summaryRow, bool) {
	if p.cursor < 0 || p.cursor >= len(p.rows) {
		return summaryRow{}, false
	}
	return p.rows[p.cursor], true
}

func (p *Panel) clampCursor() {
	if p.cursor >= len(p.rows) {
		p.cursor = len(p.rows) - 1
	}
	if p.cursor < 0 {
		p.cursor = 0
	}
}

func (p *Panel) rememberCursor() {
	p.clampCursor()
	if row, ok := p.selected(); ok {
		p.cursorKey = rowKey(row)
		return
	}
	p.cursorKey = ""
}

func (p *Panel) restoreCursor() {
	if p.cursorKey != "" {
		for i, row := range p.rows {
			if rowKey(row) == p.cursorKey {
				p.cursor = i
				return
			}
		}
	}
	p.rememberCursor()
}

// buildRows flattens the host's group rollup into the widget's cursor space:
// each group followed by its urgent sub-counts, urgent first.
func (p *Panel) buildRows() {
	groups := p.currentGroups()
	rows := make([]summaryRow, 0, len(groups))
	for _, g := range groups {
		rows = append(rows, summaryRow{group: g})
		if g.P0 > 0 {
			rows = append(rows, summaryRow{group: g, priority: "p0"})
		}
		if g.P1 > 0 {
			rows = append(rows, summaryRow{group: g, priority: "p1"})
		}
	}
	p.rows = rows
}

// priorityStyle colors an urgent sub-row, matching the priority badge ladder
// used everywhere else in the drawer. Read at render time so a live re-theme
// self-heals.
func priorityStyle(priority string) lipgloss.Style {
	th := theme.DefaultTheme
	if priority == "p0" {
		return th.Error
	}
	return th.WarningLight
}

func (p *Panel) refresh() {
	if p.viewport.Width == 0 {
		return
	}
	p.buildRows()
	p.restoreCursor()

	var b strings.Builder
	lines := 0
	write := func(s string) {
		if lines > 0 {
			b.WriteString("\n")
		}
		b.WriteString(s)
		lines++
	}

	th := theme.DefaultTheme
	if !p.hideHeading {
		write(widget.HighlightStyle().Bold(true).Render(p.HeadingText()))
		write("")
	}
	p.headerLines = lines

	if len(p.rows) == 0 {
		write(th.Muted.Render("No notes in scope."))
		p.viewport.SetContent(b.String())
		return
	}

	// One workspace heading per run of rows: even worktree-scoped, notes can
	// span two notebooks (the worktree's own plus the plan-linked ones in its
	// ecosystem's). In the common single-workspace case the name is still worth
	// its line: it is what the jump into nb resolves against.
	lastWorkspace := ""
	for i, row := range p.rows {
		if row.group.Workspace != lastWorkspace {
			lastWorkspace = row.group.Workspace
			// A workspace heading is not selectable, so it must not shift the
			// row/line mapping scrollToCursor relies on — headerLines only
			// covers the block above the FIRST row, so anything interleaved
			// afterwards is accounted for by rendering it as part of this row's
			// entry and tracking the offset below.
			write(th.Info.Render(widget.Truncate(theme.IconRepo+" "+lastWorkspace, p.viewport.Width)))
		}
		write(p.renderRow(row, i == p.cursor))
	}
	p.viewport.SetContent(b.String())
	p.scrollToCursor()
}

// renderRow draws one summary line: "<icon> <name>   (<count>)", with urgent
// sub-rows indented under their group and colored by priority.
func (p *Panel) renderRow(row summaryRow, selected bool) string {
	th := theme.DefaultTheme
	w := p.viewport.Width

	indent := " "
	icon := row.group.Icon
	if icon == "" {
		icon = theme.IconFolder
	}
	name := row.group.Group
	count := row.group.Count
	style := lipgloss.NewStyle()

	if row.priority != "" {
		indent = "   "
		icon = theme.IconFire
		name = strings.ToUpper(row.priority)
		style = priorityStyle(row.priority)
		if row.priority == "p0" {
			count = row.group.P0
		} else {
			count = row.group.P1
		}
	}

	countText := "(" + strconv.Itoa(count) + ")"
	left := indent + icon + " " + name
	// The count is right-aligned so the column reads as a column; the name
	// yields to it rather than the reverse, since the count is the whole point
	// of the row.
	pad := max(w-lipgloss.Width(left)-lipgloss.Width(countText), 1)
	if pad == 1 {
		left = widget.Truncate(left, max(w-lipgloss.Width(countText)-1, 1))
		pad = max(w-lipgloss.Width(left)-lipgloss.Width(countText), 1)
	}
	plain := left + strings.Repeat(" ", pad) + countText
	if selected {
		return widget.HighlightStyle().Render(plain)
	}
	if row.priority != "" {
		return style.Render(plain)
	}
	return style.Render(left) + strings.Repeat(" ", pad) + th.Muted.Render(countText)
}

// scrollToCursor nudges the viewport so the selected row stays visible. The
// mapping counts the interleaved workspace headings, which occupy lines but are
// not selectable rows.
func (p *Panel) scrollToCursor() {
	if p.viewport.Height <= 0 || len(p.rows) == 0 {
		return
	}
	line := p.headerLines
	lastWorkspace := ""
	for i, row := range p.rows {
		if row.group.Workspace != lastWorkspace {
			lastWorkspace = row.group.Workspace
			line++
		}
		if i == p.cursor {
			break
		}
		line++
	}
	if line < p.viewport.YOffset {
		p.viewport.SetYOffset(line)
		return
	}
	if line >= p.viewport.YOffset+p.viewport.Height {
		p.viewport.SetYOffset(line - p.viewport.Height + 1)
	}
}
