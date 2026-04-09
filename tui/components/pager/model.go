package pager

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/grovetools/core/tui/keymap"
)

// tabBarHeight is the row count the pager reserves above the active
// page (1 bar row + 1 blank).
const tabBarHeight = 2

// KeyMap is the bindings the pager consumes: 1-9 jumps + [/] cycle.
type KeyMap struct {
	Tab1    key.Binding
	Tab2    key.Binding
	Tab3    key.Binding
	Tab4    key.Binding
	Tab5    key.Binding
	Tab6    key.Binding
	Tab7    key.Binding
	Tab8    key.Binding
	Tab9    key.Binding
	NextTab key.Binding
	PrevTab key.Binding
}

// DefaultKeyMap returns the standard "1".."9" + "[" / "]" bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Tab1:    key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "tab 1")),
		Tab2:    key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "tab 2")),
		Tab3:    key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "tab 3")),
		Tab4:    key.NewBinding(key.WithKeys("4"), key.WithHelp("4", "tab 4")),
		Tab5:    key.NewBinding(key.WithKeys("5"), key.WithHelp("5", "tab 5")),
		Tab6:    key.NewBinding(key.WithKeys("6"), key.WithHelp("6", "tab 6")),
		Tab7:    key.NewBinding(key.WithKeys("7"), key.WithHelp("7", "tab 7")),
		Tab8:    key.NewBinding(key.WithKeys("8"), key.WithHelp("8", "tab 8")),
		Tab9:    key.NewBinding(key.WithKeys("9"), key.WithHelp("9", "tab 9")),
		NextTab: key.NewBinding(key.WithKeys("]"), key.WithHelp("]", "next tab")),
		PrevTab: key.NewBinding(key.WithKeys("["), key.WithHelp("[", "prev tab")),
	}
}

// KeyMapFromBase pulls a pager KeyMap out of a configured keymap.Base
// so user grove.toml overrides flow through.
func KeyMapFromBase(b keymap.Base) KeyMap {
	return KeyMap{
		Tab1:    b.Tab1,
		Tab2:    b.Tab2,
		Tab3:    b.Tab3,
		Tab4:    b.Tab4,
		Tab5:    b.Tab5,
		Tab6:    b.Tab6,
		Tab7:    b.Tab7,
		Tab8:    b.Tab8,
		Tab9:    b.Tab9,
		NextTab: b.NextTab,
		PrevTab: b.PrevTab,
	}
}

// Model owns a slice of Pages and routes lifecycle / rendering to
// the active one. Embed inside a host model and delegate via
// Model.Update for any messages the host doesn't intercept first.
type Model struct {
	pages      []Page
	activePage int
	keys       KeyMap
	width      int
	height     int
}

// New constructs a pager with the first page active.
func New(pages []Page, keys KeyMap) Model {
	return Model{pages: pages, keys: keys}
}

// NewAt constructs a pager with a specific initial active page.
// Out-of-range indices are clamped.
func NewAt(pages []Page, keys KeyMap, active int) Model {
	if active < 0 {
		active = 0
	}
	if len(pages) > 0 && active >= len(pages) {
		active = len(pages) - 1
	}
	return Model{pages: pages, activePage: active, keys: keys}
}

// Init runs the active page's Init.
func (m Model) Init() tea.Cmd {
	if len(m.pages) == 0 {
		return nil
	}
	return m.pages[m.activePage].Init()
}

// Active returns the active Page (or nil).
func (m Model) Active() Page {
	if len(m.pages) == 0 {
		return nil
	}
	return m.pages[m.activePage]
}

// ActiveIndex returns the active tab index.
func (m Model) ActiveIndex() int { return m.activePage }

// Pages returns the backing slice (do not mutate).
func (m Model) Pages() []Page { return m.pages }

// Size returns the last (width, height) the pager saw.
func (m Model) Size() (int, int) { return m.width, m.height }
