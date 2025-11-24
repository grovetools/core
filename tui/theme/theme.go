package theme

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattsolo1/grove-core/config"
)

const defaultThemeName = "kanagawa"

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
	terminalOrange               = "208"
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

// Colors encapsulates the palette used by a theme. lipgloss.TerminalColor
// allows a mix of adaptive and static colors.
type Colors struct {
	Green                lipgloss.TerminalColor
	Yellow               lipgloss.TerminalColor
	Red                  lipgloss.TerminalColor
	Orange               lipgloss.TerminalColor
	Cyan                 lipgloss.TerminalColor
	Blue                 lipgloss.TerminalColor
	Violet               lipgloss.TerminalColor
	Pink                 lipgloss.TerminalColor
	LightText            lipgloss.TerminalColor
	MutedText            lipgloss.TerminalColor
	DarkText             lipgloss.TerminalColor
	Border               lipgloss.TerminalColor
	SelectedBackground   lipgloss.TerminalColor
	SubtleBackground     lipgloss.TerminalColor
	VerySubtleBackground lipgloss.TerminalColor
}

// Exported color shortcuts for legacy usages. These are populated from DefaultTheme.
var (
	Green                lipgloss.TerminalColor
	Yellow               lipgloss.TerminalColor
	Red                  lipgloss.TerminalColor
	Orange               lipgloss.TerminalColor
	Cyan                 lipgloss.TerminalColor
	Blue                 lipgloss.TerminalColor
	Violet               lipgloss.TerminalColor
	Pink                 lipgloss.TerminalColor
	LightText            lipgloss.TerminalColor
	MutedText            lipgloss.TerminalColor
	DarkText             lipgloss.TerminalColor
	Border               lipgloss.TerminalColor
	SelectedBackground   lipgloss.TerminalColor
	SubtleBackground     lipgloss.TerminalColor
	VerySubtleBackground lipgloss.TerminalColor
)

// DefaultColors exposes the active color palette selected for the current terminal.
var DefaultColors Colors

// Theme holds all the pre-configured styles for the Grove ecosystem.
type Theme struct {
	Colors Colors

	// Headers and titles
	Header lipgloss.Style
	Title  lipgloss.Style

	// Status indicators
	Success lipgloss.Style
	Error   lipgloss.Style
	Warning lipgloss.Style
	Info    lipgloss.Style

	// Text styles - visual hierarchy
	Bold        lipgloss.Style // Emphasized text
	Normal      lipgloss.Style // Regular terminal default text
	Muted       lipgloss.Style // De-emphasized text
	Selected    lipgloss.Style
	SelectedRow lipgloss.Style

	// Selection styles
	SelectedUnfocused lipgloss.Style // Selected item in unfocused pane
	VisualSelection   lipgloss.Style // Visual selection mode highlight

	// Table styles
	TableHeader        lipgloss.Style
	TableRow           lipgloss.Style
	TableBorder        lipgloss.Style
	UseAlternatingRows bool // Whether to use alternating row backgrounds in tables

	// Container styles
	Box        lipgloss.Style
	Code       lipgloss.Style
	DetailsBox lipgloss.Style // Bordered panel for details views

	// Interactive elements
	Input       lipgloss.Style
	Placeholder lipgloss.Style
	Cursor      lipgloss.Style

	// Special styles
	Highlight lipgloss.Style
	Accent    lipgloss.Style

	// Workspace styles - used for displaying workspace hierarchies
	WorkspaceEcosystem lipgloss.Style // Bold - for ecosystem workspaces
	WorkspaceStandard  lipgloss.Style // Default - for standard workspaces
	WorkspaceWorktree  lipgloss.Style // Faint - for worktree branches

	// Dynamic color palette for components
	AccentColors []lipgloss.TerminalColor
}

var themeRegistry = map[string]func() Colors{
	"kanagawa": newKanagawaColors,
	"gruvbox":  newGruvboxColors,
	"terminal": newTerminalColors,
}

var themeAliases = map[string]string{
	"kanagawa-dark":   "kanagawa",
	"kanagawa-dragon": "kanagawa",
	"kanagawa-wave":   "kanagawa",
	"gruvbox-dark":    "gruvbox",
	"gruvbox-light":   "gruvbox",
	"branded":         "kanagawa",
}

// DefaultTheme is the default theme instance for the Grove ecosystem.
var DefaultTheme = initDefaultTheme()

// NewTheme creates a theme based on the configured theme selection.
func NewTheme() *Theme {
	return newThemeFromName(getThemeName())
}

// NewThemeWithName constructs a theme from a specific palette name.
func NewThemeWithName(name string) *Theme {
	return newThemeFromName(name)
}


// RenderHeader renders a header with the default Grove styling.
func RenderHeader(title string) string {
	return DefaultTheme.Header.Render(title)
}

// RenderTitle renders a title with the default Grove styling.
func RenderTitle(title string) string {
	return DefaultTheme.Title.Render(title)
}

// RenderStatus renders text with the appropriate status style.
func RenderStatus(status, text string) string {
	switch status {
	case "success":
		return DefaultTheme.Success.Render(text)
	case "error":
		return DefaultTheme.Error.Render(text)
	case "warning":
		return DefaultTheme.Warning.Render(text)
	case "info":
		return DefaultTheme.Info.Render(text)
	default:
		return text
	}
}

