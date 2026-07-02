package nvim

import (
	"fmt"
	"strconv"
	"strings"

	gonvim "github.com/neovim/go-client/nvim"

	"github.com/grovetools/core/tui/theme"
)

// applyGroveTheme pushes the active grove theme into the embedded neovim
// instance: it sets the 'background' option to the palette's appearance and
// defines a modest base set of highlight groups (chrome + diagnostics) from
// the fully derived theme.Palette via nvim_set_hl. It resolves the palette
// from theme.DefaultTheme.Name so a prior theme.SetTheme in this process is
// reflected. It is a silent no-op when the active theme has no registry
// palette or is an ANSI passthrough palette (e.g. "terminal"), whose index
// values cannot be expressed as nvim RGB highlights.
func applyGroveTheme(v *gonvim.Nvim) error {
	p, ok := theme.Lookup(theme.DefaultTheme.Name)
	if !ok || p.Meta.ANSI {
		return nil
	}

	b := v.NewBatch()
	b.SetOption("background", p.Meta.Appearance)
	for name, attrs := range groveHighlights(p) {
		b.SetHighlight(0, name, attrs)
	}
	if err := b.Execute(); err != nil {
		return fmt.Errorf("applying grove theme highlights: %w", err)
	}
	return nil
}

// groveHighlights maps the rich palette onto a base set of nvim highlight
// groups: enough to make a bare (--clean) instance look native to the
// active grove theme without shipping a full colorscheme.
func groveHighlights(p theme.Palette) map[string]*gonvim.HLAttrs {
	c := p.Colors
	d := c.Diagnostics

	attrs := func(fg, bg string) *gonvim.HLAttrs {
		a := &gonvim.HLAttrs{Foreground: -1, Background: -1, Special: -1}
		if fg != "" {
			a.Foreground = hexToRGBInt(fg)
		}
		if bg != "" {
			a.Background = hexToRGBInt(bg)
		}
		return a
	}

	comment := attrs(c.Comment, "")
	comment.Italic = true
	cursorLineNr := attrs(c.FgDark, "")
	cursorLineNr.Bold = true

	return map[string]*gonvim.HLAttrs{
		"Normal":       attrs(c.Fg, c.Bg),
		"NormalFloat":  attrs(c.Fg, c.BgDark),
		"FloatBorder":  attrs(c.Border, c.BgDark),
		"CursorLine":   attrs("", c.BgHighlight),
		"Visual":       attrs("", c.BgVisual),
		"LineNr":       attrs(c.FgGutter, ""),
		"CursorLineNr": cursorLineNr,
		"SignColumn":   attrs(c.FgGutter, c.Bg),
		"NonText":      attrs(c.FgGutter, ""),
		"Comment":      comment,
		"StatusLine":   attrs(c.FgDark, c.BgDark),
		"StatusLineNC": attrs(c.FgGutter, c.BgDark),
		"VertSplit":    attrs(c.Border, ""),
		"WinSeparator": attrs(c.Border, ""),
		"Pmenu":        attrs(c.Fg, c.BgDark),
		"PmenuSel":     attrs("", c.BgVisual),

		"DiagnosticError": attrs(d.Error, ""),
		"DiagnosticWarn":  attrs(d.Warning, ""),
		"DiagnosticInfo":  attrs(d.Info, ""),
		"DiagnosticHint":  attrs(d.Hint, ""),
	}
}

// hexToRGBInt converts a "#rrggbb" hex color to the packed RGB integer
// nvim_set_hl expects. Invalid input maps to -1 (unset).
func hexToRGBInt(hex string) int {
	s := strings.TrimPrefix(hex, "#")
	if len(s) != 6 {
		return -1
	}
	n, err := strconv.ParseInt(s, 16, 32)
	if err != nil {
		return -1
	}
	return int(n)
}
