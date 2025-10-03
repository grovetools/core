package tui

import (
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// InitializeTUI prepares the terminal environment for TUI applications.
// It checks for environment variables that force color output (`CLICOLOR_FORCE`,
// `COLORTERM`) and sets the appropriate lipgloss color profile when present.
//
// This ensures consistent color and styling when running TUIs in non-interactive
// or CI environments (e.g., when testing with 'tend'), while having no effect
// in production environments where these variables are not set.
//
// It is recommended to call this function at the start of your TUI application's
// main function.
func InitializeTUI() {
	if os.Getenv("CLICOLOR_FORCE") == "1" || os.Getenv("COLORTERM") == "truecolor" {
		lipgloss.SetColorProfile(termenv.TrueColor)
	}
}
