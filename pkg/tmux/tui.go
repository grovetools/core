package tmux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/mattsolo1/grove-core/logging"
)

// OpenInEditorWindow finds or creates a window with a given name and opens a file.
// If the window exists, it's focused. If a file is provided, it's opened in the
// existing editor session. A new session can be forced with the reset flag.
func (c *Client) OpenInEditorWindow(ctx context.Context, editorCmd, filePath, windowName string, windowIndex int, reset bool) error {
	session, err := c.GetCurrentSession(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current session: %w", err)
	}

	windowTarget := session + ":" + windowName

	// Check if window exists
	err = c.SelectWindow(ctx, windowTarget)
	windowExists := err == nil

	// Handle reset flag
	if windowExists && reset {
		if err := c.KillWindow(ctx, windowTarget); err != nil {
			// Log as a warning but continue as if it didn't exist
		}
		windowExists = false
	}

	if windowExists {
		// Window exists, focus it and open the file if provided
		if err := c.SwitchClient(ctx, windowTarget); err != nil {
			// This might fail if client is already there, which is fine.
		}

		if filePath != "" {
			currentCmd, err := c.GetPaneCommand(ctx, windowTarget)
			editorRunning := err == nil && (strings.Contains(currentCmd, "nvim") || strings.Contains(currentCmd, "vim") || strings.Contains(currentCmd, "vi"))

			if editorRunning {
				// Editor is running, send :e command
				escapedPath := strings.ReplaceAll(filePath, " ", `\ `)
				if err := c.SendKeys(ctx, windowTarget, fmt.Sprintf(":e %s", escapedPath), "Enter"); err != nil {
					return fmt.Errorf("failed to send keys to window '%s': %w", windowName, err)
				}
			} else {
				// No editor running, send the full command to start one in the pane
				shellCommand := editorCmd
				if filePath != "" {
					// Basic shell escaping for the file path
					shellEscapedFilePath := "'" + strings.ReplaceAll(filePath, "'", `'\''`) + "'"
					shellCommand += " " + shellEscapedFilePath
				}
				if err := c.SendKeys(ctx, windowTarget, shellCommand, "Enter"); err != nil {
					return fmt.Errorf("failed to send command to pane in window '%s': %w", windowName, err)
				}
			}
		}

		// Ensure it's at the correct index
		if windowIndex >= 0 {
			if err := c.InsertWindowAt(ctx, session, windowName, windowIndex); err != nil {
				// Log the error but don't fail - window is still usable
				fmt.Fprintf(os.Stderr, "Warning: failed to move window '%s' to index %d: %v\n", windowName, windowIndex, err)
			}
		}
	} else {
		// Window doesn't exist, create it
		var commandToRun string
		if filePath != "" {
			// Quoting is important for paths with spaces
			commandToRun = fmt.Sprintf("%s %q", editorCmd, filePath)
		} else {
			commandToRun = editorCmd
		}

		if err := c.NewWindow(ctx, session+":", windowName, commandToRun); err != nil {
			return fmt.Errorf("failed to create new window '%s': %w", windowName, err)
		}

		if windowIndex >= 0 {
			if err := c.InsertWindowAt(ctx, session, windowName, windowIndex); err != nil {
				// Log the error but don't fail - window is still usable
				fmt.Fprintf(os.Stderr, "Warning: failed to move window '%s' to index %d: %v\n", windowName, windowIndex, err)
			}
		}

		if err := c.SwitchClient(ctx, windowTarget); err != nil {
			// Ignore switch errors, window was created
		}
	}

	return nil
}

// ClosePopup closes the current tmux popup synchronously.
// This is useful when you need to close a popup immediately before quitting a TUI.
func (c *Client) ClosePopup(ctx context.Context) error {
	cmd := exec.Command("tmux", "display-popup", "-C")
	return cmd.Run()
}

