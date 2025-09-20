package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewDocsCommand creates a standard 'docs' command that prints embedded JSON documentation.
func NewDocsCommand(docsJSON []byte) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "docs",
		Short: "Print the structured JSON documentation for this tool",
		Long:  `This command outputs the structured documentation for this tool in JSON format, which is used by other ecosystem tools like grove-mcp.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println(string(docsJSON))
			return nil
		},
	}
	// The --json flag is implied since that's all this command does.
	return cmd
}