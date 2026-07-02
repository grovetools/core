package theme

import (
	"fmt"
	"math"
	"strconv"
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
