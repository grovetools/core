package theme

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

// Contrast validator: asserts WCAG-style minimum contrast ratios for the
// load-bearing role pairs of every embedded (non-ANSI) palette.
//
// Standard tier (all palettes):
//   - fg on bg:      >= 4.5:1 (7:1 is the target; 4.5 is the enforced floor
//     because several classic palettes are deliberately soft)
//   - comment on bg: >= 3:1
//   - each of the 8 accents on bg: >= 3:1
//
// High-contrast tier (palette name contains "high-contrast"):
//   - fg on bg:      >= 7:1
//   - comment on bg: >= 4.5:1
//   - accents on bg: >= 4.5:1
//
// Upstream fidelity wins over the floors: when a genuine upstream palette
// value fails a floor, the palette keeps the upstream color and the pair is
// recorded in contrastExceptions below with a relaxed floor just under the
// measured ratio, so any further regression still fails the test. Every
// exception documents the measured ratio at the time it was recorded.

// relativeLuminance computes WCAG 2.x relative luminance of a "#rrggbb" color.
func relativeLuminance(hex string) (float64, error) {
	r, g, b, ok := parseHexRGB(hex)
	if !ok {
		return 0, fmt.Errorf("not a #rrggbb color: %q", hex)
	}
	lin := func(c float64) float64 {
		c /= 255
		if c <= 0.04045 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b), nil
}

// contrastRatio computes the WCAG contrast ratio (1:1 .. 21:1) between two
// "#rrggbb" colors.
func contrastRatio(a, b string) (float64, error) {
	la, err := relativeLuminance(a)
	if err != nil {
		return 0, err
	}
	lb, err := relativeLuminance(b)
	if err != nil {
		return 0, err
	}
	if lb > la {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05), nil
}

