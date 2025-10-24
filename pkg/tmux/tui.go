package tmux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

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
