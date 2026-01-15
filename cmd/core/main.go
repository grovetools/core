package main

import (
	"os"

	"github.com/mattsolo1/grove-core/cli"
	"github.com/mattsolo1/grove-core/cmd"
)

func main() {
	rootCmd := cli.NewStandardCommand(
		"core",
		"Core libraries and debugging tools for the Grove ecosystem",
	)

	// Add subcommands
	rootCmd.AddCommand(cmd.NewVersionCmd())
	rootCmd.AddCommand(cmd.NewWsCmd())
	rootCmd.AddCommand(cmd.NewWorktreesCmd())
	rootCmd.AddCommand(cmd.NewConfigCmd())
	rootCmd.AddCommand(cmd.NewEditorCmd())
	rootCmd.AddCommand(cmd.NewOpenInWindowCmd())
	rootCmd.AddCommand(cmd.NewTmuxCmd())
	rootCmd.AddCommand(cmd.NewLogsCmd())
	rootCmd.AddCommand(cmd.NewNvimDemoCmd())

	if err := cli.Execute(rootCmd); err != nil {
		os.Exit(1)
	}
}
