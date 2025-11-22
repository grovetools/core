package tmux

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/mattsolo1/grove-core/command"
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
	return &Client{
		builder: builder,
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