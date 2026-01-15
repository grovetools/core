package cli

import (
	"fmt"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattsolo1/grove-core/tui/theme"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// HelpExtrasFunc is a callback that renders additional help sections.
// It receives the theme for consistent styling.
type HelpExtrasFunc func(t *theme.Theme)

// helpExtras stores custom help section callbacks per command
var (
	helpExtras   = make(map[*cobra.Command]HelpExtrasFunc)
	helpExtrasMu sync.RWMutex
)

// SetStyledHelp applies consistent Grove styling to a command's help output
func SetStyledHelp(cmd *cobra.Command) {
	cmd.SetHelpFunc(styledHelpFunc)
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

	// Parse description and examples from Long
	var description, examples string
	if cmd.Long != "" {
		description, examples = parseDescription(cmd.Long)
	} else {
		description = cmd.Short
	}

	// Description
	if description != "" {
		fmt.Println(description)
	}

	// Usage section
	if cmd.Runnable() || cmd.HasSubCommands() {
		fmt.Println("\n " + t.Bold.Render("USAGE"))
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

		fmt.Println("\n " + t.Bold.Render("COMMANDS"))
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
			fmt.Println("\n " + t.Bold.Render("FLAGS"))
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
				usage := f.Usage
				if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "[]" {
					usage += t.Muted.Render(fmt.Sprintf(" (default: %s)", f.DefValue))
				}
				fmt.Printf(" %s%s  %s\n", magenta.Render(flagStr), padding, usage)
			}
		}
	}

	// Examples section (if present in Long description)
	if examples != "" {
		fmt.Println("\n " + t.Bold.Render("EXAMPLES"))
		renderExamples(t, examples, cmd.CommandPath())
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
