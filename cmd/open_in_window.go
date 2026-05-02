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

// NewOpenInWindowCmd creates the `open-in-window` command.
func NewOpenInWindowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "open-in-window -- [command...]",
		Short: "Runs a command in a dedicated, focused tmux window",
		Long:  `Finds or creates a tmux window with a given name, runs the specified command if the window is new, and focuses it. If the window already exists, it is simply focused; any existing process is NOT killed or replaced.`,
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			windowName, _ := cmd.Flags().GetString("name")
			windowIndex, _ := cmd.Flags().GetInt("index")

			if windowName == "" {
				return fmt.Errorf("--name flag is required")
			}

			commandToRun := strings.Join(args, " ")

			ctx := context.Background()
			engine, err := mux.DetectMuxEngine(ctx)
			if err != nil {
				// Not in a mux session, so run the command directly as a fallback.
				execCmd := exec.Command(args[0], args[1:]...) //nolint:gosec // args from CLI invocation
				execCmd.Stdin = os.Stdin
				execCmd.Stdout = os.Stdout
				execCmd.Stderr = os.Stderr
				return execCmd.Run()
			}

			tuiEngine, ok := engine.(mux.MuxTUIEngine)
			if !ok {
				return fmt.Errorf("mux engine does not support window management")
			}

			return tuiEngine.FocusOrRunCommandInWindow(ctx, commandToRun, windowName, windowIndex)
		},
	}

	cmd.Flags().StringP("name", "n", "", "Name of the target tmux window (required)")
	cmd.Flags().IntP("index", "i", -1, "Index (position) for the tmux window")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}
