package jsontree

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines the keybindings for the JSON tree viewer.
type KeyMap struct {
	Up          key.Binding
	Down        key.Binding
	Toggle      key.Binding
	ExpandAll   key.Binding
	CollapseAll key.Binding
	Back        key.Binding
}

// DefaultKeyMap returns the default keybindings for the component.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("k/up", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("j/down", "down"),
		),
		Toggle: key.NewBinding(
			key.WithKeys("space", "enter", "l"),
			key.WithHelp("space/enter", "toggle fold"),
		),
		ExpandAll: key.NewBinding(
			key.WithKeys("z"),
			key.WithHelp("zR", "expand all"),
		),
		CollapseAll: key.NewBinding(
			key.WithKeys("z"),
			key.WithHelp("zM", "collapse all"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc", "q", "h"),
			key.WithHelp("esc/q", "back"),
		),
	}
}

// ShortHelp returns the short help bindings.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Toggle, k.Back}
}

// FullHelp returns the full help bindings.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Toggle},
		{k.ExpandAll, k.CollapseAll, k.Back},
	}
}
