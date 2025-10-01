package theme

import (
	"github.com/charmbracelet/lipgloss"
)

// Kanagawa Dragon
var (
	// Primary colors
	Green  = lipgloss.Color("#98BB6C") // springGreen: Success
	Yellow = lipgloss.Color("#FF9E3B") // roninYellow: Warning
	Red    = lipgloss.Color("#FF5D62") // peachRed: Error
	Orange = lipgloss.Color("#FFA066") // surimiOrange: Highlight
	Cyan   = lipgloss.Color("#7E9CD8") // crystalBlue: Info, Links
	Blue   = lipgloss.Color("#7FB4CA") // springBlue
	Violet = lipgloss.Color("#957FB8") // oniViolet: Accent
	Pink   = lipgloss.Color("#D27E99") // sakuraPink

	// Text colors
	LightText = lipgloss.Color("#DCD7BA") // fujiWhite: Primary text
	MutedText = lipgloss.Color("#727169") // fujiGray: Faint text, help
	DarkText  = lipgloss.Color("#1D1C19") // dragonBlack2

	// Background colors
	Border               = lipgloss.Color("#363646") // sumiInk5
	SelectedBackground   = lipgloss.Color("#223249") // waveBlue1
	SubtleBackground     = lipgloss.Color("#1F1F28") // sumiInk3
	VerySubtleBackground = lipgloss.Color("#181820") // sumiInk1
)

// Colors struct holds all the color constants for easy access
type Colors struct {
	Green                lipgloss.Color
	Yellow               lipgloss.Color
	Red                  lipgloss.Color
	Orange               lipgloss.Color
	Cyan                 lipgloss.Color
	Blue                 lipgloss.Color
	Violet               lipgloss.Color
	Pink                 lipgloss.Color
	LightText            lipgloss.Color
	MutedText            lipgloss.Color
	DarkText             lipgloss.Color
	Border               lipgloss.Color
	SelectedBackground   lipgloss.Color
	SubtleBackground     lipgloss.Color
	VerySubtleBackground lipgloss.Color
}

// DefaultColors provides easy access to all theme colors
var DefaultColors = Colors{
	Green:                Green,
	Yellow:               Yellow,
	Red:                  Red,
	Orange:               Orange,
	Cyan:                 Cyan,
	Blue:                 Blue,
	Violet:               Violet,
	Pink:                 Pink,
	LightText:            LightText,
	MutedText:            MutedText,
	DarkText:             DarkText,
	Border:               Border,
	SelectedBackground:   SelectedBackground,
	SubtleBackground:     SubtleBackground,
	VerySubtleBackground: VerySubtleBackground,
}

// Theme holds all the pre-configured styles for the Grove ecosystem
type Theme struct {
	// Colors provides access to the color palette
	Colors Colors

	// Headers and titles
	Header lipgloss.Style
	Title  lipgloss.Style

	// Status indicators
	Success lipgloss.Style
	Error   lipgloss.Style
	Warning lipgloss.Style
	Info    lipgloss.Style

	// Text styles
	Muted    lipgloss.Style
	Selected lipgloss.Style
	Bold     lipgloss.Style
	Faint    lipgloss.Style

	// Table styles
	TableHeader lipgloss.Style
	TableRow    lipgloss.Style
	TableBorder lipgloss.Style

	// Container styles
	Box  lipgloss.Style
	Code lipgloss.Style

	// Interactive elements
	Input       lipgloss.Style
	Placeholder lipgloss.Style
	Cursor      lipgloss.Style

	// Special styles
	Highlight lipgloss.Style
	Accent    lipgloss.Style
}

// NewTheme creates a new Theme with the default Grove styling
func NewTheme() *Theme {
	return &Theme{
		// Colors
		Colors: DefaultColors,

		// Headers and titles
		Header: lipgloss.NewStyle().
			Foreground(Green).
			Bold(true).
			MarginTop(1).
			MarginBottom(1),

		Title: lipgloss.NewStyle().
			Foreground(Green).
			Bold(true).
			Underline(true).
			MarginBottom(1),

		// Status indicators
		Success: lipgloss.NewStyle().
			Foreground(Green).
			Bold(true),

		Error: lipgloss.NewStyle().
			Foreground(Red).
			Bold(true),

		Warning: lipgloss.NewStyle().
			Foreground(Yellow).
			Bold(true),

		Info: lipgloss.NewStyle().
			Foreground(Cyan).
			Bold(true),

		// Text styles
		Muted: lipgloss.NewStyle().
			Foreground(MutedText),

		Selected: lipgloss.NewStyle().
			Background(SelectedBackground).
			Foreground(LightText),

		Bold: lipgloss.NewStyle().
			Bold(true).
			Foreground(LightText),

		Faint: lipgloss.NewStyle().
			Faint(true).
			Foreground(MutedText),

		// Table styles
		TableHeader: lipgloss.NewStyle().
			Bold(true).
			Foreground(Green).
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true).
			BorderForeground(Border),

		TableRow: lipgloss.NewStyle().
			Foreground(LightText),

		TableBorder: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(Border),

		// Container styles
		Box: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Border).
			Padding(1, 2).
			Margin(1, 0),

		Code: lipgloss.NewStyle().
			Background(SubtleBackground).
			Foreground(LightText).
			Padding(0, 1).
			MarginLeft(2),

		// Interactive elements
		Input: lipgloss.NewStyle().
			Foreground(LightText),

		Placeholder: lipgloss.NewStyle().
			Foreground(MutedText).
			Italic(true),

		Cursor: lipgloss.NewStyle().
			Foreground(Orange).
			Bold(true),

		// Special styles
		Highlight: lipgloss.NewStyle().
			Foreground(Orange).
			Bold(true),

		Accent: lipgloss.NewStyle().
			Foreground(Violet).
			Bold(true),
	}
}

// DefaultTheme is the default theme instance for the Grove ecosystem
var DefaultTheme = NewTheme()

// Icons used across the Grove ecosystem
const (
	// Status icons
	IconPending = "⏳"
	IconRunning = "🔄"
	IconSuccess = "✅"
	IconError   = "❌"
	IconWarning = "⚠️"
	IconInfo    = "ℹ️"

	// Navigation icons
	IconArrow  = "→"
	IconBullet = "•"
	IconTree   = "🌲" // Grove theme!
	IconLeaf   = "🍃"
	IconBranch = "🌿"

	// Action icons
	IconSearch = "🔍"
	IconHelp   = "❓"
	IconBack   = "←"
)

// Render helpers for common patterns

// RenderHeader renders a header with the default Grove styling
func RenderHeader(title string) string {
	return DefaultTheme.Header.Render(title)
}

// RenderTitle renders a title with the default Grove styling
func RenderTitle(title string) string {
	return DefaultTheme.Title.Render(title)
}

// RenderStatus renders text with the appropriate status style
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

// RenderBox renders content inside a styled box
func RenderBox(content string) string {
	return DefaultTheme.Box.Render(content)
}

