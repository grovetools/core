package jsontree

import (
	"github.com/charmbracelet/bubbles/key"

	"github.com/grovetools/core/tui/keymap"
)

// KeyMap defines the keybindings for the JSON tree viewer.
type KeyMap struct {
	Up           key.Binding
	Down         key.Binding
	HalfPageUp   key.Binding
	HalfPageDown key.Binding
	GotoTop      key.Binding
	GotoEnd      key.Binding
	Toggle       key.Binding
	Fold         key.Binding
	ExpandAll    key.Binding
	CollapseAll  key.Binding
	Back         key.Binding
	Search       key.Binding
	NextResult   key.Binding
	PrevResult   key.Binding
	YankValue    key.Binding
	YankAll      key.Binding
	VisualMode   key.Binding
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
			key.WithKeys("gg"),
			key.WithHelp("gg", "go to top"),
		),
		GotoEnd: key.NewBinding(
			key.WithKeys("G"),
			key.WithHelp("G", "go to end"),
		),
		Toggle: key.NewBinding(
			key.WithKeys(" ", "enter", "l"),
			key.WithHelp("space/l", "expand"),
		),
		ExpandAll: key.NewBinding(
			key.WithKeys("zR"),
			key.WithHelp("zR", "expand all"),
		),
		CollapseAll: key.NewBinding(
			key.WithKeys("zM"),
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
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		NextResult: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "next result"),
		),
		PrevResult: key.NewBinding(
			key.WithKeys("N"),
			key.WithHelp("N", "prev result"),
		),
		YankValue: key.NewBinding(
			key.WithKeys("y"),
			key.WithHelp("y", "yank value"),
		),
		YankAll: key.NewBinding(
			key.WithKeys("Y"),
			key.WithHelp("Y", "yank all"),
		),
		VisualMode: key.NewBinding(
			key.WithKeys("V"),
			key.WithHelp("V", "visual mode"),
		),
	}
}

// Compile-time guard: KeyMap satisfies the sectioned help/audit contract.
// Value receiver — matches how Sections() is declared and how the component
// passes the keymap to help/audit consumers.
var _ keymap.SectionedKeyMap = KeyMap{}

// Sections returns the grouped keybindings for structured help rendering and
// the keymap-coverage audit. Only keys the component's Update actually handles
// appear here.
func (k KeyMap) Sections() []keymap.Section {
	return []keymap.Section{
		keymap.NavigationSection(k.Up, k.Down, k.HalfPageUp, k.HalfPageDown, k.GotoTop, k.GotoEnd),
		keymap.NewSection("Tree", k.Toggle, k.Fold, k.ExpandAll, k.CollapseAll),
		keymap.SearchSection(k.Search, k.NextResult, k.PrevResult),
		keymap.NewSection("Yank", k.VisualMode, k.YankValue, k.YankAll),
		keymap.SystemSection(k.Back),
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
		{k.Search, k.NextResult, k.PrevResult},
		{k.VisualMode, k.YankValue, k.YankAll},
	}
}
