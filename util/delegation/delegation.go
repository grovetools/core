package delegation

import (
	"context"
	"os/exec"
)

// Command builds a command that routes through the grove delegator if available.
// If grove is not available in PATH, it falls back to direct execution of the tool.
//
// This ensures compatibility with:
// - Production environments where grove manages all tools
// - Test environments where only individual binaries exist
// - Direct binary execution scenarios
//
// Usage:
//   cmd := delegation.Command("flow", "plan", "status")
//   // Executes: grove flow plan status (if grove exists)
//   // Or: flow plan status (if grove doesn't exist)
//
//   cmd := delegation.Command("cx", "rules")
//   // Executes: grove cx rules (if grove exists)
//   // Or: cx rules (if grove doesn't exist)
func Command(tool string, args ...string) *exec.Cmd {
	// Check if grove is available in PATH
	if _, err := exec.LookPath("grove"); err == nil {
		// Grove is available, use it as delegator to respect user-configured aliases
		cmdArgs := append([]string{tool}, args...)
		return exec.Command("grove", cmdArgs...)
	}
	// Grove not available, use direct execution
	return exec.Command(tool, args...)
}

// CommandContext builds a context-aware command that routes through the grove delegator if available.
// If grove is not available in PATH, it falls back to direct execution of the tool.
//
// This is similar to Command but supports context cancellation.
//
// Usage:
//   cmd := delegation.CommandContext(ctx, "cx", "generate")
//   // Executes: grove cx generate (if grove exists)
//   // Or: cx generate (if grove doesn't exist)
func CommandContext(ctx context.Context, tool string, args ...string) *exec.Cmd {
	// Check if grove is available in PATH
	if _, err := exec.LookPath("grove"); err == nil {
		// Grove is available, use it as delegator to respect user-configured aliases
		cmdArgs := append([]string{tool}, args...)
		return exec.CommandContext(ctx, "grove", cmdArgs...)
	}
	// Grove not available, use direct execution
	return exec.CommandContext(ctx, tool, args...)
}
