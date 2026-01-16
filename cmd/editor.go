package cmd

import (
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/grovetools/core/pkg/tmux"
	"github.com/spf13/cobra"
)

func NewEditorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "editor [file]",
		Short: "Open a file or directory in the dedicated editor window",
		Long:  `Finds or creates a tmux window (default name "editor", index 1) and opens the specified file or current directory. By default, if the window exists, it is focused. New flags allow customizing the editor command, window name/index, and forcing a reset of the window.`,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := ""
			if len(args) > 0 {
				filePath = args[0]
			}

			// Get flag values
			editorCmdStr, _ := cmd.Flags().GetString("cmd")
			reset, _ := cmd.Flags().GetBool("reset")
			windowName, _ := cmd.Flags().GetString("window-name")
			windowIndex, _ := cmd.Flags().GetInt("window-index")

			// Determine editor command
			if editorCmdStr == "" {
				editorCmdStr = os.Getenv("EDITOR")
				if editorCmdStr == "" {
					editorCmdStr = "nvim" // Default to nvim
				}
			}

			client, err := tmux.NewClient()
			if err != nil {
				// Not in a tmux session, just open the editor normally.
				// Use sh -c to properly handle complex commands and arguments.
				fullCommand := editorCmdStr
				if filePath != "" {
					// Basic shell quoting for file path
					quotedPath := "'" + strings.ReplaceAll(filePath, "'", `'\''`) + "'"
					fullCommand += " " + quotedPath
				}

				editorCmd := exec.Command("sh", "-c", fullCommand)
				editorCmd.Stdin = os.Stdin
				editorCmd.Stdout = os.Stdout
				editorCmd.Stderr = os.Stderr
				return editorCmd.Run()
			}

			// In a tmux session, use the new editor window management function.
			ctx := context.Background()
			if err := client.OpenInEditorWindow(ctx, editorCmdStr, filePath, windowName, windowIndex, reset); err != nil {
				return err
			}

			// Close the popup this command was launched from.
			if err := client.ClosePopup(ctx); err != nil {
				// This might fail if not in a popup, which is acceptable.
			}

			return nil
		},
	}

	cmd.Flags().String("cmd", "", "Custom editor command to execute. The file path will be appended if provided. Defaults to $EDITOR or 'nvim'.")
	cmd.Flags().Bool("reset", false, "If the editor window exists, kill it and start a fresh session.")
	cmd.Flags().String("window-name", "editor", "Name of the target tmux window.")
	cmd.Flags().Int("window-index", 1, "Index (position) for the editor window.")

	return cmd
}
