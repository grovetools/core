// Package keymap contains extracted TUI keymaps for registry integration.
package keymap

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/grovetools/core/tui/keymap"
)

// LogKeyMap defines all key bindings for the logs TUI.
type LogKeyMap struct {
	keymap.Base
	PageUp          key.Binding
	PageDown        key.Binding
	HalfUp          key.Binding
	HalfDown        key.Binding
	GotoTop         key.Binding
	GotoEnd         key.Binding
	Expand          key.Binding
	Search          key.Binding
	Clear           key.Binding
	ToggleFollow    key.Binding
	ToggleFilters   key.Binding
	ViewJSON        key.Binding
	VisualModeStart key.Binding
	Yank            key.Binding
	SwitchFocus     key.Binding
}

// NewLogKeyMap creates a new LogKeyMap with default bindings.
func NewLogKeyMap() LogKeyMap {
	return LogKeyMap{
		Base: keymap.NewBase(),
		PageUp: key.NewBinding(
			key.WithKeys("pgup"),
			key.WithHelp("pgup", "page up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown"),
			key.WithHelp("pgdn", "page down"),
		),
		HalfUp: key.NewBinding(
			key.WithKeys("ctrl+u"),
			key.WithHelp("ctrl+u", "half page up"),
		),
		HalfDown: key.NewBinding(
			key.WithKeys("ctrl+d"),
			key.WithHelp("ctrl+d", "half page down"),
		),
		GotoTop: key.NewBinding(
			key.WithKeys("g"),
			key.WithHelp("gg", "go to top"),
		),
		GotoEnd: key.NewBinding(
			key.WithKeys("G"),
			key.WithHelp("G", "go to end"),
		),
		Expand: key.NewBinding(
			key.WithKeys(" ", "enter"),
			key.WithHelp("space/enter", "expand/collapse"),
		),
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		Clear: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "clear search"),
		),
		ToggleFollow: key.NewBinding(
			key.WithKeys("F"),
			key.WithHelp("F", "toggle follow"),
		),
		ToggleFilters: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "toggle filters"),
		),
		ViewJSON: key.NewBinding(
			key.WithKeys("J"),
			key.WithHelp("J", "view json"),
		),
		VisualModeStart: key.NewBinding(
			key.WithKeys("V"),
			key.WithHelp("V", "visual line mode"),
		),
		Yank: key.NewBinding(
			key.WithKeys("y"),
			key.WithHelp("y", "yank selection"),
		),
		SwitchFocus: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "switch focus"),
		),
	}
}

// ShortHelp returns keybindings to be shown in the mini help view.
func (k LogKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Base.Help, k.Base.Quit, k.ToggleFollow, k.ToggleFilters, k.Search}
}

// FullHelp returns keybindings for the expanded help view.
func (k LogKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{ // Navigation column
			k.Base.Up,
			k.Base.Down,
			k.PageUp,
			k.PageDown,
			k.HalfUp,
			k.HalfDown,
			k.GotoTop,
			k.GotoEnd,
		},
		{ // Actions column
			k.SwitchFocus,
			k.ToggleFollow,
			k.ToggleFilters,
			k.Search,
			k.ViewJSON,
			k.VisualModeStart,
			k.Yank,
			k.Base.Help,
			k.Base.Quit,
		},
	}
}

// KeymapInfo returns the keymap metadata for the logs TUI.
// Used by the grove keys registry generator to aggregate all TUI keybindings.
func KeymapInfo() keymap.TUIInfo {
	return keymap.MakeTUIInfo(
		"core-logs",
		"core",
		"Aggregated log viewer with filtering and search",
		NewLogKeyMap(),
	)
}
