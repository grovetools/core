package tmux

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func (c *Client) SessionExists(ctx context.Context, sessionName string) (bool, error) {
	_, err := c.run(ctx, "has-session", "-t", "="+sessionName)
	if err == nil {
		return true, nil
	}

	if strings.Contains(err.Error(), "exit status 1") {
		return false, nil
	}

	return false, err
}

// IsPopup checks if the current pane is inside a tmux popup window.
func (c *Client) IsPopup(ctx context.Context) (bool, error) {
	// Method 1: Check GROVE_IN_POPUP environment variable (set by user's binding)
	// This is the most reliable method - users should use:
	// bind-key -n C-n display-popup -w 100% -h 100% -x C -y C -E "GROVE_IN_POPUP=1 nb tui"
	if value := os.Getenv("GROVE_IN_POPUP"); value == "1" || value == "true" {
		return true, nil
	}

	// Method 2: Check TMUX_POPUP environment variable (if set by user)
	if _, exists := os.LookupEnv("TMUX_POPUP"); exists {
		return true, nil
	}

	// Method 3: Check for window with 'M' flag (popup menu)
	output, err := c.run(ctx, "display-message", "-p", "#{window_flags}")
	if err != nil {
		return false, err
	}
	if strings.Contains(strings.TrimSpace(output), "M") {
		return true, nil
	}

	// Method 4: Check if pane is marked (used in some popup configurations)
	output, err = c.run(ctx, "display-message", "-p", "#{pane_marked}")
	if err == nil && strings.TrimSpace(output) == "1" {
		return true, nil
	}

	return false, nil
}

func (c *Client) KillSession(ctx context.Context, sessionName string) error {
	_, err := c.run(ctx, "kill-session", "-t", "="+sessionName)
	return err
}

func (c *Client) CapturePane(ctx context.Context, target string) (string, error) {
	// Use -e flag to preserve ANSI escape codes (colors, formatting, etc.)
	// Use -p flag to print to stdout
	output, err := c.run(ctx, "capture-pane", "-e", "-p", "-t", target)
	if err != nil {
		return "", err
	}
	return output, nil
}

func (c *Client) NewWindow(ctx context.Context, target, windowName, command string) error {
	args := []string{"new-window", "-t", target, "-n", windowName}
	if command != "" {
		args = append(args, command)
	}
	_, err := c.run(ctx, args...)
	return err
}

func (c *Client) SendKeys(ctx context.Context, target string, keys ...string) error {
	args := []string{"send-keys", "-t", target}
	args = append(args, keys...)
	_, err := c.run(ctx, args...)
	return err
}

func (c *Client) SelectWindow(ctx context.Context, target string) error {
	_, err := c.run(ctx, "select-window", "-t", target)
	return err
}

func (c *Client) MoveWindow(ctx context.Context, source, target string) error {
	_, err := c.run(ctx, "move-window", "-s", source, "-t", target)
	return err
}

func (c *Client) SwapWindow(ctx context.Context, source, target string) error {
	_, err := c.run(ctx, "swap-window", "-s", source, "-t", target)
	return err
}

// ListWindowsDetailed returns a list of windows with detailed information for the given session.
func (c *Client) ListWindowsDetailed(ctx context.Context, sessionName string) ([]Window, error) {
	format := `#{window_id}:#{window_index}:#{window_name}:#{?window_active,1,0}:#{pane_current_command}:#{pane_pid}`
	output, err := c.run(ctx, "list-windows", "-t", sessionName, "-F", format)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	windows := make([]Window, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 6)
		if len(parts) < 6 {
			continue // Skip malformed lines
		}

		index, err := strconv.Atoi(parts[1])
		if err != nil {
			continue // Skip if index is not a number
		}

		pid, err := strconv.Atoi(parts[5])
		if err != nil {
			pid = 0 // Default to 0 if PID can't be parsed
		}

		win := Window{
			ID:       parts[0],
			Index:    index,
			Name:     parts[2],
			IsActive: parts[3] == "1",
			Command:  parts[4],
			PID:      pid,
		}
		windows = append(windows, win)
	}
	return windows, nil
}

