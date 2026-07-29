package theme

import (
	"fmt"
	"math"
	"strconv"

	"github.com/charmbracelet/lipgloss"
)

// Color blending helpers ported from floraverse.nvim's lua/floraverse/util.lua.
// All inputs and outputs are "#rrggbb" hex strings. Invalid inputs are
// returned unchanged so callers never receive an empty color.

// parseHexRGB parses a "#rrggbb" hex color into its channel values.
func parseHexRGB(s string) (r, g, b float64, ok bool) {
	if len(s) != 7 || s[0] != '#' {
		return 0, 0, 0, false
	}
	v, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return float64(v >> 16 & 0xff), float64(v >> 8 & 0xff), float64(v & 0xff), true
}

// isHexColor reports whether s is a "#rrggbb" hex color literal.
func isHexColor(s string) bool {
	_, _, _, ok := parseHexRGB(s)
	return ok
}

// Blend mixes foreground into background. alpha is between 0 and 1:
// 0 results in background, 1 results in foreground. If either color is not a
// valid "#rrggbb" hex string, foreground is returned unchanged.
func Blend(foreground string, alpha float64, background string) string {
	fr, fg, fb, ok := parseHexRGB(foreground)
	if !ok {
		return foreground
	}
	br, bg, bb, ok := parseHexRGB(background)
	if !ok {
		return foreground
	}
	channel := func(f, b float64) int {
		v := alpha*f + (1-alpha)*b
		return int(math.Floor(math.Min(math.Max(0, v), 255) + 0.5))
	}
	return fmt.Sprintf("#%02x%02x%02x", channel(fr, br), channel(fg, bg), channel(fb, bb))
}

// Darken blends hex towards the background color. amount is the fraction of
// hex retained: Darken(c, 0.2, bg) is mostly background with 20% of c.
func Darken(hex string, amount float64, background string) string {
	return Blend(hex, amount, background)
}

// Lighten blends hex towards the foreground color. amount is the fraction of
// hex retained: Lighten(c, 0.8, fg) keeps 80% of c and mixes in 20% of fg.
func Lighten(hex string, amount float64, foreground string) string {
	return Blend(hex, amount, foreground)
}

// FadeToBackground dims a palette role hex toward the active theme's background.
// alpha is the fraction of the role retained: 1 returns it unchanged, 0 returns
// the background itself. It is how a TUI animates something OUT — an element that
// dissolves into the background rather than blinking off.
//
// It takes a HEX string rather than a lipgloss.TerminalColor on purpose. A
// TerminalColor's RGBA() resolves through the active renderer's color profile,
// which flattens to black under a non-TTY profile — so blending from one would
// silently fade from black in exactly the environments (tests, pipes) where the
// mistake is invisible. Callers pass a role from ActivePaletteColors.
//
// ok is false when the fade cannot be computed honestly — a non-hex role, or a
// theme whose palette is ANSI passthrough with no hex background to blend toward.
// A caller must then render the element with its ordinary color: unfaded is a
// legible degradation, whereas a guessed blend is not.
func FadeToBackground(roleHex string, alpha float64) (lipgloss.TerminalColor, bool) {
	if !isHexColor(roleHex) {
		return nil, false
	}
	if alpha >= 1 {
		return lipgloss.Color(roleHex), true
	}
	if alpha < 0 {
		alpha = 0
	}
	_, bg, _, ok := ActiveTerminalColors()
	if !ok || !isHexColor(bg) {
		return nil, false
	}
	return lipgloss.Color(Blend(roleHex, alpha, bg)), true
}

// ActivePaletteColors returns the active theme's palette roles as "#rrggbb"
// strings — the hex source blending needs, resolved for the terminal appearance
// the adaptive legacy colors follow.
//
// ok is false for ANSI passthrough palettes, whose roles are terminal indices
// rather than colors.
func ActivePaletteColors() (PaletteColors, bool) {
	p, found := lookupForAppearance(DefaultTheme.Name, lipgloss.HasDarkBackground())
	if !found {
		if p, found = lookupForAppearance(DefaultThemeName, lipgloss.HasDarkBackground()); !found {
			return PaletteColors{}, false
		}
	}
	if p.Meta.ANSI {
		return PaletteColors{}, false
	}
	return p.Colors, true
}