// contrastExceptions relaxes the floor for specific theme/role pairs where
// the upstream palette genuinely fails the tier floor. Keyed by theme name,
// then by role. Each value is the relaxed floor; the measured ratio at
// recording time is noted alongside. Do not add entries for colors grove
// chooses itself — only for verbatim upstream values.
var contrastExceptions = map[string]map[string]float64{
	// Ayu light's accent row is designed for syntax on white and sits well
	// under 3:1 across the board (values verbatim from themes/light.yaml,
	// measured 2026-07-02).
	"ayu-light": {
		"red":     2.75, // #f07171 on #fcfcfc = 2.80
		"green":   2.40, // #86b300 on #fcfcfc = 2.42
		"yellow":  2.05, // #eba400 on #fcfcfc = 2.08
		"blue":    2.70, // #22a4e6 on #fcfcfc = 2.72
		"magenta": 1.95, // #f2a191 (ayu "pink") on #fcfcfc = 1.99
		"cyan":    2.20, // #4cbf99 (ayu "teal") on #fcfcfc = 2.22
		"orange":  2.40, // #fa8532 on #fcfcfc = 2.42
	},
	// Catppuccin's overlay0 comments are deliberately soft; latte's warm
	// accents are pastel-on-light (palette.json verbatim).
	"catppuccin-frappe": {
		"comment": 2.85, // overlay0 #737994 on base #303446 = 2.87
	},
	"catppuccin-latte": {
		"comment": 2.25, // overlay0 #9ca0b0 on base #eff1f5 = 2.30
		"green":   2.90, // #40a02b = 2.96
		"yellow":  2.25, // #df8e1d = 2.31
		"magenta": 2.30, // pink #ea76cb = 2.34
		"orange":  2.60, // peach #fe640b = 2.64
	},
	// Everforest's light accents are pastel-forest tones on warm cream; the
	// grey1 comments are soft in every light hardness (autoload palette
	// verbatim, measured 2026-07-02). The softer the background, the more
	// pairs dip under 3:1.
	"everforest-light-hard": {
		"comment": 2.65, // #939f91 on #fffbef = 2.67
		"green":   2.75, // #8da101 on #fffbef = 2.81
		"yellow":  2.15, // #dfa000 on #fffbef = 2.21
		"magenta": 2.90, // #df69ba on #fffbef = 2.96
		"cyan":    2.85, // #35a77c on #fffbef = 2.91
		"orange":  2.55, // #f57d26 on #fffbef = 2.59
		"purple":  2.90, // #df69ba on #fffbef = 2.96
	},
	"everforest-light": {
		"comment": 2.50, // #939f91 on #fdf6e3 = 2.56
		"green":   2.65, // #8da101 on #fdf6e3 = 2.69
		"yellow":  2.10, // #dfa000 on #fdf6e3 = 2.12
		"magenta": 2.80, // #df69ba on #fdf6e3 = 2.83
		"cyan":    2.75, // #35a77c on #fdf6e3 = 2.79
		"orange":  2.45, // #f57d26 on #fdf6e3 = 2.48
		"purple":  2.80, // #df69ba on #fdf6e3 = 2.83
	},
	"everforest-light-soft": {
		"comment": 2.25, // #939f91 on #f3ead3 = 2.30
		"red":     2.70, // #f85552 on #f3ead3 = 2.73
		"green":   2.40, // #8da101 on #f3ead3 = 2.42
		"yellow":  1.90, // #dfa000 on #f3ead3 = 1.91
		"blue":    2.80, // #3a94c5 on #f3ead3 = 2.82
		"magenta": 2.50, // #df69ba on #f3ead3 = 2.55
		"cyan":    2.50, // #35a77c on #f3ead3 = 2.51
		"orange":  2.20, // #f57d26 on #f3ead3 = 2.23
		"purple":  2.50, // #df69ba on #f3ead3 = 2.55
	},
	// Floraverse intentionally uses its dim purple range for comments.
	"floraverse-main":     {"comment": 1.90}, // #4c3866 on #0a0810 = 1.96
	"floraverse-midnight": {"comment": 1.90}, // #4c3866 on #0a0810 = 1.96
	"floraverse-twilight": {"comment": 2.70}, // #5a5278 on #0a0810 = 2.76
	"floraverse-day":      {"comment": 1.95}, // #baabd1 on #f8f8f9 = 2.01 (Util.invert output)
	"floraverse-dawn":     {"comment": 2.65}, // #9a95b1 on #f8f8f9 = 2.71 (Util.invert output)
	// Gruvbox's gray comments sit just under 3:1 on the soft light bg0
	// (colors/gruvbox.vim verbatim; the medium/hard backgrounds pass).
	"gruvbox-light-soft": {"comment": 2.90}, // #928374 on #f2e5bc = 2.92
	// Kanagawa lotus uses lotusGray3 for comments on the warm lotus paper.
	"kanagawa-lotus": {"comment": 2.90}, // #8a8980 on #f2ecbc = 2.93
	// Nord comments: even the brightened nord3 (#616e88, used by official
	// nord editor ports) stays low against nord0; raw nord3 is ~1.7:1.
	"nord-dark": {"comment": 2.40}, // #616e88 on #2e3440 = 2.43
	// Navarasu onedark uses its "grey" slot for comments in every variant.
	"onedark-dark":   {"comment": 2.30}, // #5c6370 on #282c34 = 2.32
	"onedark-darker": {"comment": 2.20}, // #535965 on #1f2329 = 2.24
	"onedark-cool":   {"comment": 2.25}, // #546178 on #242b38 = 2.27
	"onedark-deep":   {"comment": 2.15}, // #455574 on #1a212e = 2.16
	"onedark-warm":   {"comment": 2.35}, // #646568 on #2c2d30 = 2.36
	"onedark-warmer": {"comment": 2.30}, // #5a5b5e on #232326 = 2.31
	"onedark-light":  {"comment": 2.45}, // #a0a1a7 on #fafafa = 2.47
	// Oxocarbon dark comments use base03; light reuses several saturated
	// dark-theme accents on pure white (upstream init.lua verbatim).
	"oxocarbon-dark": {"comment": 2.30}, // #525252 on #161616 = 2.32
	"oxocarbon-light": {
		"green":   2.35, // #42be65 on #ffffff = 2.39
		"magenta": 2.35, // #ff7eb6 on #ffffff = 2.36
		"cyan":    2.30, // #08bdba on #ffffff = 2.33
		"orange":  2.75, // #ff6f00 on #ffffff = 2.79
	},
	// Rosé Pine dawn's gold and rose are pastel-warm accents on near-white
	// (rose-pine/neovim palette.lua verbatim).
	"rose-pine-dawn": {
		"yellow": 2.00, // gold #ea9d34 on #faf4ed = 2.05
		"orange": 2.55, // rose #d7827e on #faf4ed = 2.60
	},
	// Solarized's base01/base1 comment tones are the canonical values; in
	// light mode base00 body text and several accents also sit under the
	// tier floors (altercation/solarized 16-color table verbatim).
	"solarized-dark": {"comment": 2.75}, // base01 #586e75 on #002b36 = 2.79
	"solarized-light": {
		"fg":      4.10, // base00 #657b83 on #fdf6e3 = 4.13
		"comment": 2.45, // base1 #93a1a1 on #fdf6e3 = 2.48
		"green":   2.95, // #859900 on #fdf6e3 = 2.97
		"yellow":  2.95, // #b58900 on #fdf6e3 = 2.98
		"cyan":    2.90, // #2aa198 on #fdf6e3 = 2.93
	},
	// Tokyonight comments are soft by design in every style.
	"tokyonight-night": {"comment": 2.75}, // #565f89 on #1a1b26 = 2.76
	"tokyonight-storm": {"comment": 2.30}, // #565f89 on #24283b = 2.35
	"tokyonight-day":   {"comment": 2.50}, // #848cb5 on #e1e2e7 = 2.54
}

