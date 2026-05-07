// Package keymap contains extracted TUI keymaps for registry integration.
package keymap

import (
	"github.com/charmbracelet/bubbles/key"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/tui/keymap"
)

// LogKeyMap defines all key bindings for the logs TUI.
type LogKeyMap struct {
	keymap.Base
	PageUp           key.Binding
	PageDown         key.Binding
	HalfUp           key.Binding
	HalfDown         key.Binding
	GotoTop          key.Binding
	GotoEnd          key.Binding
	Expand           key.Binding
	Search           key.Binding
	Clear            key.Binding
	ToggleFollow     key.Binding
	ToggleFilters    key.Binding
	ViewJSON         key.Binding
	VisualModeStart  key.Binding
	Yank             key.Binding
	SwitchFocus      key.Binding
	ToggleScope      key.Binding
	ToggleSystem     key.Binding
	CycleLevel       key.Binding
	ComponentSummary key.Binding
	ClearBuffer      key.Binding
	CopyRawText      key.Binding
	OpenEditor       key.Binding
}

// NewLogKeyMap creates a new LogKeyMap with user configuration applied.
func NewLogKeyMap(cfg *config.Config) LogKeyMap {
	km := LogKeyMap{
		Base: keymap.Load(cfg, "core.logs"),
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
			key.WithHelp("esc", "clear/back"),
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
			key.WithHelp("y", "yank json"),
		),
		SwitchFocus: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "switch focus"),
		),
		ToggleScope: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "cycle scope"),
		),
		ToggleSystem: key.NewBinding(
			key.WithKeys("S"),
			key.WithHelp("S", "toggle system logs"),
		),
		CycleLevel: key.NewBinding(
			key.WithKeys("v"),
			key.WithHelp("v", "cycle log level"),
		),
		ComponentSummary: key.NewBinding(
			key.WithKeys("C"),
			key.WithHelp("C", "component filter"),
		),
		ClearBuffer: key.NewBinding(
			key.WithKeys("ctrl+l"),
			key.WithHelp("ctrl+l", "clear buffer"),
		),
		CopyRawText: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "copy raw text"),
		),
		OpenEditor: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "open in editor"),
		),
	}

	// Apply TUI-specific overrides from config
	keymap.ApplyTUIOverrides(cfg, "core", "logs", &km)

	return km
}

// ShortHelp returns keybindings to be shown in the mini help view.
func (k LogKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Base.Help, k.Base.Quit, k.ToggleScope, k.CycleLevel, k.ComponentSummary, k.Search, k.ToggleFollow}
}

// FullHelp returns keybindings for the expanded help view.
func (k LogKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{ // Navigation
			k.Base.Up,
			k.Base.Down,
			k.PageUp,
			k.PageDown,
			k.HalfUp,
			k.HalfDown,
			k.GotoTop,
			k.GotoEnd,
		},
		{ // Filters/View
			k.ToggleScope,
			k.ToggleSystem,
			k.CycleLevel,
			k.ComponentSummary,
			k.ToggleFilters,
			k.ToggleFollow,
			k.Search,
		},
		{ // Actions
			k.ViewJSON,
			k.VisualModeStart,
			k.Yank,
			k.CopyRawText,
			k.ClearBuffer,
			k.OpenEditor,
			k.SwitchFocus,
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
		NewLogKeyMap(nil),
	)
}
