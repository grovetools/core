package theme

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/grovetools/core/config"
)

const defaultThemeName = "kanagawa"

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

// ResolveColor maps a color name (e.g. "blue", "border", "muted_text") to the
// corresponding theme color. If the name starts with "#", it's treated as a
// hex color literal. Returns the fallback if the name is empty or unrecognized.
func (c Colors) ResolveColor(name string, fallback lipgloss.TerminalColor) lipgloss.TerminalColor {
	if name == "" {
		return fallback
	}
	if strings.ToLower(name) == "none" {
		return lipgloss.NoColor{}
	}
	if name[0] == '#' {
		return lipgloss.Color(name)
	}
	switch strings.ToLower(name) {
	case "green":
		return c.Green
	case "yellow":
		return c.Yellow
	case "red":
		return c.Red
	case "orange":
		return c.Orange
	case "cyan":
		return c.Cyan
	case "blue":
		return c.Blue
	case "violet":
		return c.Violet
	case "pink":
		return c.Pink
	case "light_text", "lighttext":
		return c.LightText
	case "muted_text", "mutedtext", "muted":
		return c.MutedText
	case "dark_text", "darktext":
		return c.DarkText
	case "border":
		return c.Border
	case "selected_background", "selectedbackground":
		return c.SelectedBackground
	case "subtle_background", "subtlebackground":
		return c.SubtleBackground
	case "very_subtle_background", "verysubtlebackground":
		return c.VerySubtleBackground
	default:
		return fallback
	}
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
	// Name is the resolved registry name the theme was built from.
	Name string

	Colors Colors

	// Headers and titles
	Header lipgloss.Style
	Title  lipgloss.Style

	// Status indicators (bold)
	Success lipgloss.Style
	Error   lipgloss.Style
	Warning lipgloss.Style
	Info    lipgloss.Style
	Magenta lipgloss.Style // Magenta/violet for special statuses like interrupted

	// Status indicators (non-bold) - for subtle/inline coloring
	SuccessLight lipgloss.Style
	ErrorLight   lipgloss.Style
	WarningLight lipgloss.Style
	InfoLight    lipgloss.Style

	// Text styles - visual hierarchy
	Bold        lipgloss.Style // Emphasized text
	Italic      lipgloss.Style // Italic text
	Normal      lipgloss.Style // Regular terminal default text
	Muted       lipgloss.Style // De-emphasized text
	Path        lipgloss.Style // File paths - muted italic
	Selected    lipgloss.Style
	SelectedRow lipgloss.Style

	// Selection styles
	SelectedUnfocused lipgloss.Style // Selected item in unfocused pane
	VisualSelection   lipgloss.Style // Visual selection mode highlight

	// Table styles
	TableHeader lipgloss.Style
	TableRow    lipgloss.Style
	TableBorder lipgloss.Style

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

	// Terminal chrome styles
	SidebarActive   lipgloss.Style // Active icon rail item
	SidebarInactive lipgloss.Style // Inactive icon rail item
	Separator       lipgloss.Style // Panel/drawer boundary separator

	// Dynamic color palette for components
	AccentColors []lipgloss.TerminalColor
}

// themeAliases maps legacy names onto registry entries. Aliases must never
// shadow a real palette name: gruvbox-dark/-light are actual palettes now,
// so aliasing them to the adaptive family would make the variant palettes
// unreachable (Lookup/SetTheme/resolveThemeColors all consult aliases before
// the registry).
var themeAliases = map[string]string{
	// kanagawa-wave and kanagawa-dragon are real palettes since the Phase 2b
	// upstream extraction; the old grove variant names map onto the upstream
	// variants that replaced them.
	"kanagawa-dark":  "kanagawa-wave",
	"kanagawa-light": "kanagawa-lotus",
	"branded":        "kanagawa",
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
	return &Theme{
		Name:   themeName,
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

		Magenta: lipgloss.NewStyle().
			Foreground(colors.Violet).
			Bold(true),

		// Non-bold variants for subtle/inline coloring
		SuccessLight: lipgloss.NewStyle().
			Foreground(colors.Green),

		ErrorLight: lipgloss.NewStyle().
			Foreground(colors.Red),

		WarningLight: lipgloss.NewStyle().
			Foreground(colors.Yellow),

		InfoLight: lipgloss.NewStyle().
			Foreground(colors.Cyan),

		// Text hierarchy: Bold → Normal → Muted
		Bold: lipgloss.NewStyle().
			Bold(true),

		Italic: lipgloss.NewStyle().
			Italic(true),

		Normal: lipgloss.NewStyle(),

		Muted: lipgloss.NewStyle().
			Faint(true),

		Path: lipgloss.NewStyle().
			Faint(true).
			Italic(true),

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

		SidebarActive: lipgloss.NewStyle().
			Background(colors.SubtleBackground).
			Foreground(colors.Orange).
			Bold(true),

		SidebarInactive: lipgloss.NewStyle().
			Background(colors.SubtleBackground).
			Foreground(colors.MutedText),

		Separator: lipgloss.NewStyle().
			Foreground(colors.MutedText),

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
	if builder, ok := themeRegistry[defaultThemeName]; ok {
		return builder()
	}
	// The embedded registry failed to load entirely; fall back to ANSI.
	return fallbackColors()
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
		// Config loading failed or returned nil - use default
		return defaultThemeName
	}

	// Check if TUI config exists and has a theme set
	if cfg.TUI != nil && cfg.TUI.Theme != "" {
		if theme := normalizeThemeName(cfg.TUI.Theme); theme != "" {
			return theme
		}
	}

	return defaultThemeName
}
