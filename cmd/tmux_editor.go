package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/grovetools/core/pkg/mux"
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

			ctx := context.Background()
			engine, err := mux.DetectMuxEngine(ctx)
			if err != nil {
				// Not in a mux session, just open the editor normally.
				fullCommand := editorCmdStr
				if vimCmd != "" && strings.Contains(editorCmdStr, "vim") {
					fullCommand += " -c " + "'" + strings.ReplaceAll(vimCmd, "'", `'\''`) + "'"
				}
				if filePath != "" {
					quotedPath := "'" + strings.ReplaceAll(filePath, "'", `'\''`) + "'"
					fullCommand += " " + quotedPath
				}

				editorCmd := exec.Command("sh", "-c", fullCommand)
				editorCmd.Stdin = os.Stdin
				editorCmd.Stdout = os.Stdout
				editorCmd.Stderr = os.Stderr
				return editorCmd.Run()
			}

			tuiEngine, ok := engine.(mux.MuxTUIEngine)
			if !ok {
				return fmt.Errorf("mux engine does not support TUI operations")
			}

			// If vim-cmd is provided, handle it specially
			if vimCmd != "" {
				session, err := engine.GetCurrentSession(ctx)
				if err != nil {
					return err
				}
				windowTarget := session + ":" + windowName

				// Check if window exists
				err = engine.SelectWindow(ctx, windowTarget)
				if err == nil {
					// Window exists, check if vim is running
					currentCmd, err := engine.GetPaneCommand(ctx, windowTarget)
					if err != nil {
						fmt.Fprintf(os.Stderr, "Failed to get pane command: %v\n", err)
					} else {
						fmt.Fprintf(os.Stderr, "Current command in editor window: %s\n", currentCmd)
					}

					editorRunning := err == nil && (strings.Contains(currentCmd, "nvim") || strings.Contains(currentCmd, "vim") || strings.Contains(currentCmd, "vi"))
					fmt.Fprintf(os.Stderr, "Editor running: %v\n", editorRunning)

					if editorRunning {
						fmt.Fprintf(os.Stderr, "Sending vim command: :%s\n", vimCmd)
						if err := engine.SwitchSession(ctx, windowTarget, ""); err != nil {
							fmt.Fprintf(os.Stderr, "Switch client error: %v\n", err)
						}
						if err := engine.SendKeys(ctx, windowTarget, ":"+vimCmd, "Enter"); err != nil {
							fmt.Fprintf(os.Stderr, "SendKeys error: %v\n", err)
							return err
						}
						_ = tuiEngine.ClosePopup(ctx)
						return nil
					} else {
						fmt.Fprintf(os.Stderr, "Window exists but vim not running, killing window\n")
						if err := engine.KillWindow(ctx, windowTarget); err != nil {
							fmt.Fprintf(os.Stderr, "Failed to kill window: %v\n", err)
						}
					}
				} else {
					fmt.Fprintf(os.Stderr, "Window doesn't exist: %v\n", err)
				}

				// Window doesn't exist or vim isn't running
				editorCmdStr = "nvim -c '" + strings.ReplaceAll(vimCmd, "'", `'\''`) + "'"
				fmt.Fprintf(os.Stderr, "Starting new editor with command: %s\n", editorCmdStr)
			}

			if err := tuiEngine.OpenInEditorWindow(ctx, editorCmdStr, filePath, windowName, windowIndex, reset); err != nil {
				return err
			}

			_ = tuiEngine.ClosePopup(ctx)

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
