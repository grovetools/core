package tmux

import (
	"context"
	"fmt"
	"os"
	"time"
)

func (c *Client) Launch(ctx context.Context, opts LaunchOptions) error {
	if opts.SessionName == "" {
		return fmt.Errorf("session name is required")
	}

	args := []string{"new-session", "-d", "-s", opts.SessionName}
	if opts.WorkingDirectory != "" {
		args = append(args, "-c", opts.WorkingDirectory)
	}

	_, err := c.run(ctx, args...)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	if opts.WindowName != "" {
		_, err = c.run(ctx, "rename-window", "-t", opts.SessionName, opts.WindowName)
		if err != nil {
			return fmt.Errorf("failed to rename window: %w", err)
		}
	}

	// Move window to specified index if provided
	if opts.WindowIndex >= 0 {
		windowToMove := opts.WindowName
		if windowToMove == "" {
			// Get the actual window name from tmux - it may be the shell name, not the session name
			windows, err := c.ListWindowsDetailed(ctx, opts.SessionName)
			if err == nil && len(windows) > 0 {
				windowToMove = windows[0].Name
			} else {
				// Fallback to session name if we can't get the window list
				windowToMove = opts.SessionName
			}
		}
		// InsertWindowAt handles moving or swapping to the target index
		if err := c.InsertWindowAt(ctx, opts.SessionName, windowToMove, opts.WindowIndex); err != nil {
			// This is not a fatal error for session creation, but we should log it
			fmt.Fprintf(os.Stderr, "Warning: failed to move window '%s' to index %d: %v\n", windowToMove, opts.WindowIndex, err)
		}
	}

	for i, pane := range opts.Panes {
		if i == 0 {
			target := opts.SessionName
			// Set environment variables before running the command
			if pane.Env != nil && len(pane.Env) > 0 {
				if err := c.SetPaneEnvironment(ctx, target, pane.Env); err != nil {
					return fmt.Errorf("failed to set environment for first pane: %w", err)
				}
				// Give the shell a moment to process all the export commands
				// This is especially important for fish shell which echoes each command
				// and needs time for output to settle before the next command
				time.Sleep(1 * time.Second)
			}
			if pane.Command != "" {
				// Debug: print the command being sent
				fmt.Fprintf(os.Stderr, "DEBUG: Sending command to pane: %q\n", pane.Command)
				// Send an extra Enter to ensure we have a fresh prompt
				// This helps with fish shell after setting many env vars
				_, err = c.run(ctx, "send-keys", "-t", opts.SessionName, "C-m")
				if err != nil {
					return fmt.Errorf("failed to send newline before command: %w", err)
				}
				time.Sleep(100 * time.Millisecond)
				_, err = c.run(ctx, "send-keys", "-t", opts.SessionName, pane.Command, "Enter")
				if err != nil {
					return fmt.Errorf("failed to send command to first pane: %w", err)
				}
			}
			if pane.SendKeys != "" {
				_, err = c.run(ctx, "send-keys", "-t", opts.SessionName, pane.SendKeys, "Enter")
				if err != nil {
					return fmt.Errorf("failed to send keys to first pane: %w", err)
				}
			}
		} else {
			splitArgs := []string{"split-window", "-t", opts.SessionName}
			if pane.WorkingDirectory != "" {
				splitArgs = append(splitArgs, "-c", pane.WorkingDirectory)
			}

			_, err = c.run(ctx, splitArgs...)
			if err != nil {
				return fmt.Errorf("failed to create pane %d: %w", i, err)
			}

			target := fmt.Sprintf("%s.%d", opts.SessionName, i)
			// Set environment variables before running the command
			if pane.Env != nil && len(pane.Env) > 0 {
				if err := c.SetPaneEnvironment(ctx, target, pane.Env); err != nil {
					return fmt.Errorf("failed to set environment for pane %d: %w", i, err)
				}
			}
			if pane.Command != "" {
				_, err = c.run(ctx, "send-keys", "-t", target, pane.Command, "Enter")
				if err != nil {
					return fmt.Errorf("failed to send command to pane %d: %w", i, err)
				}
			}
			if pane.SendKeys != "" {
				_, err = c.run(ctx, "send-keys", "-t", target, pane.SendKeys, "Enter")
				if err != nil {
					return fmt.Errorf("failed to send keys to pane %d: %w", i, err)
				}
			}
		}
	}

	if len(opts.Panes) > 1 {
		_, err = c.run(ctx, "select-layout", "-t", opts.SessionName, "tiled")
		if err != nil {
			return fmt.Errorf("failed to apply layout: %w", err)
		}
	}

	return nil
}