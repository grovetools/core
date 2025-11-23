package jsontree

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines the keybindings for the JSON tree viewer.
type KeyMap struct {
	Up          key.Binding
	Down        key.Binding
	HalfPageUp  key.Binding
	HalfPageDown key.Binding
	GotoTop     key.Binding
	GotoEnd     key.Binding
	Toggle      key.Binding
	Fold        key.Binding
	ExpandAll   key.Binding
	CollapseAll key.Binding
	Back        key.Binding
}

// DefaultKeyMap returns the default keybindings for the component.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("k/↑", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("j/↓", "down"),
		),
		HalfPageUp: key.NewBinding(
			key.WithKeys("ctrl+u"),
			key.WithHelp("ctrl+u", "half page up"),
		),
		HalfPageDown: key.NewBinding(
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
		Toggle: key.NewBinding(
			key.WithKeys("space", "enter", "l"),
			key.WithHelp("space/l", "expand"),
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
			key.WithKeys("esc", "q"),
			key.WithHelp("esc/q", "back"),
		),
		Fold: key.NewBinding(
			key.WithKeys("h"),
			key.WithHelp("h", "fold"),
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