// RenderBox renders content inside a styled box.
func RenderBox(content string) string {
	return DefaultTheme.Box.Render(content)
}

func initDefaultTheme() *Theme {
	themeName := getThemeName()
	colors := resolveThemeColors(themeName)
	applyColors(colors)
	return newThemeFromColors(colors, themeName)
}

func newThemeFromName(name string) *Theme {
	return newThemeFromColors(resolveThemeColors(name), name)
}

func newThemeFromColors(colors Colors, themeName string) *Theme {
	// Determine if we should use alternating rows based on theme
	// Disable for terminal theme since we can't control ANSI color appearance
	normalizedName := normalizeThemeName(themeName)
	useAlternatingRows := normalizedName != "terminal"
	return &Theme{
		Colors: colors,

		Header: lipgloss.NewStyle().
			Bold(true).
			MarginTop(1).
			MarginBottom(1),

		Title: lipgloss.NewStyle().
			Bold(true).
			Underline(true).
			MarginBottom(1),

		Success: lipgloss.NewStyle().
			Foreground(colors.Green).
			Bold(true),

		Error: lipgloss.NewStyle().
			Foreground(colors.Red).
			Bold(true),

		Warning: lipgloss.NewStyle().
			Foreground(colors.Yellow).
			Bold(true),

		Info: lipgloss.NewStyle().
			Foreground(colors.Cyan).
			Bold(true),

		// Text hierarchy: Bold → Normal → Muted
		Bold: lipgloss.NewStyle().
			Bold(true),

		Normal: lipgloss.NewStyle(),

		Muted: lipgloss.NewStyle().
			Faint(true),

		Selected: lipgloss.NewStyle().
			Background(colors.SelectedBackground).
			Foreground(colors.LightText),

		SelectedRow: lipgloss.NewStyle().
			Background(colors.SelectedBackground),

		SelectedUnfocused: lipgloss.NewStyle().
			Faint(true).
			Underline(true),

		VisualSelection: lipgloss.NewStyle().
			Reverse(true),

		TableHeader: lipgloss.NewStyle().
			Bold(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true).
			BorderForeground(colors.Border),

		TableRow: lipgloss.NewStyle(),

		TableBorder: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(colors.Border),

		UseAlternatingRows: useAlternatingRows,

		Box: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colors.Border).
			Padding(1, 2).
			Margin(1, 0),

		DetailsBox: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(colors.Violet).
			Padding(0, 1),

		Code: lipgloss.NewStyle().
			Background(colors.SubtleBackground).
			Foreground(colors.LightText).
			Padding(0, 1).
			MarginLeft(2),

		Input: lipgloss.NewStyle().
			Foreground(colors.LightText),

		Placeholder: lipgloss.NewStyle().
			Foreground(colors.MutedText).
			Italic(true),

		Cursor: lipgloss.NewStyle().
			Foreground(colors.Orange).
			Bold(true),

		Highlight: lipgloss.NewStyle().
			Foreground(colors.Orange).
			Bold(true),

		Accent: lipgloss.NewStyle().
			Foreground(colors.Violet).
			Bold(true),

		// Workspace styles use weight for hierarchy without explicit colors
		WorkspaceEcosystem: lipgloss.NewStyle().
			Bold(true),

		WorkspaceStandard: lipgloss.NewStyle(),

		WorkspaceWorktree: lipgloss.NewStyle().
			Faint(true),

		AccentColors: []lipgloss.TerminalColor{
			colors.Cyan,
			colors.Blue,
			colors.Violet,
			colors.Pink,
			colors.Green,
			colors.Orange,
		},
	}
}

func applyColors(colors Colors) {
	DefaultColors = colors

	Green = colors.Green
	Yellow = colors.Yellow
	Red = colors.Red
	Orange = colors.Orange
	Cyan = colors.Cyan
	Blue = colors.Blue
	Violet = colors.Violet
	Pink = colors.Pink
	LightText = colors.LightText
	MutedText = colors.MutedText
	DarkText = colors.DarkText
	Border = colors.Border
	SelectedBackground = colors.SelectedBackground
	SubtleBackground = colors.SubtleBackground
	VerySubtleBackground = colors.VerySubtleBackground
}

func resolveThemeColors(name string) Colors {
	key := normalizeThemeName(name)
	if alias, ok := themeAliases[key]; ok {
		key = alias
	}
	if builder, ok := themeRegistry[key]; ok {
		return builder()
	}
	return themeRegistry[defaultThemeName]()
}

func normalizeThemeName(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	normalized = strings.ReplaceAll(normalized, " ", "-")
	normalized = strings.ReplaceAll(normalized, "_", "-")
	return normalized
}

func getThemeName() string {
	if theme := normalizeThemeName(os.Getenv("GROVE_THEME")); theme != "" {
		return theme
	}

	cfg, err := config.LoadDefault()
	if err != nil || cfg == nil {
		return defaultThemeName
	}

	var tuiCfg struct {
		Theme string `yaml:"theme"`
	}
	if err := cfg.UnmarshalExtension("tui", &tuiCfg); err == nil {
		if theme := normalizeThemeName(tuiCfg.Theme); theme != "" {
			return theme
		}
	}

	return defaultThemeName
}

func newKanagawaColors() Colors {
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

func newGruvboxColors() Colors {
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

func newTerminalColors() Colors {
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
