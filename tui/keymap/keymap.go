package keymap

import (
	"github.com/charmbracelet/bubbles/key"
)

// Base contains the standard keybindings used across all Grove TUIs
// Prioritizes vim-style navigation and standard actions
type Base struct {
	// Navigation - vim style takes precedence
	Up       key.Binding
	Down     key.Binding
	Left     key.Binding
	Right    key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	Home     key.Binding
	End      key.Binding

	// Core actions
	Quit    key.Binding
	Help    key.Binding
	Confirm key.Binding
	Cancel  key.Binding
	Back    key.Binding

	// Search
	Search       key.Binding
	SearchNext   key.Binding
	SearchPrev   key.Binding
	ClearSearch  key.Binding

	// View management
	SwitchView   key.Binding
	NextTab      key.Binding
	PrevTab      key.Binding
	FocusNext    key.Binding
	FocusPrev    key.Binding

	// Selection
	Select       key.Binding
	SelectAll    key.Binding
	SelectNone   key.Binding
	ToggleSelect key.Binding
}

// NewBase creates a new Base keymap with default Grove keybindings
func NewBase() Base {
	return Base{
		// Navigation - vim style takes precedence
		Up: key.NewBinding(
			key.WithKeys("k", "up"),
			key.WithHelp("k/↑", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("j", "down"),
			key.WithHelp("j/↓", "down"),
		),
		Left: key.NewBinding(
			key.WithKeys("h", "left"),
			key.WithHelp("h/←", "left"),
		),
		Right: key.NewBinding(
			key.WithKeys("l", "right"),
			key.WithHelp("l/→", "right"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("b", "pgup", "ctrl+b"),
			key.WithHelp("b/PgUp", "page up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("f", "pgdown", "ctrl+f"),
			key.WithHelp("f/PgDn", "page down"),
		),
		Home: key.NewBinding(
			key.WithKeys("g", "home", "ctrl+a"),
			key.WithHelp("g/Home", "go to start"),
		),
		End: key.NewBinding(
			key.WithKeys("G", "end", "ctrl+e"),
			key.WithHelp("G/End", "go to end"),
		),

		// Core actions
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q/ctrl+c", "quit"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Confirm: key.NewBinding(
			key.WithKeys("enter", "y"),
			key.WithHelp("enter/y", "confirm"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("n", "ctrl+g"),
			key.WithHelp("n/ctrl+g", "cancel"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),

		// Search - '/' initiates search as per vim convention
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		SearchNext: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "next match"),
		),
		SearchPrev: key.NewBinding(
			key.WithKeys("N", "shift+n"),
			key.WithHelp("N", "prev match"),
		),
		ClearSearch: key.NewBinding(
			key.WithKeys("ctrl+l"),
			key.WithHelp("ctrl+l", "clear search"),
		),

		// View management - tab for switching views/components
		SwitchView: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "switch view"),
		),
		NextTab: key.NewBinding(
			key.WithKeys("}", "]"),
			key.WithHelp("}/]", "next tab"),
		),
		PrevTab: key.NewBinding(
			key.WithKeys("{", "["),
			key.WithHelp("{/[", "prev tab"),
		),
		FocusNext: key.NewBinding(
			key.WithKeys("ctrl+n"),
			key.WithHelp("ctrl+n", "focus next"),
		),
		FocusPrev: key.NewBinding(
			key.WithKeys("ctrl+p"),
			key.WithHelp("ctrl+p", "focus prev"),
		),

		// Selection
		Select: key.NewBinding(
			key.WithKeys("space", "x"),
			key.WithHelp("space/x", "select"),
		),
		SelectAll: key.NewBinding(
			key.WithKeys("a", "ctrl+a"),
			key.WithHelp("a", "select all"),
		),
		SelectNone: key.NewBinding(
			key.WithKeys("A", "ctrl+shift+a"),
			key.WithHelp("A", "select none"),
		),
		ToggleSelect: key.NewBinding(
			key.WithKeys("v"),
			key.WithHelp("v", "toggle select"),
		),
	}
}

// ShortHelp returns a slice of key bindings for the short help view
func (k Base) ShortHelp() []key.Binding {
	return []key.Binding{
		k.Quit,
	}
}

// FullHelp returns a slice of all key bindings for the full help view
func (k Base) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		// Navigation
		{k.Up, k.Down, k.Left, k.Right},
		{k.PageUp, k.PageDown, k.Home, k.End},
		// Actions
		{k.Confirm, k.Cancel, k.Back},
		// Search
		{k.Search, k.SearchNext, k.SearchPrev, k.ClearSearch},
		// View
		{k.SwitchView, k.NextTab, k.PrevTab},
		// Core
		{k.Help, k.Quit},
	}
}

// DefaultKeyMap is the default keymap instance for the Grove ecosystem
var DefaultKeyMap = NewBase()

// Extended keymaps for specific use cases

// ListKeyMap extends Base with list-specific bindings
type ListKeyMap struct {
	Base
	Edit   key.Binding
	Delete key.Binding
	Copy   key.Binding
	Paste  key.Binding
}

// NewListKeyMap creates a new list-specific keymap
func NewListKeyMap() ListKeyMap {
	return ListKeyMap{
		Base: NewBase(),
		Edit: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "edit"),
		),
		Delete: key.NewBinding(
			key.WithKeys("d", "delete"),
			key.WithHelp("d", "delete"),
		),
		Copy: key.NewBinding(
			key.WithKeys("y"),
			key.WithHelp("y", "copy/yank"),
		),
		Paste: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "paste"),
		),
	}
}

// FormKeyMap extends Base with form-specific bindings
type FormKeyMap struct {
	Base
	NextField key.Binding
	PrevField key.Binding
	Submit    key.Binding
	Reset     key.Binding
}

// NewFormKeyMap creates a new form-specific keymap
func NewFormKeyMap() FormKeyMap {
	return FormKeyMap{
		Base: NewBase(),
		NextField: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next field"),
		),
		PrevField: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "prev field"),
		),
		Submit: key.NewBinding(
			key.WithKeys("ctrl+s", "ctrl+enter"),
			key.WithHelp("ctrl+s", "submit"),
		),
		Reset: key.NewBinding(
			key.WithKeys("ctrl+r"),
			key.WithHelp("ctrl+r", "reset"),
		),
	}
}

// TreeKeyMap extends Base with tree-specific bindings
type TreeKeyMap struct {
	Base
	Expand   key.Binding
	Collapse key.Binding
	Toggle   key.Binding
}

// NewTreeKeyMap creates a new tree-specific keymap
func NewTreeKeyMap() TreeKeyMap {
	return TreeKeyMap{
		Base: NewBase(),
		Expand: key.NewBinding(
			key.WithKeys("o", "right"),
			key.WithHelp("o/→", "expand"),
		),
		Collapse: key.NewBinding(
			key.WithKeys("c", "left"),
			key.WithHelp("c/←", "collapse"),
		),
		Toggle: key.NewBinding(
			key.WithKeys("space", "enter"),
			key.WithHelp("space", "toggle"),
		),
	}
}