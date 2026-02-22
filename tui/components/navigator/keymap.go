package navigator

import (
	"github.com/charmbracelet/bubbles/key"
)

// KeyMap defines the keybindings for the navigator
type KeyMap struct {
	// Navigation
	Up       key.Binding
	Down     key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	GotoTop  key.Binding
	GotoEnd  key.Binding
	// Selection
	Select key.Binding
	Search key.Binding
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
	PageUp: key.NewBinding(
		key.WithKeys("ctrl+u"),
		key.WithHelp("ctrl+u", "page up"),
	),
	PageDown: key.NewBinding(
		key.WithKeys("ctrl+d"),
		key.WithHelp("ctrl+d", "page down"),
	),
	GotoTop: key.NewBinding(
		key.WithKeys("g"),
		key.WithHelp("g", "go to top"),
	),
	GotoEnd: key.NewBinding(
		key.WithKeys("G"),
		key.WithHelp("G", "go to end"),
	),
	Select: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select"),
	),
	Search: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "search"),
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
