package theme

import (
	"strings"
	"testing"
)

const validThemeTOML = `
[meta]
name = "testtheme-dark"
family = "testtheme"
variant = "dark"
appearance = "dark"
upstream = "https://example.com/testtheme"
author = "Test Author"
license = "MIT"

[palette]
bg = "#1F1F28"
bg_dark = "#181820"
fg = "#DCD7BA"
comment = "#727169"
border = "#363646"
red = "#FF5D62"
green = "#98BB6C"
yellow = "#FF9E3B"
blue = "#7FB4CA"
magenta = "#D27E99"
cyan = "#7E9CD8"
orange = "#FFA066"
purple = "#957FB8"
`

func TestParsePaletteValid(t *testing.T) {
	p, err := parsePalette([]byte(validThemeTOML))
	if err != nil {
		t.Fatalf("parsePalette() error: %v", err)
	}
	if p.Meta.Name != "testtheme-dark" || p.Meta.Family != "testtheme" || p.Meta.Variant != "dark" {
		t.Errorf("unexpected meta: %+v", p.Meta)
	}
	if p.Meta.Upstream != "https://example.com/testtheme" || p.Meta.Author != "Test Author" || p.Meta.License != "MIT" {
		t.Errorf("unexpected provenance metadata: %+v", p.Meta)
	}
	if p.Colors.Red != "#FF5D62" {
		t.Errorf("red = %q, want %q", p.Colors.Red, "#FF5D62")
	}
}

func TestParsePaletteDerivations(t *testing.T) {
	p, err := parsePalette([]byte(validThemeTOML))
	if err != nil {
		t.Fatalf("parsePalette() error: %v", err)
	}
	c := p.Colors

	// Copy-based derivations.
	if c.FgInverse != c.BgDark {
		t.Errorf("fg_inverse = %q, want bg_dark %q", c.FgInverse, c.BgDark)
	}
	if c.Git.Add != c.Green || c.Git.Change != c.Blue || c.Git.Delete != c.Red {
		t.Errorf("git colors not derived from accents: %+v", c.Git)
	}
	if c.Diagnostics.Error != c.Red || c.Diagnostics.Warning != c.Yellow ||
		c.Diagnostics.Info != c.Blue || c.Diagnostics.Hint != c.Cyan {
		t.Errorf("diagnostics not derived from accents: %+v", c.Diagnostics)
	}

	// Blend-based derivations.
	if want := Darken(c.Blue, 0.25, c.Bg); c.BgVisual != want {
		t.Errorf("bg_visual = %q, want %q", c.BgVisual, want)
	}
	if want := Lighten(c.Bg, 0.93, c.Fg); c.BgHighlight != want {
		t.Errorf("bg_highlight = %q, want %q", c.BgHighlight, want)
	}
	if want := Darken(c.Fg, 0.85, c.Bg); c.FgDark != want {
		t.Errorf("fg_dark = %q, want %q", c.FgDark, want)
	}
	if want := Darken(c.Fg, 0.30, c.Bg); c.FgGutter != want {
		t.Errorf("fg_gutter = %q, want %q", c.FgGutter, want)
	}

	// Terminal slot derivations.
	tc := p.Terminal
	if tc.Black != c.BgDark || tc.Red != c.Red || tc.White != c.Fg {
		t.Errorf("terminal normal slots not derived: %+v", tc)
	}
	if tc.BlackBright != c.Comment {
		t.Errorf("terminal.black_bright = %q, want comment %q", tc.BlackBright, c.Comment)
	}
	if want := Lighten(c.Red, 0.8, c.Fg); tc.RedBright != want {
		t.Errorf("terminal.red_bright = %q, want %q", tc.RedBright, want)
	}
	if tc.WhiteBright != tc.White {
		t.Errorf("terminal.white_bright = %q, want %q", tc.WhiteBright, tc.White)
	}
	for _, v := range p.colorValues() {
		if v.value == "" {
			t.Errorf("role %s left empty after derivation", v.role)
		}
	}
}

func TestParsePaletteExplicitValuesWin(t *testing.T) {
	doc := validThemeTOML + `
fg_inverse = "#111111"

[palette.git]
add = "#222222"

[palette.diagnostics]
hint = "#333333"

[terminal]
red_bright = "#444444"
`
	p, err := parsePalette([]byte(doc))
	if err != nil {
		t.Fatalf("parsePalette() error: %v", err)
	}
	if p.Colors.FgInverse != "#111111" {
		t.Errorf("fg_inverse = %q, want explicit #111111", p.Colors.FgInverse)
	}
	if p.Colors.Git.Add != "#222222" {
		t.Errorf("git.add = %q, want explicit #222222", p.Colors.Git.Add)
	}
	if p.Colors.Diagnostics.Hint != "#333333" {
		t.Errorf("diagnostics.hint = %q, want explicit #333333", p.Colors.Diagnostics.Hint)
	}
	if p.Terminal.RedBright != "#444444" {
		t.Errorf("terminal.red_bright = %q, want explicit #444444", p.Terminal.RedBright)
	}
}

