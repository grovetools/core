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
