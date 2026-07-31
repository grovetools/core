package sidecar

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/grovetools/core/panelkit/panelproto"
)

// Palette renders text in the host's theme.
//
// Every sidecar written so far carried its own hex→SGR converter, and each
// one covered a slightly different subset of what the host might send: one
// handled "#rrggbb" only, another assumed ANSI indices. The host sends
// whichever the palette resolved to, so a converter that handles one of them
// silently drops half the theme.
//
// The zero Palette is valid and renders everything unstyled, which is what a
// panel with no host should do — not guess at colors that will clash with
// chrome it cannot see.
type Palette struct {
	theme panelproto.Theme
}

// NewPalette builds a palette from a wire theme. A nil theme yields the
// unstyled zero Palette.
func NewPalette(t *panelproto.Theme) *Palette {
	if t == nil {
		return &Palette{}
	}
	return &Palette{theme: *t}
}

// Theme returns the wire theme this palette renders, for a panel that wants a
// token the helpers below do not name.
func (p *Palette) Theme() panelproto.Theme { return p.theme }

// Name is the resolved theme name, e.g. "kanagawa".
func (p *Palette) Name() string { return p.theme.Name }

// Dark reports whether the colors were resolved against a dark canvas. A panel
// choosing its own shades — a chart, a diff background — needs this, because a
// resolved color no longer says what it was resolved for. Unknown appearance
// reads as dark, which is the safer guess for a terminal.
func (p *Palette) Dark() bool {
	return !strings.EqualFold(p.theme.Appearance, "light")
}

// The semantic roles, each rendering text in that token's color. An unset
// token returns the text unchanged rather than falling back to a guess: a
// missing color is better than the wrong one next to host chrome.
func (p *Palette) Text(s string) string      { return p.fg(textToken(p.theme), s) }
func (p *Palette) Muted(s string) string     { return p.fg(p.theme.Muted, s) }
func (p *Palette) Accent(s string) string    { return p.fg(p.theme.Accent, s) }
func (p *Palette) Border(s string) string    { return p.fg(p.theme.Border, s) }
func (p *Palette) Error(s string) string     { return p.fg(p.theme.Error, s) }
func (p *Palette) Warning(s string) string   { return p.fg(p.theme.Warning, s) }
func (p *Palette) Success(s string) string   { return p.fg(p.theme.Success, s) }
func (p *Palette) Broadcast(s string) string { return p.fg(p.theme.Broadcast, s) }

// Selected renders a selected row: the selection foreground on the selection
// background. The two are applied together because using one without the other
// is how a selected row ends up unreadable.
func (p *Palette) Selected(s string) string {
	return p.pair(p.theme.SelectionFg, p.theme.SelectionBg, s)
}

// OnAccent renders text on an accent fill — a filled tab, a selected pill.
func (p *Palette) OnAccent(s string) string {
	return p.pair(p.theme.OnAccent, p.theme.Accent, s)
}

// Bold, Italic, Faint and Underline are the attributes, independent of color.
func (p *Palette) Bold(s string) string      { return wrap("1", s) }
func (p *Palette) Faint(s string) string     { return wrap("2", s) }
func (p *Palette) Italic(s string) string    { return wrap("3", s) }
func (p *Palette) Underline(s string) string { return wrap("4", s) }

// Fg renders text in an arbitrary theme token — a hex string or an ANSI index,
// as the host sends them. Unrecognised input returns the text unstyled.
func (p *Palette) Fg(color, s string) string { return p.fg(color, s) }

// Bg renders text on an arbitrary theme token.
func (p *Palette) Bg(color, s string) string {
	code, ok := sgr(color, false)
	if !ok {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func (p *Palette) fg(color, s string) string {
	code, ok := sgr(color, true)
	if !ok {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func (p *Palette) pair(fg, bg, s string) string {
	var codes []string
	if c, ok := sgr(fg, true); ok {
		codes = append(codes, c)
	}
	if c, ok := sgr(bg, false); ok {
		codes = append(codes, c)
	}
	if len(codes) == 0 {
		return s
	}
	return "\x1b[" + strings.Join(codes, ";") + "m" + s + "\x1b[0m"
}

func wrap(attr, s string) string { return "\x1b[" + attr + "m" + s + "\x1b[0m" }

// textToken prefers Text and falls back to Foreground. Both carry the same
// role and both are on the wire so neither vocabulary has a hole; a host may
// send either.
func textToken(t panelproto.Theme) string {
	if t.Text != "" {
		return t.Text
	}
	return t.Foreground
}

// sgr converts a theme token to an SGR parameter, foreground or background.
//
// The host sends whatever its palette resolved to, which is either a hex
// string or an ANSI index — a converter that handles one silently drops half
// the theme. Both forms are accepted, plus the "#" being optional.
func sgr(color string, foreground bool) (string, bool) {
	color = strings.TrimSpace(color)
	if color == "" {
		return "", false
	}

	// An ANSI index: 0-7 and 8-15 have dedicated codes, 16-255 go through the
	// 256-color form. Using the dedicated codes for the first sixteen is what
	// keeps a panel following the user's terminal palette rather than pinning
	// their theme's idea of "red".
	if n, err := strconv.Atoi(color); err == nil && n >= 0 && n <= 255 {
		switch {
		case n < 8:
			base := 30
			if !foreground {
				base = 40
			}
			return strconv.Itoa(base + n), true
		case n < 16:
			base := 90
			if !foreground {
				base = 100
			}
			return strconv.Itoa(base + n - 8), true
		default:
			lead := "38"
			if !foreground {
				lead = "48"
			}
			return lead + ";5;" + strconv.Itoa(n), true
		}
	}

	r, g, b, ok := parseHex(color)
	if !ok {
		return "", false
	}
	lead := "38"
	if !foreground {
		lead = "48"
	}
	return fmt.Sprintf("%s;2;%d;%d;%d", lead, r, g, b), true
}

// parseHex accepts "#rrggbb", "rrggbb", "#rgb" and "rgb".
func parseHex(s string) (r, g, b int, ok bool) {
	s = strings.TrimPrefix(s, "#")
	switch len(s) {
	case 3:
		// The shorthand doubles each digit: #f0a is #ff00aa.
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	case 6:
	default:
		return 0, 0, 0, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return int(v>>16) & 0xff, int(v>>8) & 0xff, int(v) & 0xff, true
}
