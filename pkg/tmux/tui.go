package tmux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// OpenFileInEditor finds or creates a window named "notebook" and opens the file in it.
// If the window exists, it switches to it and opens the file.
// If not, it creates the window with the editor and file, and switches the client to it.
// It's intended for workflows where a TUI in a popup needs to open a file in the main session.
func (c *Client) OpenFileInEditor(ctx context.Context, editorCmd string, filePath string) error {
	session, err := c.GetCurrentSession(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current session: %w", err)
	}

	windowName := "notebook"
	windowTarget := session + ":" + windowName

	// Try to select the "notebook" window to see if it exists
	err = c.SelectWindow(ctx, windowTarget)
	windowExists := (err == nil)

	if windowExists {
		// Notebook window exists, switch to it and send keys to open the file
		if err := c.SwitchClient(ctx, windowTarget); err != nil {
			// SwitchClient might fail, but SelectWindow already worked, so continue
		}

		// Escape path for vim's :e command
		escapedPath := strings.ReplaceAll(filePath, " ", `\ `)

		// Send keys to the active pane in the notebook window
		// Use empty string as target to send to the current pane
		if err := c.SendKeys(ctx, "", fmt.Sprintf(":e %s", escapedPath), "Enter"); err != nil {
			return fmt.Errorf("failed to send keys to notebook window: %w", err)
		}
	} else {
		// Notebook window doesn't exist, create it
		// The command needs to be properly quoted for the shell
		command := fmt.Sprintf("%s %q", editorCmd, filePath)

		if err := c.NewWindow(ctx, session+":", windowName, command); err != nil {
			return fmt.Errorf("failed to create new window: %w", err)
		}

		// Switch to the new window
		if err := c.SwitchClient(ctx, windowTarget); err != nil {
			// Ignore errors - the window was created successfully
		}
	}

	return nil
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
