package theme

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// This file pins the legacy Colors values that were hand-coded in theme.go
// before themes became TOML data files. The derived legacy Colors for the
// original three themes must stay byte-identical to these constants.

// --- Kanagawa Dragon (dark) palette ---
const (
	kanagawaDarkGreen                = "#98BB6C"
	kanagawaDarkYellow               = "#FF9E3B"
	kanagawaDarkRed                  = "#FF5D62"
	kanagawaDarkOrange               = "#FFA066"
	kanagawaDarkCyan                 = "#7E9CD8"
	kanagawaDarkBlue                 = "#7FB4CA"
	kanagawaDarkViolet               = "#957FB8"
	kanagawaDarkPink                 = "#D27E99"
	kanagawaDarkLightText            = "#DCD7BA"
	kanagawaDarkMutedText            = "#727169"
	kanagawaDarkDarkText             = "#1D1C19"
	kanagawaDarkBorder               = "#363646"
	kanagawaDarkSelectedBackground   = "#223249"
	kanagawaDarkSubtleBackground     = "#1F1F28"
	kanagawaDarkVerySubtleBackground = "#181820"
)

// --- Kanagawa Wave (light-inspired) palette ---
const (
	kanagawaLightGreen                = "#4E7C5A"
	kanagawaLightYellow               = "#A68A64"
	kanagawaLightRed                  = "#C34043"
	kanagawaLightOrange               = "#CC6B4E"
	kanagawaLightCyan                 = "#5B8BBE"
	kanagawaLightBlue                 = "#4F7CAC"
	kanagawaLightViolet               = "#674D7A"
	kanagawaLightPink                 = "#B35C74"
	kanagawaLightLightText            = "#2B2F42"
	kanagawaLightMutedText            = "#6C7086"
	kanagawaLightDarkText             = "#E6E9EF"
	kanagawaLightBorder               = "#B5BDC5"
	kanagawaLightSelectedBackground   = "#E2E6F3"
	kanagawaLightSubtleBackground     = "#F7F7FB"
	kanagawaLightVerySubtleBackground = "#EFF1F8"
)

// --- Gruvbox palette ---
const (
	gruvboxDarkGreen                 = "#B8BB26"
	gruvboxLightGreen                = "#98971A"
	gruvboxDarkYellow                = "#FABD2F"
	gruvboxLightYellow               = "#D79921"
	gruvboxDarkRed                   = "#FB4934"
	gruvboxLightRed                  = "#CC241D"
	gruvboxDarkOrange                = "#FE8019"
	gruvboxLightOrange               = "#D65D0E"
	gruvboxDarkCyan                  = "#83A598"
	gruvboxLightCyan                 = "#458588"
	gruvboxDarkBlue                  = "#458588"
	gruvboxLightBlue                 = "#076678"
	gruvboxDarkViolet                = "#B16286"
	gruvboxLightViolet               = "#8F3F71"
	gruvboxDarkPink                  = "#D3869B"
	gruvboxLightPink                 = "#B57679"
	gruvboxDarkLightText             = "#EBDBB2"
	gruvboxLightLightText            = "#3C3836"
	gruvboxDarkMutedText             = "#BDAE93"
	gruvboxLightMutedText            = "#928374"
	gruvboxDarkDarkText              = "#1D2021"
	gruvboxLightDarkText             = "#F9F5D7"
	gruvboxDarkBorder                = "#504945"
	gruvboxLightBorder               = "#D5C4A1"
	gruvboxDarkSelectedBackground    = "#32302F"
	gruvboxLightSelectedBackground   = "#F2E5BC"
	gruvboxDarkSubtleBackground      = "#282828"
	gruvboxLightSubtleBackground     = "#FBF1C7"
	gruvboxDarkVerySubtleBackground  = "#1D2021"
	gruvboxLightVerySubtleBackground = "#F9F5D7"
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
// palettes are identical to the constants previously hand-coded in theme.go.
func TestLegacyColorsGoldenParity(t *testing.T) {
	compareColors(t, "kanagawa", goldenKanagawaColors(), resolveThemeColors("kanagawa"))
	compareColors(t, "gruvbox", goldenGruvboxColors(), resolveThemeColors("gruvbox"))
	compareColors(t, "terminal", goldenTerminalColors(), resolveThemeColors("terminal"))
}

// TestAliasesResolveToFamilies asserts the pre-existing alias behavior is
// unchanged: variant-style names resolve to the family's adaptive colors.
func TestAliasesResolveToFamilies(t *testing.T) {
	for _, alias := range []string{"kanagawa-dark", "kanagawa-dragon", "kanagawa-wave", "branded", "Kanagawa Dragon"} {
		compareColors(t, alias, goldenKanagawaColors(), resolveThemeColors(alias))
	}
	for _, alias := range []string{"gruvbox-dark", "gruvbox-light", "gruvbox_dark"} {
		compareColors(t, alias, goldenGruvboxColors(), resolveThemeColors(alias))
	}
}

// TestUnknownThemeFallsBackToDefault asserts unrecognized names resolve to
// the default theme, as before.
func TestUnknownThemeFallsBackToDefault(t *testing.T) {
	compareColors(t, "unknown", goldenKanagawaColors(), resolveThemeColors("definitely-not-a-theme"))
	compareColors(t, "empty", goldenKanagawaColors(), resolveThemeColors(""))
}
