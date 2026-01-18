package tmux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/grovetools/core/command"
)

type Client struct {
	builder *command.SafeBuilder
	socket  string // Socket name for dedicated tmux server (uses -L flag)
}

func NewClient() (*Client, error) {
	if _, err := exec.LookPath("tmux"); err != nil {
		return nil, fmt.Errorf("tmux command not found in PATH: %w", err)
	}

	builder := command.NewSafeBuilder()

	// Check if we're in a test environment with an isolated tmux socket
	// Tests set GROVE_TMUX_SOCKET to ensure spawned processes use the same isolated server
	socket := ""
	if testSocket := os.Getenv("GROVE_TMUX_SOCKET"); testSocket != "" {
		socket = testSocket
	}

	return &Client{
		builder: builder,
		socket:  socket,
	}, nil
}

// NewClientWithSocket creates a tmux client that uses a dedicated server socket.
// This provides isolation from the default tmux server.
func NewClientWithSocket(socket string) (*Client, error) {
	if _, err := exec.LookPath("tmux"); err != nil {
		return nil, fmt.Errorf("tmux command not found in PATH: %w", err)
	}

	builder := command.NewSafeBuilder()
	return &Client{
		builder: builder,
		socket:  socket,
	}, nil
}

// Socket returns the socket name this client uses, or empty string for default.
func (c *Client) Socket() string {
	return c.socket
}

// KillServer kills the tmux server for this client's socket.
// This is useful for cleaning up isolated test servers.
// If the client uses the default socket, this will kill the default tmux server (use with caution!).
func (c *Client) KillServer(ctx context.Context) error {
	_, err := c.run(ctx, "kill-server")
	// Ignore "no server running" errors - server is already gone
	if err != nil && strings.Contains(err.Error(), "no server running") {
		return nil
	}
	return err
}

// Command creates an exec.Cmd for tmux that respects GROVE_TMUX_SOCKET.
// Use this instead of exec.Command("tmux", ...) for socket-aware operations
// when you need a raw *exec.Cmd (e.g., for AttachStdin/Stdout).
// For most operations, prefer using the Client methods instead.
func Command(args ...string) *exec.Cmd {
	if socket := os.Getenv("GROVE_TMUX_SOCKET"); socket != "" {
		args = append([]string{"-L", socket}, args...)
	}
	return exec.Command("tmux", args...)
}

func (c *Client) run(ctx context.Context, args ...string) (string, error) {
	// Prepend socket flag if using a dedicated server
	if c.socket != "" {
		args = append([]string{"-L", c.socket}, args...)
	}

	cmd, err := c.builder.Build(ctx, "tmux", args...)
	if err != nil {
		return "", fmt.Errorf("failed to build command: %w", err)
	}

	execCmd := cmd.Exec()
	output, err := execCmd.CombinedOutput()
	if err != nil {
		cmdStr := "tmux " + strings.Join(args, " ")
		return string(output), fmt.Errorf("tmux command failed: `%s`: %w, output: %s", cmdStr, err, string(output))
	}

	return string(output), nil
}