func TestParsePaletteMissingRequiredRole(t *testing.T) {
	doc := strings.Replace(validThemeTOML, "purple = \"#957FB8\"\n", "", 1)
	_, err := parsePalette([]byte(doc))
	if err == nil {
		t.Fatal("expected error for missing required role, got nil")
	}
	if !strings.Contains(err.Error(), "palette.purple is required") {
		t.Errorf("error %q does not mention missing purple", err)
	}
}

func TestParsePaletteMissingMeta(t *testing.T) {
	doc := strings.Replace(validThemeTOML, "appearance = \"dark\"\n", "appearance = \"sepia\"\n", 1)
	doc = strings.Replace(doc, "family = \"testtheme\"\n", "", 1)
	_, err := parsePalette([]byte(doc))
	if err == nil {
		t.Fatal("expected error for invalid metadata, got nil")
	}
	if !strings.Contains(err.Error(), "meta.family is required") {
		t.Errorf("error %q does not mention missing family", err)
	}
	if !strings.Contains(err.Error(), "meta.appearance") {
		t.Errorf("error %q does not mention invalid appearance", err)
	}
}

func TestParsePaletteInvalidHex(t *testing.T) {
	doc := strings.Replace(validThemeTOML, "red = \"#FF5D62\"", "red = \"#FF5D6\"", 1)
	_, err := parsePalette([]byte(doc))
	if err == nil {
		t.Fatal("expected error for invalid hex, got nil")
	}
	if !strings.Contains(err.Error(), "red") {
		t.Errorf("error %q does not name the offending role", err)
	}
}

func TestParsePaletteInvalidTOML(t *testing.T) {
	if _, err := parsePalette([]byte("this is not toml = [")); err == nil {
		t.Fatal("expected TOML parse error, got nil")
	}
}

const validANSITOML = `
[meta]
name = "ansi-test"
family = "ansi-test"
variant = "default"
appearance = "dark"
ansi = true

[palette]
bg = "0"
bg_dark = "0"
fg = "7"
comment = "8"
border = "8"
red = "1"
green = "2"
yellow = "3"
blue = "4"
magenta = "13"
cyan = "6"
orange = "11"
purple = "5"
`

func TestParsePaletteANSI(t *testing.T) {
	p, err := parsePalette([]byte(validANSITOML))
	if err != nil {
		t.Fatalf("parsePalette() error: %v", err)
	}
	c := p.Colors
	// ANSI palettes derive by copying, never by blending.
	if c.BgVisual != c.Comment {
		t.Errorf("ansi bg_visual = %q, want comment %q", c.BgVisual, c.Comment)
	}
	if c.FgDark != c.Fg || c.FgGutter != c.Comment || c.FgInverse != c.BgDark {
		t.Errorf("ansi fg derivations wrong: %+v", c)
	}
	if p.Terminal.RedBright != p.Terminal.Red {
		t.Errorf("ansi terminal.red_bright = %q, want %q", p.Terminal.RedBright, p.Terminal.Red)
	}
}

func TestParsePaletteANSIInvalidIndex(t *testing.T) {
	doc := strings.Replace(validANSITOML, "red = \"1\"", "red = \"256\"", 1)
	if _, err := parsePalette([]byte(doc)); err == nil {
		t.Fatal("expected error for ANSI index out of range, got nil")
	}
	doc = strings.Replace(validANSITOML, "red = \"1\"", "red = \"#FF0000\"", 1)
	if _, err := parsePalette([]byte(doc)); err == nil {
		t.Fatal("expected error for hex value in ANSI palette, got nil")
	}
}

func TestBlend(t *testing.T) {
	cases := []struct {
		fg    string
		alpha float64
		bg    string
		want  string
	}{
		{"#ffffff", 1, "#000000", "#ffffff"},
		{"#ffffff", 0, "#000000", "#000000"},
		{"#ffffff", 0.5, "#000000", "#808080"},
		{"#ff0000", 0.5, "#0000ff", "#800080"},
		{"#FF5D62", 0.2, "#1F1F28", "#4c2b34"}, // mixed case input
	}
	for _, c := range cases {
		if got := Blend(c.fg, c.alpha, c.bg); got != c.want {
			t.Errorf("Blend(%q, %v, %q) = %q, want %q", c.fg, c.alpha, c.bg, got, c.want)
		}
	}
	// Invalid inputs pass through unchanged.
	if got := Blend("nope", 0.5, "#000000"); got != "nope" {
		t.Errorf("Blend with invalid fg = %q, want passthrough", got)
	}
	if got := Blend("#ffffff", 0.5, "8"); got != "#ffffff" {
		t.Errorf("Blend with invalid bg = %q, want passthrough", got)
	}
}

func TestDarkenLighten(t *testing.T) {
	if got, want := Darken("#ffffff", 0.5, "#000000"), "#808080"; got != want {
		t.Errorf("Darken = %q, want %q", got, want)
	}
	if got, want := Lighten("#000000", 0.5, "#ffffff"), "#808080"; got != want {
		t.Errorf("Lighten = %q, want %q", got, want)
	}
	// amount is the fraction of the original color retained.
	if got, want := Darken("#808080", 1, "#000000"), "#808080"; got != want {
		t.Errorf("Darken with amount=1 = %q, want %q", got, want)
	}
}
