package theme

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// This file pins the legacy Colors derived for the original three themes
// (kanagawa, gruvbox, terminal) so the TOML palettes cannot drift silently.
//
// Phase 2b intentionally broke parity with the pre-TOML hand-coded values
// for kanagawa and gruvbox: both families were re-extracted from their
// upstreams (rebelot/kanagawa.nvim wave/lotus and gruvbox-community/gruvbox)
// and upstream fidelity won over grove's hand-coded approximations. The
// constants below pin the new upstream-derived values; each one matches the
// corresponding themes/*.toml role verbatim (or its documented derivation,
// e.g. DarkText = fg_inverse = bg_dark). The terminal palette is unchanged
// from the original hand-coded values.

// --- Kanagawa Wave (dark family default; themes/kanagawa-wave.toml) ---
const (
	kanagawaDarkGreen                = "#98bb6c"
	kanagawaDarkYellow               = "#e6c384"
	kanagawaDarkRed                  = "#ff5d62"
	kanagawaDarkOrange               = "#ffa066"
	kanagawaDarkCyan                 = "#7aa89f"
	kanagawaDarkBlue                 = "#7e9cd8"
	kanagawaDarkViolet               = "#957fb8"
	kanagawaDarkPink                 = "#d27e99"
	kanagawaDarkLightText            = "#dcd7ba"
	kanagawaDarkMutedText            = "#727169"
	kanagawaDarkDarkText             = "#181820"
	kanagawaDarkBorder               = "#54546d"
	kanagawaDarkSelectedBackground   = "#223249"
	kanagawaDarkSubtleBackground     = "#1f1f28"
	kanagawaDarkVerySubtleBackground = "#181820"
)

// --- Kanagawa Lotus (light family default; themes/kanagawa-lotus.toml) ---
const (
	kanagawaLightGreen                = "#6f894e"
	kanagawaLightYellow               = "#77713f"
	kanagawaLightRed                  = "#c84053"
	kanagawaLightOrange               = "#cc6d00"
	kanagawaLightCyan                 = "#597b75"
	kanagawaLightBlue                 = "#4d699b"
	kanagawaLightViolet               = "#624c83"
	kanagawaLightPink                 = "#b35b79"
	kanagawaLightLightText            = "#545464"
	kanagawaLightMutedText            = "#8a8980"
	kanagawaLightDarkText             = "#dcd5ac"
	kanagawaLightBorder               = "#716e61"
	kanagawaLightSelectedBackground   = "#c9cbd1"
	kanagawaLightSubtleBackground     = "#f2ecbc"
	kanagawaLightVerySubtleBackground = "#dcd5ac"
)

// --- Gruvbox (medium contrast; themes/gruvbox-dark.toml / -light.toml) ---
const (
	gruvboxDarkGreen                 = "#b8bb26"
	gruvboxLightGreen                = "#79740e"
	gruvboxDarkYellow                = "#fabd2f"
	gruvboxLightYellow               = "#b57614"
	gruvboxDarkRed                   = "#fb4934"
	gruvboxLightRed                  = "#9d0006"
	gruvboxDarkOrange                = "#fe8019"
	gruvboxLightOrange               = "#af3a03"
	gruvboxDarkCyan                  = "#8ec07c"
	gruvboxLightCyan                 = "#427b58"
	gruvboxDarkBlue                  = "#83a598"
	gruvboxLightBlue                 = "#076678"
	gruvboxDarkViolet                = "#b16286"
	gruvboxLightViolet               = "#8f3f71"
	gruvboxDarkPink                  = "#d3869b"
	gruvboxLightPink                 = "#b16286"
	gruvboxDarkLightText             = "#ebdbb2"
	gruvboxLightLightText            = "#3c3836"
	gruvboxDarkMutedText             = "#928374"
	gruvboxLightMutedText            = "#928374"
	gruvboxDarkDarkText              = "#1d2021"
	gruvboxLightDarkText             = "#f9f5d7"
	gruvboxDarkBorder                = "#665c54"
	gruvboxLightBorder               = "#bdae93"
	gruvboxDarkSelectedBackground    = "#665c54"
	gruvboxLightSelectedBackground   = "#bdae93"
	gruvboxDarkSubtleBackground      = "#282828"
	gruvboxLightSubtleBackground     = "#fbf1c7"
	gruvboxDarkVerySubtleBackground  = "#1d2021"
	gruvboxLightVerySubtleBackground = "#f9f5d7"
)

// --- Terminal (ANSI-friendly) palette ---
const (
	terminalGreen                = "2"
	terminalYellow               = "3"
	terminalRed                  = "1"
	terminalOrange               = "11"
	terminalCyan                 = "6"
	terminalBlue                 = "4"
	terminalViolet               = "5"
	terminalPink                 = "13"
	terminalLightText            = "7"
	terminalMutedText            = "8"
	terminalDarkText             = "0"
	terminalBorder               = "8"
	terminalSelectedBackground   = "8"
	terminalSubtleBackground     = "0"
	terminalVerySubtleBackground = "0"
)