func contrastFloor(themeName, role string, tierFloor float64) float64 {
	if roles, ok := contrastExceptions[themeName]; ok {
		if floor, ok := roles[role]; ok {
			return floor
		}
	}
	return tierFloor
}

func TestPaletteContrast(t *testing.T) {
	if len(registry.palettes) == 0 {
		t.Fatal("registry is empty")
	}
	for name, p := range registry.palettes {
		if p.Meta.ANSI {
			continue // ANSI palettes have no resolvable hex values.
		}
		t.Run(name, func(t *testing.T) {
			highContrast := strings.Contains(name, "high-contrast")
			fgFloor, commentFloor, accentFloor := 4.5, 3.0, 3.0
			if highContrast {
				fgFloor, commentFloor, accentFloor = 7.0, 4.5, 4.5
			}

			c := p.Colors
			checks := []struct {
				role  string
				color string
				floor float64
			}{
				{"fg", c.Fg, fgFloor},
				{"comment", c.Comment, commentFloor},
				{"red", c.Red, accentFloor},
				{"green", c.Green, accentFloor},
				{"yellow", c.Yellow, accentFloor},
				{"blue", c.Blue, accentFloor},
				{"magenta", c.Magenta, accentFloor},
				{"cyan", c.Cyan, accentFloor},
				{"orange", c.Orange, accentFloor},
				{"purple", c.Purple, accentFloor},
			}
			for _, check := range checks {
				ratio, err := contrastRatio(check.color, c.Bg)
				if err != nil {
					t.Errorf("%s: %v", check.role, err)
					continue
				}
				floor := contrastFloor(name, check.role, check.floor)
				if ratio < floor {
					t.Errorf("%s %s on bg %s = %.2f:1, want >= %.2f:1",
						check.role, check.color, c.Bg, ratio, floor)
				}
			}
		})
	}
}

// TestContrastExceptionsAreLive fails when an exception entry stops being
// needed (theme removed, or the color now passes the regular tier floor) so
// the table cannot rot.
func TestContrastExceptionsAreLive(t *testing.T) {
	for name, roles := range contrastExceptions {
		p, ok := registry.palettes[name]
		if !ok {
			t.Errorf("contrastExceptions references unknown theme %q", name)
			continue
		}
		highContrast := strings.Contains(name, "high-contrast")
		for role, floor := range roles {
			var color string
			var tierFloor float64
			switch role {
			case "fg":
				color = p.Colors.Fg
				tierFloor = 4.5
				if highContrast {
					tierFloor = 7.0
				}
			case "comment":
				color = p.Colors.Comment
				tierFloor = 3.0
				if highContrast {
					tierFloor = 4.5
				}
			default:
				accents := map[string]string{
					"red": p.Colors.Red, "green": p.Colors.Green,
					"yellow": p.Colors.Yellow, "blue": p.Colors.Blue,
					"magenta": p.Colors.Magenta, "cyan": p.Colors.Cyan,
					"orange": p.Colors.Orange, "purple": p.Colors.Purple,
				}
				var okRole bool
				color, okRole = accents[role]
				if !okRole {
					t.Errorf("contrastExceptions[%q] references unknown role %q", name, role)
					continue
				}
				tierFloor = 3.0
				if highContrast {
					tierFloor = 4.5
				}
			}
			ratio, err := contrastRatio(color, p.Colors.Bg)
			if err != nil {
				t.Errorf("%s/%s: %v", name, role, err)
				continue
			}
			if ratio >= tierFloor {
				t.Errorf("contrastExceptions[%q][%q] is stale: %s on %s = %.2f:1 now passes the %.2f:1 tier floor",
					name, role, color, p.Colors.Bg, ratio, tierFloor)
			}
			if floor > ratio {
				t.Errorf("contrastExceptions[%q][%q] floor %.2f exceeds the measured ratio %.2f — exception floors must sit at or below the recorded measurement",
					name, role, floor, ratio)
			}
		}
	}
}
