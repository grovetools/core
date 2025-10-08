package starship

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mattsolo1/grove-core/state"
	"github.com/spf13/cobra"
)

// NewStarshipCmd creates the starship command and its subcommands.
// The binaryName parameter is used to configure the command in starship.toml
// (e.g., "flow" will generate "command = \"flow starship status\"").
func NewStarshipCmd(binaryName string) *cobra.Command {
	starshipCmd := &cobra.Command{
		Use:   "starship",
		Short: "Manage Starship prompt integration",
		Long:  `Provides commands to integrate Grove status with the Starship prompt.`,
	}

	installCmd := &cobra.Command{
		Use:   "install",
		Short: "Install the Grove module to your starship.toml",
		Long: `Appends a custom module to your starship.toml configuration file to display
Grove status in your shell prompt. It will also attempt to add the module to
your main prompt format.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStarshipInstall(binaryName)
		},
	}

	statusCmd := &cobra.Command{
		Use:    "status",
		Short:  "Print status for Starship prompt (for internal use)",
		Hidden: true,
		RunE:   runStarshipStatus,
	}

	starshipCmd.AddCommand(installCmd)
	starshipCmd.AddCommand(statusCmd)

	return starshipCmd
}

func runStarshipInstall(binaryName string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not get home directory: %w", err)
	}

	configPath := filepath.Join(home, ".config", "starship.toml")

	contentBytes, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("starship config not found at %s. Please ensure starship is installed and configured", configPath)
		}
		return fmt.Errorf("could not read starship config: %w", err)
	}
	content := string(contentBytes)

	// --- 1. Add or update the custom module definition ---
	moduleConfig := fmt.Sprintf(`
# Added by '%s starship install'
[custom.grove]
description = "Shows Grove status"
command = "%s starship status"
when = "test -f .grove/state.yml || test -f grove.yml"
format = " $output "
`, binaryName, binaryName)

	// Check if [custom.grove] already exists
	if strings.Contains(content, "[custom.grove]") {
		// If it exists, check if the command matches
		if !strings.Contains(content, fmt.Sprintf(`command = "%s starship status"`, binaryName)) {
			// Different command exists - don't overwrite to avoid conflicts between Grove tools
			fmt.Printf("ℹ️  [custom.grove] already exists with a different command.\n")
			fmt.Printf("   Keeping existing configuration to avoid conflicts.\n")
		} else {
			// Same command - replace the entire section
			startIdx := strings.Index(content, "[custom.grove]")
			if startIdx != -1 {
				afterGrove := content[startIdx:]
				nextSectionIdx := strings.Index(afterGrove[1:], "\n[")

				var endIdx int
				if nextSectionIdx != -1 {
					endIdx = startIdx + nextSectionIdx + 1
				} else {
					endIdx = len(content)
				}

				content = content[:startIdx] + moduleConfig + content[endIdx:]
				fmt.Println("✓ Updated existing Grove starship module configuration.")
			}
		}
	} else {
		content += moduleConfig
		fmt.Println("✓ Added [custom.grove] module to starship config.")
	}

	// --- 2. Add the module to the prompt format if not already present ---
	if strings.Contains(content, "${custom.grove}") || strings.Contains(content, "$custom.grove") {
		fmt.Println("✓ Grove module already in starship format.")
	} else {
		// Try to insert it after git_metrics, which is a common element.
		target := "$git_metrics\\"
		if strings.Contains(content, target) {
			replacement := target + "\n${custom.grove}\\"
			content = strings.Replace(content, target, replacement, 1)
			fmt.Println("✓ Added Grove module to starship format.")
		} else {
			fmt.Printf("⚠️  Could not automatically add '${custom.grove}' to your starship format.\n")
			fmt.Printf("   Please add it manually to the 'format' string in %s\n", configPath)
		}
	}

	// --- 3. Write the updated config back ---
	err = os.WriteFile(configPath, []byte(content), 0o644)
	if err != nil {
		return fmt.Errorf("failed to write updated starship config: %w", err)
	}

	fmt.Printf("\nSuccessfully updated %s. Please restart your shell to see the changes.\n", configPath)
	return nil
}

func runStarshipStatus(cmd *cobra.Command, args []string) error {
	// This command must be fast and should not print errors to stderr.
	// Load the state
	currentState, err := state.Load()
	if err != nil {
		// Silently fail if we can't load state
		return nil
	}

	// Call all registered providers and collect their output
	var outputs []string
	for _, provider := range providers {
		output, err := provider(currentState)
		if err != nil {
			// Silently ignore provider errors
			continue
		}
		if output != "" {
			outputs = append(outputs, output)
		}
	}

	// Print all non-empty outputs, joined by separator
	if len(outputs) > 0 {
		fmt.Print(strings.Join(outputs, " | "))
	}

	return nil
}
