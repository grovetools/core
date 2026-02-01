package cli

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/grovetools/core/tui/theme"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/term"
)

// HelpExtrasFunc is a callback that renders additional help sections.
// It receives the theme for consistent styling.
type HelpExtrasFunc func(t *theme.Theme)

// helpExtras stores custom help section callbacks per command
var (
	helpExtras   = make(map[*cobra.Command]HelpExtrasFunc)
	helpExtrasMu sync.RWMutex
)

const maxWidth = 60
const minWidth = 40

// getTerminalWidth returns the terminal width capped at maxWidth.
func getTerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width < minWidth {
		return maxWidth
	}
	if width > maxWidth {
		return maxWidth
	}
	return width
}

// wrapText wraps text to the specified width, preserving existing line breaks.
func wrapText(text string, width int) string {
	if width <= 0 {
		width = maxWidth
	}

	var result []string
	for _, paragraph := range strings.Split(text, "\n") {
		if len(paragraph) <= width {
			result = append(result, paragraph)
			continue
		}

		// Wrap long lines
		var line string
		for _, word := range strings.Fields(paragraph) {
			if line == "" {
				line = word
			} else if len(line)+1+len(word) <= width {
				line += " " + word
			} else {
				result = append(result, line)
				line = word
			}
		}
		if line != "" {
			result = append(result, line)
		}
	}
	return strings.Join(result, "\n")
}

// SetStyledHelp applies consistent Grove styling to a command's help output.
// Call this on the root command before Execute().
func SetStyledHelp(cmd *cobra.Command) {
	cmd.SetHelpFunc(styledHelpFunc)
}

// ApplyStyledHelpRecursive applies styled help and usage to a command and all its subcommands.
// Call this after all subcommands have been added, before Execute().
func ApplyStyledHelpRecursive(cmd *cobra.Command) {
	cmd.SetHelpFunc(styledHelpFunc)
	cmd.SetUsageFunc(styledUsageFunc)
	for _, sub := range cmd.Commands() {
		ApplyStyledHelpRecursive(sub)
	}
}

// styledUsageFunc provides minimal usage output (shown on errors).
// Returns nothing since error handling is done in cli.Execute.
func styledUsageFunc(cmd *cobra.Command) error {
	return nil
}

// PrintError prints a styled error message to stderr with help hint.
func PrintError(cmd *cobra.Command, err error) {
	t := theme.DefaultTheme
	red := lipgloss.NewStyle().Bold(true).Foreground(t.Colors.Red)
	fmt.Fprintf(cmd.ErrOrStderr(), "%s %s\n", red.Render("Error:"), err.Error())
	fmt.Fprintf(cmd.ErrOrStderr(), "%s\n", t.Muted.Render(fmt.Sprintf("Run '%s --help' for usage.", cmd.CommandPath())))
}

// SetStyledHelpWithExtras applies Grove styling with additional custom sections.
// The extras function is called after COMMANDS but before the help hint.
func SetStyledHelpWithExtras(cmd *cobra.Command, extras HelpExtrasFunc) {
	helpExtrasMu.Lock()
	helpExtras[cmd] = extras
	helpExtrasMu.Unlock()
	cmd.SetHelpFunc(styledHelpFunc)
}

// parseDescription splits a command's long description into main text and examples.
func parseDescription(long string) (description string, examples string) {
	// Look for "Examples:" or "Example:" section
	markers := []string{"\nExamples:\n", "\nExample:\n", "\nEXAMPLES:\n", "\nEXAMPLE:\n"}
	for _, marker := range markers {
		if idx := strings.Index(long, marker); idx != -1 {
			return strings.TrimSpace(long[:idx]), strings.TrimSpace(long[idx+len(marker):])
		}
	}
	return long, ""
}

// renderExamples styles example lines with muted comments and styled commands.
func renderExamples(t *theme.Theme, examples string, cmdPath string) {
	cyan := lipgloss.NewStyle().Foreground(t.Colors.Cyan)
	blue := lipgloss.NewStyle().Foreground(t.Colors.Blue)
	magenta := lipgloss.NewStyle().Foreground(t.Colors.Violet)

	// Extract root command name from command path (e.g., "core logs" -> "core")
	rootCmd := strings.Split(cmdPath, " ")[0]

	lines := strings.Split(examples, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			fmt.Println()
			continue
		}
		// Style comment lines as muted
		if strings.HasPrefix(trimmed, "#") {
			fmt.Println(" " + t.Muted.Render(trimmed))
		} else {
			// Parse and style command lines
			fmt.Println(" " + styleCommandLine(trimmed, rootCmd, cyan, blue, magenta))
		}
	}
}

