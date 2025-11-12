package tmux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// OpenFileInEditor finds or creates a window with the given name and opens the file in it.
// If the window exists, it switches to it and opens the file.
// If not, it creates the window with the editor and file, and switches the client to it.
// It's intended for workflows where a TUI in a popup needs to open a file in the main session.
// The windowIndex parameter specifies the desired window position (0-based). Use -1 to skip positioning.
func (c *Client) OpenFileInEditor(ctx context.Context, editorCmd string, filePath string, windowName string, windowIndex int) error {
	session, err := c.GetCurrentSession(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current session: %w", err)
	}

	if windowName == "" {
		windowName = "editor" // A sensible default
	}
	windowTarget := session + ":" + windowName

	// Try to select the window to see if it exists
	err = c.SelectWindow(ctx, windowTarget)
	windowExists := (err == nil)

	if windowExists {
		// Check if an editor is still running in the window
		currentCmd, err := c.GetPaneCommand(ctx, windowTarget)
		editorRunning := err == nil && (currentCmd == "nvim" || currentCmd == "vim" || currentCmd == "vi")

		if !editorRunning {
			// Editor not running, kill the window and recreate it
			if err := c.KillWindow(ctx, windowTarget); err != nil {
				// Ignore error, continue to recreate
			}
			windowExists = false
		} else {
			// Editor is running, insert it at desired position if specified
			if windowIndex >= 0 {
				if err := c.InsertWindowAt(ctx, session, windowName, windowIndex); err != nil {
					// Ignore insert errors - window exists and we can still use it
				}
			}

			// Switch to it and send keys to open the file (if a file was specified)
			if err := c.SwitchClient(ctx, windowTarget); err != nil {
				// SwitchClient might fail, but SelectWindow already worked, so continue
			}

			// Only send :e command if a file path was specified
			if filePath != "" {
				// Escape path for vim's :e command
				escapedPath := strings.ReplaceAll(filePath, " ", `\ `)

				// Send keys to the active pane in the window
				// Use empty string as target to send to the current pane
				if err := c.SendKeys(ctx, "", fmt.Sprintf(":e %s", escapedPath), "Enter"); err != nil {
					return fmt.Errorf("failed to send keys to window '%s': %w", windowName, err)
				}
			}
		}
	}

	if !windowExists {
		// Window doesn't exist, create it
		// The command needs to be properly quoted for the shell
		var command string
		if filePath != "" {
			command = fmt.Sprintf("%s %q", editorCmd, filePath)
		} else {
			command = editorCmd
		}

		if err := c.NewWindow(ctx, session+":", windowName, command); err != nil {
			return fmt.Errorf("failed to create new window: %w", err)
		}

		// Insert it at the desired position if specified
		if windowIndex >= 0 {
			if err := c.InsertWindowAt(ctx, session, windowName, windowIndex); err != nil {
				// Ignore insert errors - window was created successfully
			}
		}

		// Switch to the new window
		if err := c.SwitchClient(ctx, windowTarget); err != nil {
			// Ignore errors - the window was created successfully
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
	if err := c.SwitchClient(ctx, sessionName); err != nil {
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
	if err := c.SwitchClient(ctx, sessionName); err != nil {
		return nil, fmt.Errorf("failed to switch session: %w", err)
	}

	// Create new window
	if err := c.NewWindow(ctx, sessionName+":", windowName, command); err != nil {
		return nil, fmt.Errorf("failed to create window: %w", err)
	}

	return c.ClosePopupCmd(), nil
}
