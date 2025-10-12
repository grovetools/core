package wsnav

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines the keybindings for the workspace navigator TUI.
type KeyMap struct {
	Up              key.Binding
	Down            key.Binding
	PageUp          key.Binding
	PageDown        key.Binding
	Top             key.Binding
	Bottom          key.Binding
	Help            key.Binding
	Quit            key.Binding
	Search          key.Binding
	Focus           key.Binding
	ClearFocus      key.Binding
	ToggleWorktrees key.Binding
}

// DefaultKeyMap is the default set of keybindings.
var DefaultKeyMap = KeyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	PageUp: key.NewBinding(
		key.WithKeys("pgup", "ctrl+u"),
		key.WithHelp("pgup/ctrl+u", "page up"),
	),
	PageDown: key.NewBinding(
		key.WithKeys("pgdown", "ctrl+d"),
		key.WithHelp("pgdn/ctrl+d", "page down"),
	),
	Top: key.NewBinding(
		key.WithKeys("g"),
		key.WithHelp("gg", "jump to top"),
	),
	Bottom: key.NewBinding(
		key.WithKeys("G"),
		key.WithHelp("G", "jump to bottom"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "esc", "ctrl+c"),
		key.WithHelp("q/esc", "quit"),
	),
	Search: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "filter"),
	),
	Focus: key.NewBinding(
		key.WithKeys("ctrl+f"),
		key.WithHelp("ctrl+f", "focus"),
	),
	ClearFocus: key.NewBinding(
		key.WithKeys("ctrl+g"),
		key.WithHelp("ctrl+g", "clear focus"),
	),
	ToggleWorktrees: key.NewBinding(
		key.WithKeys("w"),
		key.WithHelp("w", "toggle worktrees"),
	),
}

// ShortHelp returns keybindings to be shown in the compact help view.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Help, k.Quit}
}

// FullHelp returns keybindings for the full help view.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown},
		{k.Top, k.Bottom, k.Search, k.Focus},
		{k.ClearFocus, k.ToggleWorktrees, k.Help, k.Quit},
	}
}
