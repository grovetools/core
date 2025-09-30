package theme

import (
	"github.com/charmbracelet/lipgloss"
)

// Nature-inspired color palette for Grove
var (
	// Primary colors - Earthy and nature-themed
	Green  = lipgloss.Color("#A3BE8C") // Success, Up-to-date
	Yellow = lipgloss.Color("#EBCB8B") // Warning, Pending
	Red    = lipgloss.Color("#BF616A") // Error, Failed
	Orange = lipgloss.Color("#D08770") // Highlight, Accent
	Cyan   = lipgloss.Color("#88C0D0") // Info, Links
	Brown  = lipgloss.Color("#8B7355") // Earth tone

	// Text colors
	LightText = lipgloss.Color("#ECEFF4") // Primary text
	MutedText = lipgloss.Color("#6272A4") // Faint text, help
	DarkText  = lipgloss.Color("#2E3440") // Dark text for light backgrounds

	// Background colors
	Border             = lipgloss.Color("#4C566A")
	SelectedBackground = lipgloss.Color("#434C5E")
	SubtleBackground   = lipgloss.Color("#3B4252")
	VerySubtleBackground = lipgloss.Color("#0d0d0d") // Barely visible, just a hint above pure black
)

// Theme holds all the pre-configured styles for the Grove ecosystem
type Theme struct {
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
		// Headers and titles
		Header: lipgloss.NewStyle().
			Foreground(Green).
			Bold(true).
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
			Foreground(Brown).
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