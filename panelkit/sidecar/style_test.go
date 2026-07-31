package sidecar

import (
	"strings"
	"testing"

	"github.com/grovetools/core/panelkit/panelproto"
)

func TestPaletteRendersHexTokens(t *testing.T) {
	p := NewPalette(&panelproto.Theme{Accent: "#7e9cd8"})
	got := p.Accent("hi")
	if got != "\x1b[38;2;126;156;216mhi\x1b[0m" {
		t.Errorf("Accent = %q, want a truecolor SGR", got)
	}
}

// The host sends whatever its palette resolved to, which is a hex string from
// some themes and an ANSI index from others. A converter that handles one
// silently drops half the theme.
func TestPaletteRendersAnsiIndices(t *testing.T) {
	tests := []struct {
		name  string
		color string
		want  string
	}{
		{"a base color uses the dedicated code", "4", "\x1b[34m"},
		{"a bright color uses the bright code", "12", "\x1b[94m"},
		{"a 256-color index uses the extended form", "200", "\x1b[38;5;200m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewPalette(&panelproto.Theme{Accent: tt.color}).Accent("x")
			if !strings.HasPrefix(got, tt.want) {
				t.Errorf("Accent = %q, want the prefix %q", got, tt.want)
			}
		})
	}
}

func TestPaletteAcceptsHexVariants(t *testing.T) {
	full := NewPalette(&panelproto.Theme{Accent: "#ff00aa"}).Accent("x")
	noHash := NewPalette(&panelproto.Theme{Accent: "ff00aa"}).Accent("x")
	short := NewPalette(&panelproto.Theme{Accent: "#f0a"}).Accent("x")

	if full != noHash {
		t.Errorf("the leading # changed the result: %q vs %q", full, noHash)
	}
	if full != short {
		t.Errorf("the three-digit shorthand did not expand: %q vs %q", short, full)
	}
}

// An unset token returns the text unchanged. A missing color is better than
// the wrong one next to host chrome the panel cannot see.
func TestPaletteLeavesUnsetTokensUnstyled(t *testing.T) {
	p := NewPalette(&panelproto.Theme{Accent: "#ffffff"})
	if got := p.Error("boom"); got != "boom" {
		t.Errorf("Error with no error token = %q, want the text unchanged", got)
	}
	if got := p.Fg("not-a-color", "x"); got != "x" {
		t.Errorf("Fg with garbage = %q, want the text unchanged", got)
	}
}

// A panel with no host must not guess at colors.
func TestZeroPaletteIsUnstyled(t *testing.T) {
	p := NewPalette(nil)
	for name, got := range map[string]string{
		"Text":     p.Text("x"),
		"Accent":   p.Accent("x"),
		"Muted":    p.Muted("x"),
		"Selected": p.Selected("x"),
		"OnAccent": p.OnAccent("x"),
	} {
		if got != "x" {
			t.Errorf("%s on a hostless palette = %q, want the text unchanged", name, got)
		}
	}
}

// The selection pair is applied together: one without the other is how a
// selected row ends up unreadable.
func TestPaletteSelectedAppliesBothHalves(t *testing.T) {
	p := NewPalette(&panelproto.Theme{SelectionFg: "#000000", SelectionBg: "#ffffff"})
	got := p.Selected("row")
	if !strings.Contains(got, "38;2;0;0;0") {
		t.Errorf("Selected = %q, missing the foreground", got)
	}
	if !strings.Contains(got, "48;2;255;255;255") {
		t.Errorf("Selected = %q, missing the background", got)
	}
}

func TestPaletteTextFallsBackToForeground(t *testing.T) {
	// Both names carry the same role and a host may send either.
	fromText := NewPalette(&panelproto.Theme{Text: "#abcdef"}).Text("x")
	fromFg := NewPalette(&panelproto.Theme{Foreground: "#abcdef"}).Text("x")
	if fromText != fromFg {
		t.Errorf("Text and Foreground disagreed: %q vs %q", fromText, fromFg)
	}
}

func TestPaletteAppearance(t *testing.T) {
	if !NewPalette(&panelproto.Theme{Appearance: "dark"}).Dark() {
		t.Error("a dark appearance did not report Dark")
	}
	if NewPalette(&panelproto.Theme{Appearance: "light"}).Dark() {
		t.Error("a light appearance reported Dark")
	}
	// A host that predates the field sends nothing; dark is the safer guess
	// for a terminal.
	if !NewPalette(&panelproto.Theme{}).Dark() {
		t.Error("an unset appearance should read as dark")
	}
}

func TestPaletteAttributes(t *testing.T) {
	p := NewPalette(nil)
	if got := p.Bold("x"); got != "\x1b[1mx\x1b[0m" {
		t.Errorf("Bold = %q", got)
	}
	if got := p.Faint("x"); got != "\x1b[2mx\x1b[0m" {
		t.Errorf("Faint = %q", got)
	}
}
