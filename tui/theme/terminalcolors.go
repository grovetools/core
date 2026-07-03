package theme

import "github.com/charmbracelet/lipgloss"

// ActiveTerminalColors resolves the active theme's terminal-facing colors:
// the default foreground/background plus the 16 ANSI slot colors, all as
// "#rrggbb" hex strings. Consumers use them to recolor real terminals —
// OSC 11/10 emission toward the outer terminal and default-color injection
// into embedded ghostty panes.
//
// The runtime Theme only retains flattened legacy Colors, so this re-resolves
// the full Palette from the registry under DefaultTheme.Name. Family names
// pick the variant matching the terminal appearance the same way the adaptive
// legacy colors do (lipgloss dark-background detection, sampled at startup
// and kept for the session).
//
// ok is false for ANSI passthrough palettes (e.g. the "terminal" theme),
// whose values are ANSI indices rather than hex colors — callers must
// reset/clear any previously applied colors instead of setting new ones.
func ActiveTerminalColors() (fg, bg string, palette [16]string, ok bool) {
	return terminalColorsFor(DefaultTheme.Name, lipgloss.HasDarkBackground())
}

// terminalColorsFor is the testable core of ActiveTerminalColors with the
// theme name and appearance made explicit.
func terminalColorsFor(name string, dark bool) (fg, bg string, palette [16]string, ok bool) {
	p, found := lookupForAppearance(name, dark)
	if !found {
		// Mirror resolveThemeColors: an unknown name renders with the
		// default theme's colors, so report that theme's values.
		if p, found = lookupForAppearance(DefaultThemeName, dark); !found {
			return "", "", palette, false
		}
	}
	if p.Meta.ANSI {
		return "", "", palette, false
	}
	t := p.Terminal
	palette = [16]string{
		t.Black, t.Red, t.Green, t.Yellow,
		t.Blue, t.Magenta, t.Cyan, t.White,
		t.BlackBright, t.RedBright, t.GreenBright, t.YellowBright,
		t.BlueBright, t.MagentaBright, t.CyanBright, t.WhiteBright,
	}
	return p.Colors.Fg, p.Colors.Bg, palette, true
}

// lookupForAppearance resolves name like Lookup, except family names pick
// the variant for the given appearance (falling back to whichever exists)
// instead of always preferring dark.
func lookupForAppearance(name string, dark bool) (Palette, bool) {
	key := normalizeThemeName(name)
	if alias, ok := themeAliases[key]; ok {
		key = alias
	}
	if p, ok := registry.palettes[key]; ok {
		return *p, true
	}
	if entry, ok := registry.families[key]; ok {
		p := entry.dark
		if (!dark && entry.light != nil) || p == nil {
			p = entry.light
		}
		if p != nil {
			return *p, true
		}
	}
	return Palette{}, false
}
