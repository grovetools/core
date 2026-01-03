package command

import (
	"context"
	"os/exec"
)

// Executor creates exec.Cmd instances. This abstraction allows for dependency
// injection, enabling test-specific command creation logic (e.g., setting up
// a PATH with mock binaries) without modifying production code.
type Executor interface {
	// Command creates a new exec.Cmd instance for the given command and arguments.
	Command(name string, args ...string) *exec.Cmd

	// CommandContext creates a new context-aware exec.Cmd instance.
	CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd
}

// RealExecutor is the production implementation of the Executor interface,
// which uses the standard os/exec package to create commands.
type RealExecutor struct{}

// Command creates a standard exec.Cmd.
func (e *RealExecutor) Command(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

// CommandContext creates a standard context-aware exec.Cmd.
func (e *RealExecutor) CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}
