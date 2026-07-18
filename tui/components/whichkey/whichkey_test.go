package whichkey

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/grovetools/core/tui/theme"
)

// testTheme returns a concrete theme value for rendering assertions.
func testTheme() theme.Theme {
	return *theme.DefaultTheme
}

// sampleGroups builds two small groups used across the layout tests.
func sampleGroups() []KeyGroup {
	return []KeyGroup{
		{Title: "View (v…)", Rows: []KeyRow{
			{Keys: "l", Desc: "logs"},
			{Keys: "f", Desc: "frontmatter"},
		}},
		{Title: "Change (c…)", Rows: []KeyRow{
			{Keys: "s", Desc: "status"},
			{Keys: "t", Desc: "type"},
		}},
	}
}

func TestRenderKeyGroupsContainsRows(t *testing.T) {
	out := RenderKeyGroups("Pending", sampleGroups(), testTheme(), 120, 40)

	for _, want := range []string{"Pending", "logs", "frontmatter", "status", "type"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered popup missing %q:\n%s", want, out)
		}
	}
}

func TestTwoColumnPacking(t *testing.T) {
	groups := sampleGroups()

	// Wide enough + a left/right split → two columns.
	if _, twoCol := KeyGroupLines(groups, testTheme(), 100, 1); !twoCol {
		t.Errorf("availW=100 with 2 groups: expected two-column packing")
	}

	// Below the two-column floor → single column.
	if _, twoCol := KeyGroupLines(groups, testTheme(), 40, 1); twoCol {
		t.Errorf("availW=40: expected single-column fallback")
	}
}

// TestOverlayCenter: the popup band is vertically centered over the base, the
// base rows above and below the band are preserved verbatim, and each popup row
// is horizontally centered within the given width.
func TestOverlayCenter(t *testing.T) {
	base := strings.Join([]string{
		"base-line-0",
		"base-line-1",
		"base-line-2",
		"base-line-3",
		"base-line-4",
		"base-line-5",
		"base-line-6",
	}, "\n")
	popup := strings.Join([]string{"POP-A", "POP-B"}, "\n")

	const width = 40
	out := OverlayCenter(base, popup, width)
	lines := strings.Split(out, "\n")

	if len(lines) != 7 {
		t.Fatalf("OverlayCenter changed line count: got %d, want 7:\n%s", len(lines), out)
	}

	// 7 base lines, 2 popup lines → start = (7-2)/2 = 2, band covers rows 2,3.
	start := (7 - 2) / 2
	// Rows above and below the band are untouched.
	baseLines := strings.Split(base, "\n")
	for i, l := range lines {
		if i >= start && i < start+2 {
			continue
		}
		if l != baseLines[i] {
			t.Errorf("row %d = %q, want preserved base %q", i, l, baseLines[i])
		}
	}

	// Band rows contain the popup content, centered to the full width.
	for j, want := range []string{"POP-A", "POP-B"} {
		row := lines[start+j]
		if !strings.Contains(row, want) {
			t.Errorf("band row %d missing %q: %q", start+j, want, row)
		}
		if w := lipgloss.Width(row); w != width {
			t.Errorf("band row %d width = %d, want %d (%q)", start+j, w, width, row)
		}
		// Centered → some left padding before the content.
		if !strings.HasPrefix(row, " ") {
			t.Errorf("band row %d not horizontally centered (no left pad): %q", start+j, row)
		}
	}
}

func TestMaxHeightTruncation(t *testing.T) {
	// Many rows in a single group, squeezed into a short popup.
	var rows []KeyRow
	for _, r := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		rows = append(rows, KeyRow{Keys: r, Desc: "action " + r})
	}
	groups := []KeyGroup{{Title: "Big", Rows: rows}}

	out := RenderKeyGroups("Pending", groups, testTheme(), 120, 8)

	if !strings.Contains(out, "esc to close") {
		t.Errorf("expected truncation marker in a height-clamped popup:\n%s", out)
	}
	if got := strings.Count(out, "\n") + 1; got > 8 {
		t.Errorf("expected popup height bounded by maxH=8, got %d lines:\n%s", got, out)
	}
}

// TestOverlayBottom pins the two properties that keep a which-key popup on-screen
// and low: the overlay does NOT grow base's line count (so it can never push a
// footer past the terminal), the popup lands on the BOTTOM rows, and the rule is
// drawn on the row directly above the popup band. Ported from flow-status's
// overlayWhichKeyBottom test when the primitive was promoted into core.
func TestOverlayBottom(t *testing.T) {
	base := strings.Join([]string{
		"row0", "row1", "row2", "row3", "row4",
		"row5", "row6", "row7", "row8", "row9",
	}, "\n")
	popup := strings.Join([]string{"POPUP-A", "POPUP-B", "POPUP-C"}, "\n")
	rule := strings.Repeat("-", 12)

	out := OverlayBottom(base, popup, rule)
	lines := strings.Split(out, "\n")

	if len(lines) != 10 {
		t.Fatalf("overlay changed height: got %d lines, want 10 (base height)", len(lines))
	}
	// The rule replaces the row directly above the popup (row 6).
	if !strings.Contains(lines[6], "----") {
		t.Errorf("expected rule on row 6, got %q", lines[6])
	}
	// Top rows untouched (row 6 becomes the rule, rows 7-9 the popup).
	for i := 0; i < 6; i++ {
		if !strings.Contains(lines[i], "row"+string(rune('0'+i))) {
			t.Errorf("top row %d was disturbed: %q", i, lines[i])
		}
	}
	// Popup occupies the bottom 3 rows.
	for i, want := range []string{"POPUP-A", "POPUP-B", "POPUP-C"} {
		if !strings.Contains(lines[7+i], want) {
			t.Errorf("bottom row %d = %q, want to contain %q", 7+i, lines[7+i], want)
		}
	}
}

// TestOverlayBottomTallPopupFallsBackToCenter: a popup at least as tall as base
// degenerates to the OverlayCenter fallback (no rule row).
func TestOverlayBottomTallPopupFallsBackToCenter(t *testing.T) {
	base := strings.Join([]string{"a", "b", "c"}, "\n")
	popup := strings.Join([]string{"P0", "P1", "P2", "P3"}, "\n")
	out := OverlayBottom(base, popup, strings.Repeat("-", 4))
	if out != popup {
		t.Errorf("tall popup should fall back to OverlayCenter (popup replaces base), got:\n%s", out)
	}
}