// styleCommandLine applies styling to different parts of a command example.
func styleCommandLine(line, rootCmd string, mainStyle, subStyle, flagStyle lipgloss.Style) string {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return line
	}

	var result []string
	for i, part := range parts {
		switch {
		case i == 0 && part == rootCmd:
			// Main command - blue bold
			result = append(result, mainStyle.Render(part))
		case i == 1 && !strings.HasPrefix(part, "-"):
			// Subcommand (second word, not a flag) - cyan
			result = append(result, subStyle.Render(part))
		case strings.HasPrefix(part, "-"):
			// Flags - muted
			result = append(result, flagStyle.Render(part))
		default:
			// Arguments and other text - normal
			result = append(result, part)
		}
	}
	return "  " + strings.Join(result, " ")
}

func styledHelpFunc(cmd *cobra.Command, args []string) {
	t := theme.DefaultTheme
	blue := lipgloss.NewStyle().Bold(true).Foreground(t.Colors.Blue)
	section := lipgloss.NewStyle().Italic(true).Foreground(t.Colors.Orange)

	// Get terminal width for wrapping (subtract 2 for indent)
	width := getTerminalWidth() - 2

	// Title - uppercase command path in orange
	title := lipgloss.NewStyle().Bold(true).Foreground(t.Colors.Orange)
	fmt.Println(" " + title.Render(strings.ToUpper(cmd.CommandPath())))

	// Parse description and examples from Long
	var description, examples string
	if cmd.Long != "" {
		description, examples = parseDescription(cmd.Long)
	} else {
		description = cmd.Short
	}

	// Short description in italic, then expanded description below
	if cmd.Short != "" {
		for _, line := range strings.Split(wrapText(cmd.Short, width), "\n") {
			fmt.Println(" " + t.Italic.Render(line))
		}
	}
	if description != "" && description != cmd.Short {
		fmt.Println()
		// Wrap and indent each line of the description
		wrapped := wrapText(description, width)
		for _, line := range strings.Split(wrapped, "\n") {
			fmt.Println(" " + line)
		}
	}

	// Usage section
	if cmd.Runnable() || cmd.HasSubCommands() {
		fmt.Println("\n " + section.Render("USAGE"))
		if cmd.Runnable() {
			fmt.Printf(" %s\n", cmd.UseLine())
		}
		if cmd.HasSubCommands() {
			fmt.Printf(" %s [command]\n", cmd.CommandPath())
		}
	}

	// Commands section
	if cmd.HasAvailableSubCommands() {
		maxLen := 0
		for _, sub := range cmd.Commands() {
			if sub.IsAvailableCommand() && len(sub.Name()) > maxLen {
				maxLen = len(sub.Name())
			}
		}

		fmt.Println("\n " + section.Render("COMMANDS"))
		for _, sub := range cmd.Commands() {
			if sub.IsAvailableCommand() {
				padding := strings.Repeat(" ", maxLen-len(sub.Name()))
				fmt.Printf(" %s%s  %s\n", blue.Render(sub.Name()), padding, sub.Short)
			}
		}
	}

	// Flags section - show detailed flags for leaf commands, inline for parent commands
	var visibleFlags []*pflag.Flag
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if !f.Hidden {
			visibleFlags = append(visibleFlags, f)
		}
	})

	if len(visibleFlags) > 0 {
		if cmd.HasAvailableSubCommands() {
			// Parent commands: show inline flags (compact)
			var flags []string
			for _, f := range visibleFlags {
				if f.Shorthand != "" {
					flags = append(flags, fmt.Sprintf("-%s/--%s", f.Shorthand, f.Name))
				} else {
					flags = append(flags, fmt.Sprintf("--%s", f.Name))
				}
			}
			fmt.Println("\n " + t.Muted.Render("Flags: "+strings.Join(flags, ", ")))
		} else {
			// Leaf commands: show detailed flags
			magenta := lipgloss.NewStyle().Foreground(t.Colors.Violet)
			fmt.Println("\n " + section.Render("FLAGS"))
			maxFlagLen := 0
			for _, f := range visibleFlags {
				flagStr := formatFlagName(f)
				if len(flagStr) > maxFlagLen {
					maxFlagLen = len(flagStr)
				}
			}
			for _, f := range visibleFlags {
				flagStr := formatFlagName(f)
				padding := strings.Repeat(" ", maxFlagLen-len(flagStr))
				indent := strings.Repeat(" ", maxFlagLen+3) // for continuation lines

				usage, choices := parseChoices(f.Usage)
				if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "[]" {
					usage += t.Muted.Render(fmt.Sprintf(" (default: %s)", f.DefValue))
				}
				fmt.Printf(" %s%s  %s\n", magenta.Render(flagStr), padding, usage)

				// Print choices on separate lines if present
				for _, choice := range choices {
					fmt.Printf(" %s  %s\n", indent, t.Muted.Render("• "+choice))
				}
			}
		}
	}

	// Examples section (from cmd.Example field or parsed from Long description)
	exampleText := cmd.Example
	if exampleText == "" {
		exampleText = examples
	}
	if exampleText != "" {
		fmt.Println("\n " + section.Render("EXAMPLES"))
		renderExamples(t, exampleText, cmd.CommandPath())
	}

	// Custom extras section (if registered)
	helpExtrasMu.RLock()
	extras := helpExtras[cmd]
	helpExtrasMu.RUnlock()
	if extras != nil {
		extras(t)
	}

	// Help hint
	if cmd.HasSubCommands() {
		fmt.Printf("\n Use \"%s [command] --help\" for more information.\n", cmd.CommandPath())
	}
}

