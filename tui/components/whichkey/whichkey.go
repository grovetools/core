// Package whichkey renders a which-key popup: a titled, optionally two-column
// list of pending key completions shown while a chord prefix is armed. It is a
// near-verbatim port of treemux's internal which-key renderer, promoted into
// core so every Grove TUI can render the same popup. It takes plain data
// (KeyRow/KeyGroup) plus a theme and imports only lipgloss and core's theme —
// no keymap dependency — so keymap.Namespace can import it without a cycle.
//
// Contrast note (from the treemux original): descriptions render in
// Colors.LightText, NOT MutedText. Keys are Violet bold, group titles bold,
// the popup title Orange bold.
package whichkey

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/grovetools/core/tui/theme"
)

// KeyRow is one binding row: a key column and its description.
type KeyRow struct {
	Keys string
	Desc string
}

// KeyGroup is a titled list of binding rows.
type KeyGroup struct {
	Title string
	Rows  []KeyRow
}

// keyGroupTwoColMinWidth is the floor width (columns) below which the layout
// never attempts two columns — a cheap early-out before the exact-fit
// measurement. Above it, two columns are used only if the packed block
// actually fits the available width.
const keyGroupTwoColMinWidth = 90

// keyGroupColGap is the space between the two group columns.
const keyGroupColGap = 4

// keyGroupStyles bundles the lipgloss styles the layout renders with, built
// from a theme with the contrast fix applied.
type keyGroupStyles struct {
	title lipgloss.Style // popup header
	group lipgloss.Style // group titles
	key   lipgloss.Style // key column
	desc  lipgloss.Style // description column (LightText, not muted)
}

func newKeyGroupStyles(t theme.Theme) keyGroupStyles {
	return keyGroupStyles{
		title: lipgloss.NewStyle().Bold(true).Foreground(t.Colors.Orange),
		group: lipgloss.NewStyle().Bold(true).Foreground(t.Colors.LightText),
		key:   lipgloss.NewStyle().Bold(true).Foreground(t.Colors.Violet),
		desc:  lipgloss.NewStyle().Foreground(t.Colors.LightText),
	}
}

// renderGroupLines renders one group (title + aligned binding rows) into flat
// lines with no embedded newlines, so line-based scroll windowing stays
// accurate. Key alignment is computed within the group.
func renderGroupLines(g KeyGroup, sty keyGroupStyles) []string {
	out := []string{sty.group.Render(g.Title)}
	maxKey := 0
	for _, r := range g.Rows {
		if len(r.Keys) > maxKey {
			maxKey = len(r.Keys)
		}
	}
	for _, r := range g.Rows {
		pad := strings.Repeat(" ", maxKey-len(r.Keys)+2)
		out = append(out, "  "+sty.key.Render(r.Keys)+pad+sty.desc.Render(r.Desc))
	}
	return out
}

// stackGroups joins a set of groups vertically with a blank line between.
func stackGroups(gs []KeyGroup, sty keyGroupStyles) []string {
	var out []string
	for i, g := range gs {
		if i > 0 {
			out = append(out, "")
		}
		out = append(out, renderGroupLines(g, sty)...)
	}
	return out
}

// packKeyGroups lays groups out into flat content lines: the first leftCount
// groups form the left column and the rest the right column, but only when
// availW is wide enough AND the packed block actually fits (chrome allowance
// of 6). Otherwise it falls back to a single stacked column. Returns the lines
// and whether two columns were used.
func packKeyGroups(groups []KeyGroup, sty keyGroupStyles, availW, leftCount int) ([]string, bool) {
	if availW >= keyGroupTwoColMinWidth && leftCount > 0 && len(groups) > leftCount {
		leftBlock := strings.Join(stackGroups(groups[:leftCount], sty), "\n")
		rightBlock := strings.Join(stackGroups(groups[leftCount:], sty), "\n")
		// Pad the left column so its key rows stay aligned and the columns are
		// visually separated.
		leftW := lipgloss.Width(leftBlock) + keyGroupColGap
		leftBlock = lipgloss.NewStyle().Width(leftW).Render(leftBlock)
		body := lipgloss.JoinHorizontal(lipgloss.Top, leftBlock, rightBlock)
		if lipgloss.Width(body)+6 <= availW {
			return strings.Split(body, "\n"), true
		}
	}
	return stackGroups(groups, sty), false
}

// KeyGroupLines renders groups into flat content lines (no header, no box),
// packing the first leftCount groups into a left column when availW allows.
// A help overlay calls this to keep its own scroll windowing and boxing while
// sharing the exact layout the popup uses. Returns the lines and whether two
// columns were used.
func KeyGroupLines(groups []KeyGroup, t theme.Theme, availW, leftCount int) ([]string, bool) {
	return packKeyGroups(groups, newKeyGroupStyles(t), availW, leftCount)
}

// OverlayCenter composites a popup box onto base so the popup band sits
// vertically centered, each covered row replaced by a full-width horizontally
// centered line. It replaces whole base rows rather than splicing mid-line, so
// no ANSI escape sequence is ever cut — the safe centering primitive every
// Grove which-key TUI shares (the treemux-style side-preserving splice via
// charmbracelet/x/ansi is an optional upgrade). width is the content width the
// popup is centered within; if the popup is at least as tall as base it simply
// replaces base.
func OverlayCenter(base, popup string, width int) string {
	baseLines := strings.Split(base, "\n")
	popupLines := strings.Split(popup, "\n")
	if len(popupLines) >= len(baseLines) {
		return popup
	}
	start := (len(baseLines) - len(popupLines)) / 2
	for i, pl := range popupLines {
		baseLines[start+i] = lipgloss.PlaceHorizontal(width, lipgloss.Center, pl)
	}
	return strings.Join(baseLines, "\n")
}

// RenderKeyGroups renders a complete which-key popup: an Orange title header,
// the groups packed into up to two columns, inside a rounded border. maxW/maxH
// bound the block (maxH truncates overflow with a marker; chords are small so
// this rarely fires). The result is NOT screen-centered — the caller anchors it
// via a compositor blit / lipgloss.Place overlay.
func RenderKeyGroups(title string, groups []KeyGroup, t theme.Theme, maxW, maxH int) string {
	sty := newKeyGroupStyles(t)

	// Split roughly in half so two-column popups stay balanced.
	leftCount := 0
	if len(groups) >= 2 {
		leftCount = (len(groups) + 1) / 2
	}
	body, _ := packKeyGroups(groups, sty, maxW, leftCount)

	content := []string{sty.title.Render(title), ""}
	content = append(content, body...)

	// Clamp height: reserve the box chrome (border + padding = 4 lines) and, if
	// the body still overflows, drop the tail with a muted marker so the popup
	// never grows past the terminal.
	moreStyle := lipgloss.NewStyle().Foreground(t.Colors.MutedText).Italic(true)
	if maxH > 0 {
		limit := maxH - 4
		if limit < 1 {
			limit = 1
		}
		if len(content) > limit {
			if limit >= 1 {
				content = content[:limit-1]
				content = append(content, moreStyle.Render("… (esc to close)"))
			}
		}
	}

	maxWidth := 0
	for _, l := range content {
		if w := lipgloss.Width(l); w > maxWidth {
			maxWidth = w
		}
	}

	box := lipgloss.NewStyle().
		Padding(0, 2).
		Width(maxWidth + 4). // +4 for left/right padding (2 each)
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Colors.Orange).
		Render(strings.Join(content, "\n"))
	return box
}
