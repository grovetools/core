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

// NewOpenInWindowCmd creates the `open-in-window` command.
func NewOpenInWindowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "open-in-window -- [command...]",
		Short: "Runs a command in a dedicated, focused tmux window",
		Long:  `Finds or creates a tmux window with a given name, kills any existing process in it, runs the specified command, and focuses the window. If not in a tmux session, it runs the command directly.`,
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			windowName, _ := cmd.Flags().GetString("name")
			windowIndex, _ := cmd.Flags().GetInt("index")

			if windowName == "" {
				return fmt.Errorf("--name flag is required")
			}

			// The command and its arguments are passed after "--"
			commandToRun := strings.Join(args, " ")

			client, err := tmux.NewClient()
			if err != nil {
				// Not in a tmux session, so run the command directly as a fallback.
				execCmd := exec.Command(args[0], args[1:]...)
				execCmd.Stdin = os.Stdin
				execCmd.Stdout = os.Stdout
				execCmd.Stderr = os.Stderr
				return execCmd.Run()
			}

			// In a tmux session, use the new window management function.
			return client.FocusOrRunCommandInWindow(context.Background(), commandToRun, windowName, windowIndex)
		},
	}

	cmd.Flags().StringP("name", "n", "", "Name of the target tmux window (required)")
	cmd.Flags().IntP("index", "i", -1, "Index (position) for the tmux window")
	cmd.MarkFlagRequired("name")
	return cmd
}
