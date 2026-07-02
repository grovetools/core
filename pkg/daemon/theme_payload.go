package daemon

import (
	"github.com/grovetools/core/tui/theme"
)

// BuildThemePayload assembles the theme_changed wire payload for a theme
// selection (family name, variant name, or legacy alias) from core's theme
// registry. The selected variant occupies its own appearance slot; the
// family's default variant for the opposite appearance fills the other slot
// when the family has one. It is the ONE payload-assembly implementation
// shared by the daemon (theme_changed broadcasts + initial-snapshot stamps)
// and grove.nvim's `internal theme` subcommand, so every consumer parses an
// identical shape.
func BuildThemePayload(name string) (*ThemeChangedPayload, bool) {
	selected, ok := theme.Lookup(name)
	if !ok {
		return nil, false
	}

	dark, light := themeFamilyDefaults(selected.Meta.Family)
	if selected.Meta.Appearance == "light" {
		light = &selected
	} else {
		dark = &selected
	}

	mode := "hex"
	if selected.Meta.ANSI {
		mode = "ansi"
	}

	return &ThemeChangedPayload{
		Name:   theme.NormalizeName(name),
		Family: selected.Meta.Family,
		Mode:   mode,
		Dark:   themeWirePalette(dark),
		Light:  themeWirePalette(light),
	}, true
}

// themeFamilyDefaults finds the family's default palette per appearance,
// replicating the registry's rule (first variant by name wins unless a later
// one is flagged default). Lookup resolves legacy aliases before exact names,
// so a variant whose name is shadowed by an alias (e.g. an alias pointing a
// variant name at its family) resolves elsewhere; such slots are dropped
// rather than filled with the wrong appearance.
func themeFamilyDefaults(family string) (dark, light *theme.Palette) {
	for _, meta := range theme.List() {
		if meta.Family != family {
			continue
		}
		p, ok := theme.Lookup(meta.Name)
		if !ok || p.Meta.Name != meta.Name || p.Meta.Appearance != meta.Appearance {
			continue // alias-shadowed name; skip rather than mis-slot
		}
		switch meta.Appearance {
		case "dark":
			if dark == nil || (meta.Default && !dark.Meta.Default) {
				dark = &p
			}
		case "light":
			if light == nil || (meta.Default && !light.Meta.Default) {
				light = &p
			}
		}
	}
	return dark, light
}

// themeWirePalette maps a fully derived theme.Palette onto the JSON wire
// struct.
func themeWirePalette(p *theme.Palette) *ThemePalette {
	if p == nil {
		return nil
	}
	c := p.Colors
	t := p.Terminal
	return &ThemePalette{
		Name:       p.Meta.Name,
		Variant:    p.Meta.Variant,
		Appearance: p.Meta.Appearance,

		Bg:          c.Bg,
		BgDark:      c.BgDark,
		BgHighlight: c.BgHighlight,
		BgVisual:    c.BgVisual,

		Fg:        c.Fg,
		FgDark:    c.FgDark,
		FgGutter:  c.FgGutter,
		FgInverse: c.FgInverse,
		Comment:   c.Comment,
		Border:    c.Border,

		Red:     c.Red,
		Green:   c.Green,
		Yellow:  c.Yellow,
		Blue:    c.Blue,
		Magenta: c.Magenta,
		Cyan:    c.Cyan,
		Orange:  c.Orange,
		Purple:  c.Purple,

		Git: ThemeGitColors{
			Add:    c.Git.Add,
			Change: c.Git.Change,
			Delete: c.Git.Delete,
		},
		Diagnostics: ThemeDiagnosticColors{
			Error:   c.Diagnostics.Error,
			Warning: c.Diagnostics.Warning,
			Info:    c.Diagnostics.Info,
			Hint:    c.Diagnostics.Hint,
		},
		Terminal: ThemeTerminalColors{
			Black:         t.Black,
			Red:           t.Red,
			Green:         t.Green,
			Yellow:        t.Yellow,
			Blue:          t.Blue,
			Magenta:       t.Magenta,
			Cyan:          t.Cyan,
			White:         t.White,
			BlackBright:   t.BlackBright,
			RedBright:     t.RedBright,
			GreenBright:   t.GreenBright,
			YellowBright:  t.YellowBright,
			BlueBright:    t.BlueBright,
			MagentaBright: t.MagentaBright,
			CyanBright:    t.CyanBright,
			WhiteBright:   t.WhiteBright,
		},
	}
}
