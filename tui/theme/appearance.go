package theme

import "github.com/charmbracelet/lipgloss"

// Appearance is the light/dark classification of the colors a consumer is
// actually being handed. It exists for out-of-process consumers — a sidecar
// panel receiving resolved theme tokens over embed/v1 — which get hex strings
// and no way to tell whether they are painting on a light or a dark canvas.
//
// In-process that question never comes up: an lipgloss.AdaptiveColor resolves
// itself against the terminal, so nothing needs to ask. A resolved color has
// already lost that information, which is why it has to travel alongside.

// Appearance reports the active theme's appearance, "dark" or "light".
func Appearance() string { return AppearanceOf(DefaultTheme.Name) }

// AppearanceOf reports the appearance of a named theme.
//
// The answer comes from two different places on purpose:
//
//   - A single palette name (e.g. "kanagawa-lotus") builds STATIC colors from
//     that palette, so its declared meta.appearance is authoritative — even on
//     a terminal whose background disagrees. Selecting a light theme in a dark
//     terminal is a choice, not a mistake, and the colors really are light.
//   - A family name with both variants (e.g. "kanagawa") builds ADAPTIVE
//     colors, which lipgloss resolves against the detected background. There
//     the terminal decides, so this has to ask the same oracle lipgloss does,
//     or the reported appearance and the delivered colors would disagree.
//
// An unknown name falls back to the terminal's background, which is what the
// fallback palette's ANSI colors will render against.
func AppearanceOf(name string) string {
	key := normalizeThemeName(name)
	if alias, ok := themeAliases[key]; ok {
		key = alias
	}
	if p, ok := registry.palettes[key]; ok {
		if p.Meta.Appearance == "light" {
			return "light"
		}
		return "dark"
	}
	if entry, ok := registry.families[key]; ok {
		switch {
		case entry.dark != nil && entry.light != nil:
			return terminalAppearance()
		case entry.light != nil:
			return "light"
		case entry.dark != nil:
			return "dark"
		}
	}
	return terminalAppearance()
}

// terminalAppearance asks lipgloss the same question AdaptiveColor asks when
// it resolves, so a reported appearance and the colors sent beside it cannot
// disagree.
func terminalAppearance() string {
	if lipgloss.HasDarkBackground() {
		return "dark"
	}
	return "light"
}
