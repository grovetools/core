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
	rootCmd.AddCommand(cmd.NewConfigCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
