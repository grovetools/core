package navigator

import (
	"github.com/charmbracelet/bubbles/key"
)

// KeyMap defines the keybindings for the navigator
type KeyMap struct {
	// Navigation
	Up   key.Binding
	Down key.Binding
	// Focus management
	FocusEcosystem  key.Binding
	ClearFocus      key.Binding
	ToggleWorktrees key.Binding
	// Help and quit
	Help key.Binding
	Quit key.Binding
}

// defaultKeyMap is the default keymap for the navigator
var defaultKeyMap = KeyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k", "ctrl+p"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j", "ctrl+n"),
		key.WithHelp("↓/j", "down"),
	),
	FocusEcosystem: key.NewBinding(
		key.WithKeys("ctrl+f", "@"),
		key.WithHelp("@/ctrl+f", "focus ecosystem"),
	),
	ClearFocus: key.NewBinding(
		key.WithKeys("ctrl+g"),
		key.WithHelp("ctrl+g", "clear focus"),
	),
	ToggleWorktrees: key.NewBinding(
		key.WithKeys("ctrl+w"),
		key.WithHelp("ctrl+w", "toggle worktrees"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "esc", "ctrl+c"),
		key.WithHelp("q/esc", "quit"),
	),
}

// ShortHelp returns the short help text for the keymap
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Quit}
}

// FullHelp returns the full help text for the keymap
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{
			key.NewBinding(key.WithKeys(""), key.WithHelp("", "Actions")),
			k.Help,
			k.Quit,
		},
	}
}
