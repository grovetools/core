package cmd

import (
	"github.com/grovetools/core/pkg/keymap"
	corekeymap "github.com/grovetools/core/tui/keymap"
)

// LogsKeymapInfo re-exports the logs TUI keymap info for the registry generator.
// This provides a stable import path from the cmd package.
func LogsKeymapInfo() corekeymap.TUIInfo {
	return keymap.KeymapInfo()
}
