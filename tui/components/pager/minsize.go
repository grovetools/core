package pager

import (
	"github.com/charmbracelet/lipgloss"

	core_theme "github.com/grovetools/core/tui/theme"
)

// tooSmall reports whether a page has declared a minimum size that the given
// body does not meet. A page without PageWithMinSize is never too small — the
// contract is opt-in, and a page that has not opted in is assumed to cope.
//
// A body of zero or negative width or height means the pager has not been
// sized yet, which is not the same as being too small: reporting it as too
// small would flash the placeholder on every first paint before the first
// WindowSizeMsg lands.
func tooSmall(p Page, width, height int) bool {
	ms, ok := p.(PageWithMinSize)
	if !ok || width <= 0 || height <= 0 {
		return false
	}
	minW, minH := ms.MinSize()
	return (minW > 0 && width < minW) || (minH > 0 && height < minH)
}

// TooSmallPlaceholder renders the message a pager shows in place of a page
// whose declared minimum size the pane does not meet.
//
// Exported because a page that composes its own sub-panes has the same problem
// one level down — a split whose halves each have a minimum — and the answer
// should look the same wherever it appears. A user resizing a pane should not
// have to work out whether the message came from the host or the panel.
//
// The message degrades with the space available: the full sentence, then a
// short form, then a single glyph. There is always something, because a blank
// pane reads as a crash.
func TooSmallPlaceholder(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	th := core_theme.DefaultTheme

	var msg string
	switch {
	case width >= 34:
		msg = "Pane too small — widen or hide a split"
	case width >= 15:
		msg = "Pane too small"
	default:
		msg = core_theme.IconWarning
	}

	return lipgloss.Place(width, height,
		lipgloss.Center, lipgloss.Center,
		th.Muted.Render(msg))
}