// RenameWindow renames a tmux window.
func (c *Client) RenameWindow(ctx context.Context, target string, newName string) error {
	_, err := c.run(ctx, "rename-window", "-t", target, newName)
	return err
}

// ListWindows returns a list of window indices for the current session
func (c *Client) ListWindows(ctx context.Context, session string) ([]int, error) {
	output, err := c.run(ctx, "list-windows", "-t", session, "-F", "#{window_index}")
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	indices := make([]int, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		idx, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil {
			continue
		}
		indices = append(indices, idx)
	}
	return indices, nil
}

// InsertWindowAt moves a window to a specific position
// Uses tmux's swap-window to avoid renumbering issues
func (c *Client) InsertWindowAt(ctx context.Context, session, windowName string, targetIndex int) error {
	windowTarget := session + ":" + windowName
	targetPos := fmt.Sprintf("%s:%d", session, targetIndex)

	// Check if target index exists
	err := c.SelectWindow(ctx, targetPos)
	if err == nil {
		// Target exists, swap with it
		return c.SwapWindow(ctx, windowTarget, targetPos)
	}

	// Target doesn't exist, just move to it
	return c.MoveWindow(ctx, windowTarget, targetPos)
}

func (c *Client) GetPaneCommand(ctx context.Context, target string) (string, error) {
	output, err := c.run(ctx, "display-message", "-t", target, "-p", "#{pane_current_command}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func (c *Client) KillWindow(ctx context.Context, target string) error {
	_, err := c.run(ctx, "kill-window", "-t", target)
	return err
}

func (c *Client) SwitchClient(ctx context.Context, target string) error {
	_, err := c.run(ctx, "switch-client", "-t", target)
	return err
}

// SwitchClientToSession switches the client to the specified session.
// It uses an exact match for the session name to avoid ambiguity.
func (c *Client) SwitchClientToSession(ctx context.Context, sessionName string) error {
	_, err := c.run(ctx, "switch-client", "-t", "="+sessionName)
	return err
}

func (c *Client) GetCurrentSession(ctx context.Context) (string, error) {
	output, err := c.run(ctx, "display-message", "-p", "#{session_name}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func (c *Client) ListSessions(ctx context.Context) ([]string, error) {
	output, err := c.run(ctx, "list-sessions", "-F", "#{session_name}")
	if err != nil {
		// If no sessions exist, tmux returns an error
		if strings.Contains(err.Error(), "no server running") || strings.Contains(err.Error(), "exit status 1") {
			return []string{}, nil
		}
		return nil, err
	}

	sessions := strings.Split(strings.TrimSpace(output), "\n")
	return sessions, nil
}

// GetSessionPath returns the working directory path of a specific tmux session.
func (c *Client) GetSessionPath(ctx context.Context, sessionName string) (string, error) {
	output, err := c.run(ctx, "display-message", "-p", "-t", "="+sessionName, "#{session_path}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

// GetSessionPID returns the process ID of the tmux server for a given session.
func (c *Client) GetSessionPID(ctx context.Context, sessionName string) (int, error) {
	output, err := c.run(ctx, "display-message", "-p", "-t", "="+sessionName, "#{session_pid}")
	if err != nil {
		return 0, fmt.Errorf("failed to get session PID from tmux: %w", err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(output))
	if err != nil {
		return 0, fmt.Errorf("failed to parse session PID from tmux output '%s': %w", output, err)
	}

	return pid, nil
}

// GetCursorPosition returns the 1-based row and column of the cursor in the specified session's active pane.
func (c *Client) GetCursorPosition(ctx context.Context, sessionName string) (row int, col int, err error) {
	// target-pane format is {session}:. which targets the active pane
	targetPane := sessionName + ":."

	// Get cursor position using tmux display-message with cursor_y and cursor_x format
	output, err := c.run(ctx, "display-message", "-p", "-t", targetPane, "#{cursor_y},#{cursor_x}")
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get cursor position from tmux: %w", err)
	}

	parts := strings.Split(strings.TrimSpace(output), ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("unexpected output from tmux for cursor position: %s", output)
	}

	y, errY := strconv.Atoi(parts[0])
	x, errX := strconv.Atoi(parts[1])

	if errY != nil || errX != nil {
		return 0, 0, fmt.Errorf("failed to parse cursor coordinates from output '%s'", output)
	}

	// Tmux provides 0-indexed coordinates, so convert to 1-based for the API
	return y + 1, x + 1, nil
}

// GetCurrentPaneID returns the ID of the currently active pane
func (c *Client) GetCurrentPaneID(ctx context.Context) (string, error) {
	output, err := c.run(ctx, "display-message", "-p", "#{pane_id}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

// PaneExists checks if a pane with the given ID exists
func (c *Client) PaneExists(ctx context.Context, paneID string) bool {
	// Try to get the pane's ID - if it doesn't exist, this will fail
	_, err := c.run(ctx, "display-message", "-p", "-t", paneID, "#{pane_id}")
	return err == nil
}

// GetPaneWidth returns the width of the specified pane (or current pane if target is empty)
func (c *Client) GetPaneWidth(ctx context.Context, target string) (int, error) {
	args := []string{"display-message", "-p", "#{pane_width}"}
	if target != "" {
		args = append(args, "-t", target)
	}
	output, err := c.run(ctx, args...)
	if err != nil {
		return 0, fmt.Errorf("failed to get pane width: %w", err)
	}

	width, err := strconv.Atoi(strings.TrimSpace(output))
	if err != nil {
		return 0, fmt.Errorf("failed to parse pane width '%s': %w", output, err)
	}
	return width, nil
}

// SelectPane switches focus to the specified pane
func (c *Client) SelectPane(ctx context.Context, paneID string) error {
	_, err := c.run(ctx, "select-pane", "-t", paneID)
	return err
}

// SplitWindow splits a target pane and optionally runs a command.
// target can be a session, window, or pane identifier. Use "" for the current pane.
// If horizontal is true, it creates a vertical split (side-by-side panes).
// size, if > 0, specifies the width (for horizontal) or height (for vertical) of the new pane in cells.
// Returns the pane ID of the newly created pane.
func (c *Client) SplitWindow(ctx context.Context, target string, horizontal bool, size int, command string) (string, error) {
	args := []string{"split-window", "-P", "-F", "#{pane_id}"}
	if horizontal {
		args = append(args, "-h")
	}
	if size > 0 {
		args = append(args, "-l", strconv.Itoa(size))
	}
	if target != "" {
		args = append(args, "-t", target)
	}
	if command != "" {
		args = append(args, command)
	}

	output, err := c.run(ctx, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

// GetWindowPaneID returns the active pane ID for a given window target.
func (c *Client) GetWindowPaneID(ctx context.Context, windowTarget string) (string, error) {
	output, err := c.run(ctx, "display-message", "-p", "-t", windowTarget, "#{pane_id}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

// SetPaneEnvironment sets environment variables for a specific pane by sending export commands.
// This affects the shell running in the pane for its entire lifetime.
// Commands are prefixed with a space to prevent shell history pollution.
func (c *Client) SetPaneEnvironment(ctx context.Context, paneTarget string, env map[string]string) error {
	for key, value := range env {
		// Use %q to safely quote the value for the shell.
		// Prepend space to prevent shell history pollution (bash, zsh, fish).
		exportCmd := fmt.Sprintf(" export %s=%q", key, value)
		if err := c.SendKeys(ctx, paneTarget, exportCmd, "C-m"); err != nil {
			return fmt.Errorf("failed to send export command for %s to pane %s: %w", key, paneTarget, err)
		}
	}
	return nil
}

// NewWindowWithOptions creates a new window with extended options.
func (c *Client) NewWindowWithOptions(ctx context.Context, opts NewWindowOptions) error {
	args := []string{"new-window", "-t", opts.Target, "-n", opts.WindowName}
	if opts.WorkingDir != "" {
		args = append(args, "-c", opts.WorkingDir)
	}
	for _, e := range opts.Env {
		args = append(args, "-e", e)
	}
	if opts.Command != "" {
		args = append(args, opts.Command)
	}
	_, err := c.run(ctx, args...)
	return err
}