func goldenKanagawaColors() Colors {
	return Colors{
		Green:                lipgloss.AdaptiveColor{Light: kanagawaLightGreen, Dark: kanagawaDarkGreen},
		Yellow:               lipgloss.AdaptiveColor{Light: kanagawaLightYellow, Dark: kanagawaDarkYellow},
		Red:                  lipgloss.AdaptiveColor{Light: kanagawaLightRed, Dark: kanagawaDarkRed},
		Orange:               lipgloss.AdaptiveColor{Light: kanagawaLightOrange, Dark: kanagawaDarkOrange},
		Cyan:                 lipgloss.AdaptiveColor{Light: kanagawaLightCyan, Dark: kanagawaDarkCyan},
		Blue:                 lipgloss.AdaptiveColor{Light: kanagawaLightBlue, Dark: kanagawaDarkBlue},
		Violet:               lipgloss.AdaptiveColor{Light: kanagawaLightViolet, Dark: kanagawaDarkViolet},
		Pink:                 lipgloss.AdaptiveColor{Light: kanagawaLightPink, Dark: kanagawaDarkPink},
		LightText:            lipgloss.AdaptiveColor{Light: kanagawaLightLightText, Dark: kanagawaDarkLightText},
		MutedText:            lipgloss.AdaptiveColor{Light: kanagawaLightMutedText, Dark: kanagawaDarkMutedText},
		DarkText:             lipgloss.AdaptiveColor{Light: kanagawaLightDarkText, Dark: kanagawaDarkDarkText},
		Border:               lipgloss.AdaptiveColor{Light: kanagawaLightBorder, Dark: kanagawaDarkBorder},
		SelectedBackground:   lipgloss.AdaptiveColor{Light: kanagawaLightSelectedBackground, Dark: kanagawaDarkSelectedBackground},
		SubtleBackground:     lipgloss.AdaptiveColor{Light: kanagawaLightSubtleBackground, Dark: kanagawaDarkSubtleBackground},
		VerySubtleBackground: lipgloss.AdaptiveColor{Light: kanagawaLightVerySubtleBackground, Dark: kanagawaDarkVerySubtleBackground},
	}
}

func goldenGruvboxColors() Colors {
	return Colors{
		Green:                lipgloss.AdaptiveColor{Light: gruvboxLightGreen, Dark: gruvboxDarkGreen},
		Yellow:               lipgloss.AdaptiveColor{Light: gruvboxLightYellow, Dark: gruvboxDarkYellow},
		Red:                  lipgloss.AdaptiveColor{Light: gruvboxLightRed, Dark: gruvboxDarkRed},
		Orange:               lipgloss.AdaptiveColor{Light: gruvboxLightOrange, Dark: gruvboxDarkOrange},
		Cyan:                 lipgloss.AdaptiveColor{Light: gruvboxLightCyan, Dark: gruvboxDarkCyan},
		Blue:                 lipgloss.AdaptiveColor{Light: gruvboxLightBlue, Dark: gruvboxDarkBlue},
		Violet:               lipgloss.AdaptiveColor{Light: gruvboxLightViolet, Dark: gruvboxDarkViolet},
		Pink:                 lipgloss.AdaptiveColor{Light: gruvboxLightPink, Dark: gruvboxDarkPink},
		LightText:            lipgloss.AdaptiveColor{Light: gruvboxLightLightText, Dark: gruvboxDarkLightText},
		MutedText:            lipgloss.AdaptiveColor{Light: gruvboxLightMutedText, Dark: gruvboxDarkMutedText},
		DarkText:             lipgloss.AdaptiveColor{Light: gruvboxLightDarkText, Dark: gruvboxDarkDarkText},
		Border:               lipgloss.AdaptiveColor{Light: gruvboxLightBorder, Dark: gruvboxDarkBorder},
		SelectedBackground:   lipgloss.AdaptiveColor{Light: gruvboxLightSelectedBackground, Dark: gruvboxDarkSelectedBackground},
		SubtleBackground:     lipgloss.AdaptiveColor{Light: gruvboxLightSubtleBackground, Dark: gruvboxDarkSubtleBackground},
		VerySubtleBackground: lipgloss.AdaptiveColor{Light: gruvboxLightVerySubtleBackground, Dark: gruvboxDarkVerySubtleBackground},
	}
}

func goldenTerminalColors() Colors {
	return Colors{
		Green:                lipgloss.Color(terminalGreen),
		Yellow:               lipgloss.Color(terminalYellow),
		Red:                  lipgloss.Color(terminalRed),
		Orange:               lipgloss.Color(terminalOrange),
		Cyan:                 lipgloss.Color(terminalCyan),
		Blue:                 lipgloss.Color(terminalBlue),
		Violet:               lipgloss.Color(terminalViolet),
		Pink:                 lipgloss.Color(terminalPink),
		LightText:            lipgloss.Color(terminalLightText),
		MutedText:            lipgloss.Color(terminalMutedText),
		DarkText:             lipgloss.Color(terminalDarkText),
		Border:               lipgloss.Color(terminalBorder),
		SelectedBackground:   lipgloss.Color(terminalSelectedBackground),
		SubtleBackground:     lipgloss.Color(terminalSubtleBackground),
		VerySubtleBackground: lipgloss.Color(terminalVerySubtleBackground),
	}
}

