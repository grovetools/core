package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/mattsolo1/grove-core/pkg/tmux"
	"github.com/spf13/cobra"
)

func NewTmuxEditorCmd() *cobra.Command {
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
			vimCmd, _ := cmd.Flags().GetString("vim-cmd")

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
				if vimCmd != "" && strings.Contains(editorCmdStr, "vim") {
					// If vim-cmd is provided and editor is vim-based, add -c flag
					fullCommand += " -c " + "'" + strings.ReplaceAll(vimCmd, "'", `'\''`) + "'"
				}
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

			ctx := context.Background()

			// If vim-cmd is provided, handle it specially
			if vimCmd != "" {
				session, err := client.GetCurrentSession(ctx)
				if err != nil {
					return err
				}
				windowTarget := session + ":" + windowName

				// Check if window exists
				err = client.SelectWindow(ctx, windowTarget)
				if err == nil {
					// Window exists, check if vim is running
					currentCmd, err := client.GetPaneCommand(ctx, windowTarget)
					if err != nil {
						fmt.Fprintf(os.Stderr, "Failed to get pane command: %v\n", err)
					} else {
						fmt.Fprintf(os.Stderr, "Current command in editor window: %s\n", currentCmd)
					}

					editorRunning := err == nil && (strings.Contains(currentCmd, "nvim") || strings.Contains(currentCmd, "vim") || strings.Contains(currentCmd, "vi"))
					fmt.Fprintf(os.Stderr, "Editor running: %v\n", editorRunning)

					if editorRunning {
						// Editor is running, send the vim command
						fmt.Fprintf(os.Stderr, "Sending vim command: :%s\n", vimCmd)
						if err := client.SwitchClient(ctx, windowTarget); err != nil {
							fmt.Fprintf(os.Stderr, "Switch client error: %v\n", err)
						}
						// Send keys directly to the window target
						if err := client.SendKeys(ctx, windowTarget, ":"+vimCmd, "Enter"); err != nil {
							fmt.Fprintf(os.Stderr, "SendKeys error: %v\n", err)
							return err
						}
						if err := client.ClosePopup(ctx); err != nil {
							// Ignore popup close errors
						}
						return nil
					} else {
						// Window exists but vim isn't running - kill it so we can recreate
						fmt.Fprintf(os.Stderr, "Window exists but vim not running, killing window\n")
						if err := client.KillWindow(ctx, windowTarget); err != nil {
							fmt.Fprintf(os.Stderr, "Failed to kill window: %v\n", err)
						}
					}
				} else {
					fmt.Fprintf(os.Stderr, "Window doesn't exist: %v\n", err)
				}

				// Window doesn't exist or vim isn't running
				// For new editor windows, use --cmd approach which is simpler
				if editorCmdStr == "" {
					editorCmdStr = os.Getenv("EDITOR")
					if editorCmdStr == "" {
						editorCmdStr = "nvim"
					}
				}
				// Use the cmd flag to pass the vim command on startup
				editorCmdStr = "nvim -c '" + strings.ReplaceAll(vimCmd, "'", `'\''`) + "'"
				fmt.Fprintf(os.Stderr, "Starting new editor with command: %s\n", editorCmdStr)
			}

			// In a tmux session, use the new editor window management function.
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
	cmd.Flags().Int("window-index", -1, "Index (position) for the editor window. -1 means no positioning.")
	cmd.Flags().String("vim-cmd", "", "Vim command to execute. If editor is already running, sends as :command. Otherwise starts with -c flag.")

	return cmd
}
