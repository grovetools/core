package nvim

import (
	"testing"

	"github.com/grovetools/core/tui/theme"
)

func TestHexToRGBInt(t *testing.T) {
	cases := map[string]int{
		"#ffffff": 0xffffff,
		"#000000": 0x000000,
		"#1f1f28": 0x1f1f28,
		"1f1f28":  0x1f1f28,
		"":        -1,
		"#fff":    -1,
		"#zzzzzz": -1,
	}
	for in, want := range cases {
		if got := hexToRGBInt(in); got != want {
			t.Errorf("hexToRGBInt(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestGroveHighlightsCoversBaseGroups(t *testing.T) {
	p, ok := theme.Lookup(theme.DefaultThemeName)
	if !ok {
		t.Fatalf("default theme %q not in registry", theme.DefaultThemeName)
	}
	hl := groveHighlights(p)

	for _, group := range []string{
		"Normal", "NormalFloat", "CursorLine", "Visual", "LineNr",
		"Comment", "StatusLine", "VertSplit", "WinSeparator", "Pmenu",
		"PmenuSel", "DiagnosticError", "DiagnosticWarn", "DiagnosticInfo",
		"DiagnosticHint",
	} {
		if _, ok := hl[group]; !ok {
			t.Errorf("groveHighlights missing group %s", group)
		}
	}

	normal := hl["Normal"]
	if normal.Foreground == -1 || normal.Background == -1 {
		t.Errorf("Normal must carry both fg and bg, got %+v", normal)
	}
	if hl["Comment"].Foreground == -1 || !hl["Comment"].Italic {
		t.Errorf("Comment should be an italic fg-only group, got %+v", hl["Comment"])
	}
	if hl["Visual"].Background == -1 {
		t.Errorf("Visual must carry a bg, got %+v", hl["Visual"])
	}
}