// formatFlagName returns a formatted flag string like "-f, --flag" or "--flag".
func formatFlagName(f *pflag.Flag) string {
	if f.Shorthand != "" {
		return fmt.Sprintf("-%s, --%s", f.Shorthand, f.Name)
	}
	return fmt.Sprintf("    --%s", f.Name)
}

// parseChoices extracts choices from a flag usage string.
// Supports two formats:
// 1. Inline: "type: x, y, z" - comma-separated choices on one line
// 2. Multi-line with descriptions:
//
//	"Description:
//	   • choice1 - Description of choice1
//	   • choice2 - Description of choice2"
//
// Returns the base description and the list of choices (with descriptions if present).
func parseChoices(usage string) (description string, choices []string) {
	// First check for multi-line bullet point format
	lines := strings.Split(usage, "\n")
	if len(lines) > 1 {
		var baseDesc string
		var bulletChoices []string

		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			// Check for bullet point lines (• or -)
			if strings.HasPrefix(trimmed, "• ") || strings.HasPrefix(trimmed, "- ") {
				// Extract the choice (including description after dash if present)
				choice := strings.TrimPrefix(trimmed, "• ")
				choice = strings.TrimPrefix(choice, "- ")
				bulletChoices = append(bulletChoices, choice)
			} else if i == 0 && trimmed != "" {
				// First non-bullet line is the description
				baseDesc = trimmed
			}
		}

		if len(bulletChoices) > 0 {
			return baseDesc, bulletChoices
		}
	}

	// Fall back to inline comma-separated format
	// Look for patterns like ": x, y, z" or ": x, y, or z"
	colonIdx := strings.Index(usage, ": ")
	if colonIdx == -1 {
		return usage, nil
	}

	// Check if what follows looks like a list of choices (contains commas)
	afterColon := usage[colonIdx+2:]

	// Find where the choices end (at a parenthesis or end of string)
	endIdx := strings.Index(afterColon, " (")
	var choicesStr string
	var suffix string
	if endIdx != -1 {
		choicesStr = afterColon[:endIdx]
		suffix = afterColon[endIdx:]
	} else {
		choicesStr = afterColon
	}

	// Must have at least one comma to be considered a choice list
	if !strings.Contains(choicesStr, ", ") {
		return usage, nil
	}

	// Split by comma, handling "or" in the last item
	parts := strings.Split(choicesStr, ", ")
	if len(parts) < 3 {
		// Not enough choices to warrant splitting
		return usage, nil
	}

	// Clean up choices (handle "or x" in last item)
	for i, p := range parts {
		p = strings.TrimPrefix(p, "or ")
		parts[i] = strings.TrimSpace(p)
	}

	// Return base description (up to and including colon) + suffix
	baseDesc := usage[:colonIdx+1]
	if suffix != "" {
		baseDesc += suffix
	}

	return baseDesc, parts
}
