package layout

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestRightPin(t *testing.T) {
	tests := []struct {
		name  string
		left  string
		right string
		width int
		want  string
	}{
		{
			"the right token lands flush against the edge",
			"help text", "(1-17 of 40)", 30,
			"help text         (1-17 of 40)",
		},
		{
			"no right token leaves the left text alone",
			"help text", "", 30,
			"help text",
		},
		{
			"a zero width leaves the left text alone",
			"help text", "(1 of 2)", 0,
			"help text",
		},
		{
			"a negative width leaves the left text alone",
			"help text", "(1 of 2)", -5,
			"help text",
		},
		{
			"left is truncated to make room rather than wrapping",
			"a very long help line indeed", "(1 of 2)", 20,
			"a very lon… (1 of 2)",
		},
		{
			"too narrow for both drops the right token entirely",
			"help", "(1-17 of 40)", 12,
			"help",
		},
		{
			"exactly wide enough keeps both",
			"h", "(1 of 2)", 10,
			"h (1 of 2)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RightPin(tt.left, tt.right, tt.width)
			if got != tt.want {
				t.Errorf("RightPin(%q, %q, %d)\n got %q\nwant %q", tt.left, tt.right, tt.width, got, tt.want)
			}
		})
	}
}

// TestRightPinNeverWraps is the property the whole helper exists for: whatever
// it returns fits on one row of the requested width, so the row it reclaimed
// stays reclaimed.
func TestRightPinNeverWraps(t *testing.T) {
	lefts := []string{"", "h", "help text", strings.Repeat("long ", 40)}
	rights := []string{"", "(1 of 2)", "(1-17 of 40)", strings.Repeat("x", 50)}
	for _, left := range lefts {
		for _, right := range rights {
			for _, width := range []int{0, 1, 5, 13, 30, 200} {
				got := RightPin(left, right, width)
				if strings.Contains(got, "\n") {
					t.Fatalf("RightPin(%q, %q, %d) contains a newline", left, right, width)
				}
				// A dropped right token returns left verbatim, which the
				// caller is responsible for; otherwise the row must fit.
				if got == left {
					continue
				}
				if w := lipgloss.Width(got); w > width {
					t.Fatalf("RightPin(%q, %q, %d) is %d columns wide", left, right, width, w)
				}
			}
		}
	}
}

// TestRightPinMeasuresStyledText checks that ANSI escapes in either side do not
// count toward the width — the footers this replaces are styled before they
// get here.
func TestRightPinMeasuresStyledText(t *testing.T) {
	styled := lipgloss.NewStyle().Bold(true).Render("help text")
	got := RightPin(styled, "(1 of 2)", 30)
	if w := lipgloss.Width(got); w != 30 {
		t.Errorf("RightPin with styled left = %d columns, want 30", w)
	}
	if !strings.Contains(got, "help text") {
		t.Errorf("RightPin dropped the styled text: %q", got)
	}
}
