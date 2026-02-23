// Package keygen provides unified tmux key binding generation logic.
// It supports multiple prefix modes for creating customizable keybinding hierarchies.
package keygen

import (
	"fmt"
	"strings"
)

// PrefixMode represents how bindings are scoped in tmux.
type PrefixMode int

const (
	// ModeDirectRoot binds directly to root table with -n (requires modifier keys).
	// prefix = ""
	ModeDirectRoot PrefixMode = iota
	// ModeDirectPrefix binds to the tmux prefix table.
	// prefix = "<prefix>"
	ModeDirectPrefix
	// ModeSubTablePrefix creates a sub-table under the tmux prefix.
	// prefix = "<prefix> X" where X is the key to enter the sub-table
	ModeSubTablePrefix
	// ModeSubTableRoot creates a sub-table under a root key.
	// prefix = "C-g" (any key combo) enters a custom key table
	ModeSubTableRoot
	// ModeGroveDirect binds directly to the grove-popups table.
	// prefix = "<grove>"
	ModeGroveDirect
	// ModeGroveSubTable creates a sub-table under grove-popups.
	// prefix = "<grove> X" where X is the key to enter the sub-table from grove-popups
	ModeGroveSubTable
)

// GrovePopupsTable is the standard name for the grove-popups key table.
const GrovePopupsTable = "grove-popups"

// Config holds prefix configuration for tmux key generation.
type Config struct {
	// Prefix is the user-configured prefix (e.g., "C-g", "<prefix>", "<prefix> g", "").
	Prefix string
	// TableName is the name of the key table (e.g., "grove-popups", "nav-workspaces").
	TableName string
}

// Mode returns the PrefixMode based on the Prefix string.
func (c *Config) Mode() PrefixMode {
	if c.Prefix == "" {
		return ModeDirectRoot
	}
	if c.Prefix == "<prefix>" {
		return ModeDirectPrefix
	}
	if strings.HasPrefix(c.Prefix, "<prefix> ") {
		return ModeSubTablePrefix
	}
	if c.Prefix == "<grove>" {
		return ModeGroveDirect
	}
	if strings.HasPrefix(c.Prefix, "<grove> ") {
		return ModeGroveSubTable
	}
	return ModeSubTableRoot
}

// GenerateEntryPoint returns bind-key lines for entering the key table.
// Returns empty slice for ModeDirectRoot, ModeDirectPrefix, and ModeGroveDirect.
func (c *Config) GenerateEntryPoint() []string {
	mode := c.Mode()
	var lines []string

	switch mode {
	case ModeSubTablePrefix:
		lines = append(lines, "# --- Prefix Entry Point ---")
		nativeKey := strings.TrimPrefix(c.Prefix, "<prefix> ")
		lines = append(lines, fmt.Sprintf("bind-key %s switch-client -T %s", EscapeKey(nativeKey), c.TableName))
		lines = append(lines, "")
	case ModeSubTableRoot:
		lines = append(lines, "# --- Prefix Entry Point ---")
		lines = append(lines, fmt.Sprintf("bind-key -n %s switch-client -T %s", EscapeKey(c.Prefix), c.TableName))
		lines = append(lines, "")
	case ModeDirectPrefix:
		lines = append(lines, "# --- Direct Prefix Table Mode ---")
		lines = append(lines, "# Bindings are added directly to tmux prefix table")
		lines = append(lines, "")
	case ModeGroveDirect:
		lines = append(lines, "# --- Grove Popups Table Mode ---")
		lines = append(lines, "# Bindings are added directly to grove-popups table")
		lines = append(lines, "")
	case ModeGroveSubTable:
		lines = append(lines, "# --- Grove Sub-table Entry Point ---")
		groveKey := strings.TrimPrefix(c.Prefix, "<grove> ")
		lines = append(lines, fmt.Sprintf("bind-key -T %s %s switch-client -T %s", GrovePopupsTable, EscapeKey(groveKey), c.TableName))
		lines = append(lines, "")
	}
	// ModeDirectRoot has no entry point

	return lines
}

// GenerateEscapeHatches returns bind-key lines for exiting the table.
// Includes: Escape, q, C-c to exit; ? for help; prefix passthrough.
// Returns empty slice for modes without a sub-table.
func (c *Config) GenerateEscapeHatches(helpCmd string) []string {
	mode := c.Mode()

	// These modes don't have sub-tables, so no escape hatches needed
	if mode == ModeDirectRoot || mode == ModeDirectPrefix || mode == ModeGroveDirect {
		return nil
	}

	// Determine where to escape to
	escapeTarget := "root"
	if mode == ModeGroveSubTable {
		escapeTarget = GrovePopupsTable
	}

	lines := []string{
		"# --- Built-in Table Commands ---",
		fmt.Sprintf("bind-key -T %s Escape switch-client -T %s", c.TableName, escapeTarget),
		fmt.Sprintf("bind-key -T %s C-c switch-client -T %s", c.TableName, escapeTarget),
		fmt.Sprintf("bind-key -T %s q switch-client -T %s", c.TableName, escapeTarget),
	}

	// For root key mode, add passthrough for the prefix key itself
	if mode == ModeSubTableRoot {
		escaped := EscapeKey(c.Prefix)
		lines = append(lines, fmt.Sprintf("bind-key -T %s %s send-keys %s", c.TableName, escaped, escaped))
	}

	// Add help command
	lines = append(lines, fmt.Sprintf("bind-key -T %s ? display-popup -w 100%% -h 98%% -E \"%s\"", c.TableName, EscapeCommand(helpCmd)))
	lines = append(lines, "")

	return lines
}

// BindTarget returns the bind-key flag for this mode:
// - ModeDirectRoot: "-n"
// - ModeDirectPrefix: "" (no flag)
// - ModeGroveDirect: "-T grove-popups"
// - ModeSubTablePrefix/ModeSubTableRoot/ModeGroveSubTable: "-T <TableName>"
func (c *Config) BindTarget() string {
	mode := c.Mode()
	switch mode {
	case ModeDirectRoot:
		return "-n"
	case ModeDirectPrefix:
		return ""
	case ModeGroveDirect:
		return "-T " + GrovePopupsTable
	default:
		return "-T " + c.TableName
	}
}

// FormatBindKey formats a complete bind-key command.
// extraFlags can include flags like "-r" for repeatable bindings.
func (c *Config) FormatBindKey(key string, action string, extraFlags ...string) string {
	var parts []string
	parts = append(parts, "bind-key")

	// Add extra flags first (e.g., -r for repeatable)
	for _, flag := range extraFlags {
		if flag != "" {
			parts = append(parts, flag)
		}
	}

	// Add bind target
	target := c.BindTarget()
	if target != "" {
		parts = append(parts, target)
	}

	// Add key and action
	parts = append(parts, EscapeKey(key), action)

	return strings.Join(parts, " ")
}

// EscapeCommand escapes a command for embedding in tmux double-quoted strings.
// This escapes double quotes and dollar signs so shell expansion happens at runtime.
func EscapeCommand(cmd string) string {
	// Escape backslashes first, then quotes, then dollar signs
	result := strings.ReplaceAll(cmd, "\\", "\\\\")
	result = strings.ReplaceAll(result, "\"", "\\\"")
	result = strings.ReplaceAll(result, "$", "\\$")
	return result
}

// EscapeKey escapes special characters for tmux config (e.g., backslash).
func EscapeKey(key string) string {
	// Replace any backslash with double backslash for tmux config
	return strings.ReplaceAll(key, "\\", "\\\\")
}
