package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/grovetools/core/cli"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/core/tui/wsnav"
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

		// Discover all workspaces using the centralized function
		projects, err := workspace.GetProjects(logger)
		if err != nil {
			return fmt.Errorf("failed to discover workspaces: %w", err)
		}

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

		// Launch the TUI with 30 second refresh interval
		p := tea.NewProgram(wsnav.New(projects, 30), tea.WithAltScreen(), tea.WithMouseCellMotion())
		finalModel, err := p.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
			return err
		}

		// If a project was selected, print its path to stdout.
		if m, ok := finalModel.(*wsnav.Model); ok && m.SelectedProject != nil {
			fmt.Println(m.SelectedProject.Path)
		}

		return nil
	}

	// Add subcommand for getting current workspace
	cmd.AddCommand(newWsCwdCmd())

	return cmd
}

// newWsCwdCmd creates the `ws cwd` subcommand
func newWsCwdCmd() *cobra.Command {
	cmd := cli.NewStandardCommand(
		"cwd",
		"Get workspace information for current working directory",
	)
	cmd.Long = `Get the workspace information for the current working directory.
This command uses GetProjectByPath to find the workspace containing the current directory.`

	cmd.Flags().Bool("json", false, "Output workspace in JSON format")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		// Get current working directory
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}

		// Get the workspace for this path
		node, err := workspace.GetProjectByPath(cwd)
		if err != nil {
			return fmt.Errorf("failed to get workspace: %w", err)
		}

		// Handle JSON output
		jsonOutput, _ := cmd.Flags().GetBool("json")
		if jsonOutput {
			jsonData, err := json.MarshalIndent(node, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal workspace to JSON: %w", err)
			}
			fmt.Println(string(jsonData))
			return nil
		}

		// Pretty print output
		fmt.Printf("Name: %s\n", node.Name)
		fmt.Printf("Path: %s\n", node.Path)
		fmt.Printf("Kind: %s\n", node.Kind)
		if node.ParentProjectPath != "" {
			fmt.Printf("Parent Project: %s\n", node.ParentProjectPath)
		}
		if node.ParentEcosystemPath != "" {
			fmt.Printf("Parent Ecosystem: %s\n", node.ParentEcosystemPath)
		}
		if node.RootEcosystemPath != "" {
			fmt.Printf("Root Ecosystem: %s\n", node.RootEcosystemPath)
		}

		return nil
	}

	return cmd
}
