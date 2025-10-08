package tmux

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

func (c *Client) SessionExists(ctx context.Context, sessionName string) (bool, error) {
	_, err := c.run(ctx, "has-session", "-t", sessionName)
	if err == nil {
		return true, nil
	}

	if strings.Contains(err.Error(), "exit status 1") {
		return false, nil
	}

	return false, err
}

func (c *Client) KillSession(ctx context.Context, sessionName string) error {
	_, err := c.run(ctx, "kill-session", "-t", sessionName)
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

func (c *Client) SwitchClient(ctx context.Context, target string) error {
	_, err := c.run(ctx, "switch-client", "-t", target)
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
	output, err := c.run(ctx, "display-message", "-p", "-t", sessionName, "#{session_path}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

// GetSessionPID returns the process ID of the tmux server for a given session.
func (c *Client) GetSessionPID(ctx context.Context, sessionName string) (int, error) {
	output, err := c.run(ctx, "display-message", "-p", "-t", sessionName, "#{session_pid}")
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