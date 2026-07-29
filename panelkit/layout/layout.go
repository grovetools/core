// Package layout holds the small composition helpers a panel needs to place
// text on a row, in the couple of shapes the ecosystem kept re-deriving.
//
// Nothing here holds state or renders a widget. These are string-to-string
// functions over lipgloss-measured widths, so they work equally on a footer, a
// header, a status row, or a line inside a sidecar's own frame.
package layout

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// RightPin lays left and right out on one row of the given width, with right
// flush against the right edge and at least one column between them.
//
// It exists because sharing a row is how a view reclaims vertical space: a
// footer is mostly empty and a scroll indicator like "(1-17 of 40)" is one
// short token, so putting them on the same row hands the list back two rows —
// the indicator's own, plus the blank that separated it from the content.
//
// The row must never wrap, because a wrapped row costs back the row that was
// just reclaimed. So the rules, in order:
//
//   - No right text, or no usable width: return left unchanged.
//   - Not enough room for right plus a separating column: drop right entirely.
//     The caller's content still conveys whatever right was reporting — a
//     cursor row shows position without an indicator.
//   - left too long for what is left: truncate it with an ellipsis.
//
// Widths are measured with lipgloss, so styled input is handled correctly:
// ANSI escapes do not count toward the width, and the returned row is padded,
// not space-run-concatenated.
func RightPin(left, right string, width int) string {
	if right == "" || width <= 0 {
		return left
	}
	// One column of separation between the two.
	leftWidth := width - lipgloss.Width(right) - 1
	if leftWidth < 1 {
		return left
	}
	if lipgloss.Width(left) > leftWidth {
		left = ansi.Truncate(left, leftWidth, "…")
	}
	// Width() pads the (now guaranteed short enough) left text out so right
	// lands flush against the edge without a manual space run.
	return lipgloss.NewStyle().Width(leftWidth).Render(left) + " " + right
}
