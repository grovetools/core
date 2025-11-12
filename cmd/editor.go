package cmd

import (
	"context"
	"os"
	"os/exec"

	"github.com/mattsolo1/grove-core/pkg/tmux"
	"github.com/spf13/cobra"
)

func NewEditorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "editor [file]",
		Short: "Open a file or directory in the dedicated editor window",
		Long:  `Finds or creates a tmux window named "editor" at index 1 and opens the specified file or current directory. This command is intended to be run from within a tmux popup.`,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := "."
			if len(args) > 0 {
				filePath = args[0]
			}

			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = "nvim" // Default to nvim
			}

			client, err := tmux.NewClient()
			if err != nil {
				// Not in a tmux session, just open the editor normally.
				editorCmd := exec.Command(editor, filePath)
				editorCmd.Stdin = os.Stdin
				editorCmd.Stdout = os.Stdout
				editorCmd.Stderr = os.Stderr
				return editorCmd.Run()
			}

			// Open the file in the "editor" window at index 1.
			ctx := context.Background()
			if err := client.OpenFileInEditor(ctx, editor, filePath, "editor", 1); err != nil {
				return err
			}

			// Close the popup this command was launched from.
			if err := client.ClosePopup(ctx); err != nil {
				// This might fail if not in a popup, which is acceptable.
			}

			return nil
		},
	}
	return cmd
}
