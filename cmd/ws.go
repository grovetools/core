package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattsolo1/grove-core/cli"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-core/tui/wsnav"
	"github.com/spf13/cobra"
)

// NewWsCmd creates the `ws` command
func NewWsCmd() *cobra.Command {
	cmd := cli.NewStandardCommand(
		"ws",
		"Navigate and explore Grove workspaces",
	)
	cmd.Long = `This command launches an interactive TUI to navigate and explore all workspaces
discovered by Grove based on your configuration. It provides a hierarchical view
of ecosystems, projects, and worktrees.`

	cmd.Flags().Bool("json", false, "Output discovered workspaces in JSON format")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		logger := cli.GetLogger(cmd)

		// Discover all workspaces
		discoveryService := workspace.NewDiscoveryService(logger)
		discoveryResult, err := discoveryService.DiscoverAll()
		if err != nil {
			return fmt.Errorf("failed to discover workspaces: %w", err)
		}

		// Transform into a flat list of enriched project info
		projects := workspace.TransformToProjectInfo(discoveryResult)

		// Handle JSON output
		jsonOutput, _ := cmd.Flags().GetBool("json")
		if jsonOutput {
			jsonData, err := json.MarshalIndent(projects, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal projects to JSON: %w", err)
			}
			fmt.Println(string(jsonData))
			return nil
		}

		// Launch the TUI
		p := tea.NewProgram(wsnav.New(projects), tea.WithAltScreen(), tea.WithMouseCellMotion())
		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
			return err
		}

		return nil
	}

	return cmd
}