func compareColors(t *testing.T, theme string, want, got Colors) {
	t.Helper()
	fields := []struct {
		name      string
		want, got lipgloss.TerminalColor
	}{
		{"Green", want.Green, got.Green},
		{"Yellow", want.Yellow, got.Yellow},
		{"Red", want.Red, got.Red},
		{"Orange", want.Orange, got.Orange},
		{"Cyan", want.Cyan, got.Cyan},
		{"Blue", want.Blue, got.Blue},
		{"Violet", want.Violet, got.Violet},
		{"Pink", want.Pink, got.Pink},
		{"LightText", want.LightText, got.LightText},
		{"MutedText", want.MutedText, got.MutedText},
		{"DarkText", want.DarkText, got.DarkText},
		{"Border", want.Border, got.Border},
		{"SelectedBackground", want.SelectedBackground, got.SelectedBackground},
		{"SubtleBackground", want.SubtleBackground, got.SubtleBackground},
		{"VerySubtleBackground", want.VerySubtleBackground, got.VerySubtleBackground},
	}
	for _, f := range fields {
		if f.want != f.got {
			t.Errorf("%s.%s = %#v, want %#v", theme, f.name, f.got, f.want)
		}
	}
}

// TestLegacyColorsGoldenParity asserts that the Colors derived from the TOML
// palettes are identical to the pinned constants above.
func TestLegacyColorsGoldenParity(t *testing.T) {
	compareColors(t, "kanagawa", goldenKanagawaColors(), resolveThemeColors("kanagawa"))
	compareColors(t, "gruvbox", goldenGruvboxColors(), resolveThemeColors("gruvbox"))
	compareColors(t, "terminal", goldenTerminalColors(), resolveThemeColors("terminal"))
}

// TestAliasesResolveToFamilies asserts alias behavior: family-style names
// resolve to the family's adaptive colors, and the legacy kanagawa variant
// names resolve to the upstream variants that replaced them in Phase 2b.
func TestAliasesResolveToFamilies(t *testing.T) {
	for _, alias := range []string{"branded", "Kanagawa"} {
		compareColors(t, alias, goldenKanagawaColors(), resolveThemeColors(alias))
	}
	// Old grove variant names point at the upstream palettes that replaced
	// their hand-coded predecessors.
	compareColors(t, "kanagawa-dark", resolveThemeColors("kanagawa-wave"), resolveThemeColors("kanagawa-dark"))
	compareColors(t, "kanagawa-light", resolveThemeColors("kanagawa-lotus"), resolveThemeColors("kanagawa-light"))
}

// TestVariantNamesResolveToVariantPalettes asserts that gruvbox-dark and
// gruvbox-light select their real variant palettes (static colors), not the
// adaptive family: the former aliases onto the family shadowed the variant
// palettes, which made e.g. the family's light slot unreachable for the
// theme_changed payload builder.
func TestVariantNamesResolveToVariantPalettes(t *testing.T) {
	adaptive := goldenGruvboxColors()
	darkHalf := func(c lipgloss.TerminalColor) lipgloss.TerminalColor {
		return lipgloss.Color(c.(lipgloss.AdaptiveColor).Dark)
	}
	lightHalf := func(c lipgloss.TerminalColor) lipgloss.TerminalColor {
		return lipgloss.Color(c.(lipgloss.AdaptiveColor).Light)
	}
	half := func(pick func(lipgloss.TerminalColor) lipgloss.TerminalColor) Colors {
		return Colors{
			Green:                pick(adaptive.Green),
			Yellow:               pick(adaptive.Yellow),
			Red:                  pick(adaptive.Red),
			Orange:               pick(adaptive.Orange),
			Cyan:                 pick(adaptive.Cyan),
			Blue:                 pick(adaptive.Blue),
			Violet:               pick(adaptive.Violet),
			Pink:                 pick(adaptive.Pink),
			LightText:            pick(adaptive.LightText),
			MutedText:            pick(adaptive.MutedText),
			DarkText:             pick(adaptive.DarkText),
			Border:               pick(adaptive.Border),
			SelectedBackground:   pick(adaptive.SelectedBackground),
			SubtleBackground:     pick(adaptive.SubtleBackground),
			VerySubtleBackground: pick(adaptive.VerySubtleBackground),
		}
	}
	for _, name := range []string{"gruvbox-dark", "gruvbox_dark"} {
		compareColors(t, name, half(darkHalf), resolveThemeColors(name))
	}
	compareColors(t, "gruvbox-light", half(lightHalf), resolveThemeColors("gruvbox-light"))
}

// TestUnknownThemeFallsBackToDefault asserts unrecognized names resolve to
// the default theme, as before.
func TestUnknownThemeFallsBackToDefault(t *testing.T) {
	compareColors(t, "unknown", goldenKanagawaColors(), resolveThemeColors("definitely-not-a-theme"))
	compareColors(t, "empty", goldenKanagawaColors(), resolveThemeColors(""))
}