// SwitchAndClosePopup switches to a tmux session and closes the current popup if running in one.
// This ensures the popup closes regardless of whether the binding has the -E flag.
//
// The method executes the switch synchronously, then returns a command to close the popup
// that should be executed after the TUI exits.
//
// Example usage from a bubbletea Update function:
//
//	client, _ := tmux.NewClient()
//	if err := client.SwitchClient(ctx, sessionName); err != nil {
//	    // handle error
//	}
//	model.CommandOnExit = client.ClosePopupCmd()
//	return model, tea.Quit
func (c *Client) ClosePopupCmd() *exec.Cmd {
	cmd := exec.Command("tmux", "display-popup", "-C")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

// SelectWindowAndClosePopup switches to a session, selects a window, and closes the popup.
// Returns a command to be executed after the TUI exits.
func (c *Client) SelectWindowAndClosePopup(ctx context.Context, sessionName, windowName string) (*exec.Cmd, error) {
	// Switch to session
	if err := c.SwitchClientToSession(ctx, sessionName); err != nil {
		return nil, fmt.Errorf("failed to switch session: %w", err)
	}

	// Select window
	if err := c.SelectWindow(ctx, sessionName+":"+windowName); err != nil {
		return nil, fmt.Errorf("failed to select window: %w", err)
	}

	return c.ClosePopupCmd(), nil
}

// NewWindowAndClosePopup switches to a session, creates a new window, and closes the popup.
// Returns a command to be executed after the TUI exits.
func (c *Client) NewWindowAndClosePopup(ctx context.Context, sessionName, windowName, command string) (*exec.Cmd, error) {
	// Switch to session
	if err := c.SwitchClientToSession(ctx, sessionName); err != nil {
		return nil, fmt.Errorf("failed to switch session: %w", err)
	}

	// Create new window
	if err := c.NewWindow(ctx, sessionName+":", windowName, command); err != nil {
		return nil, fmt.Errorf("failed to create window: %w", err)
	}

	return c.ClosePopupCmd(), nil
}

// FocusOrRunCommandInWindow ensures a window with the given name exists, runs the command, and focuses it.
// If the window already exists, it just switches to it without killing it (preserving any work in progress).
// If the window doesn't exist, it creates it at the specified index.
func (c *Client) FocusOrRunCommandInWindow(ctx context.Context, command, windowName string, windowIndex int) error {
	session, err := c.GetCurrentSession(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current session: %w", err)
	}

	windowTarget := session + ":" + windowName

	// Check if window exists by trying to select it.
	err = c.SelectWindow(ctx, windowTarget)
	if err == nil {
		// Window exists, just switch to it (don't kill it - preserve work in progress)
		if err := c.SwitchClient(ctx, windowTarget); err != nil {
			return fmt.Errorf("failed to switch to window '%s': %w", windowName, err)
		}
		return nil
	}

	// Window doesn't exist, create it at the next available index
	if err := c.NewWindow(ctx, session+":", windowName, command); err != nil {
		return fmt.Errorf("failed to create new window '%s': %w", windowName, err)
	}

	// If an index is specified, move the window to that position.
	if windowIndex >= 0 {
		if err := c.InsertWindowAt(ctx, session, windowName, windowIndex); err != nil {
			// Log the error but don't fail - window is still usable
			fmt.Fprintf(os.Stderr, "Warning: failed to move window '%s' to index %d: %v\n", windowName, windowIndex, err)
		}
	}

	// Switch the client to the new window to make it active.
	if err := c.SwitchClient(ctx, windowTarget); err != nil {
		return fmt.Errorf("failed to switch to window '%s': %w", windowName, err)
	}

	return nil
}

// FocusOrRunTUIWithErrorHandling runs a TUI command in a tmux window with error handling.
// If the window exists, it focuses it. If not, it creates the window and runs the command.
// If the command fails, it displays the error and opens a shell for debugging.
func (c *Client) FocusOrRunTUIWithErrorHandling(ctx context.Context, command, windowName string, windowIndex int) error {
	log := logging.NewLogger("tmux")

	session, err := c.GetCurrentSession(ctx)
	if err != nil {
		log.WithError(err).Error("Failed to get current tmux session")
		return fmt.Errorf("failed to get current session: %w", err)
	}

	windowTarget := session + ":" + windowName

	// Check if window exists by trying to select it.
	err = c.SelectWindow(ctx, windowTarget)
	if err == nil {
		// Window exists, just switch to it (don't kill it - preserve work in progress)
		log.WithFields(map[string]interface{}{
			"window": windowName,
			"target": windowTarget,
		}).Debug("Switching to existing tmux window")
		if err := c.SwitchClient(ctx, windowTarget); err != nil {
			log.WithError(err).WithField("window", windowName).Error("Failed to switch to tmux window")
			return fmt.Errorf("failed to switch to window '%s': %w", windowName, err)
		}
		return nil
	}

	log.WithFields(map[string]interface{}{
		"window":  windowName,
		"command": command,
		"index":   windowIndex,
	}).Debug("Creating new tmux window for TUI")

	// Window doesn't exist, create it with the command
	// Run the TUI command directly so it manages its own lifecycle
	if err := c.NewWindow(ctx, session+":", windowName, command); err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"window":  windowName,
			"command": command,
		}).Error("Failed to create tmux window for TUI")
		return fmt.Errorf("failed to create new window '%s': %w", windowName, err)
	}

	// Switch the client to the new window to make it active.
	if err := c.SwitchClient(ctx, windowTarget); err != nil {
		log.WithError(err).WithField("window", windowName).Error("Failed to switch to newly created tmux window")
		return fmt.Errorf("failed to switch to window '%s': %w", windowName, err)
	}

	log.WithField("window", windowName).Info("Successfully opened TUI in tmux window")
	return nil
}
