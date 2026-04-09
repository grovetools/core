package pager

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/grovetools/core/tui/keymap"
)

// tabBarHeight is the number of vertical rows the pager reserves for
// its tab bar when it deducts from tea.WindowSizeMsg before forwarding
// to pages. One row for the tab labels plus one blank row beneath is
// the same layout nav uses, so all ecosystem TUIs look uniform.
const tabBarHeight = 2

// KeyMap defines the bindings the pager itself consumes. Tab jumps
// 1-9 are the primary navigation; "[" and "]" remain the fallback
// cycle bindings so muscle memory from the previous per-TUI pagers
// keeps working.
//
// The embedding TUI constructs its own keymap on top of keymap.Base
// and passes the relevant subset here; the pager does not try to be
// clever about where the bindings come from.
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

// DefaultKeyMap returns a standalone keymap wired to the standard
// "1"-"9" jump keys and "[" / "]" cycle keys. Hosts that load a
// configured keymap.Base should prefer KeyMapFromBase.
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

// KeyMapFromBase builds a pager KeyMap from a keymap.Base so host
// TUIs can share a single source of truth for user keybinding
// overrides (grove.toml → keymap.Base → pager.KeyMap) instead of
// hard-coding strings.
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

// Model is a Bubble Tea model that owns a slice of Page sub-models
// and routes lifecycle + rendering calls to the active one. It is
// intended to be embedded inside a host TUI's model, which keeps
// its own Update loop and delegates to Model.Update for anything
// it doesn't want to intercept first.
type Model struct {
	pages      []Page
	activePage int
	keys       KeyMap
	width      int
	height     int
}

// New constructs a pager Model around the supplied pages. At least
// one page is required; callers that need a conditionally-populated
// tab list should filter before calling. The first page is active on
// construction.
func New(pages []Page, keys KeyMap) Model {
	return Model{
		pages:      pages,
		activePage: 0,
		keys:       keys,
	}
}

// NewAt is like New but lets the caller pick the initial active page.
// Out-of-range indices are silently clamped to the valid range; no
// Focus/Blur lifecycle is run (the host is expected to return the
// initial page's Init/Focus command separately during its own Init).
func NewAt(pages []Page, keys KeyMap, active int) Model {
	if active < 0 {
		active = 0
	}
	if len(pages) > 0 && active >= len(pages) {
		active = len(pages) - 1
	}
	return Model{
		pages:      pages,
		activePage: active,
		keys:       keys,
	}
}

// Init runs the active page's Init command. Other pages are not
// initialized eagerly — they are built by the host and assumed to be
// inert until focused. Hosts that want eager multi-page init can
// batch commands themselves before handing ownership to the pager.
func (m Model) Init() tea.Cmd {
	if len(m.pages) == 0 {
		return nil
	}
	return m.pages[m.activePage].Init()
}

// Active returns the currently active Page. Useful for hosts that
// need to run page-specific logic outside the message dispatcher
// (e.g. inspecting the active tab name to decide help text).
func (m Model) Active() Page {
	if len(m.pages) == 0 {
		return nil
	}
	return m.pages[m.activePage]
}

// ActiveIndex returns the index of the active tab.
func (m Model) ActiveIndex() int { return m.activePage }

// Pages returns the backing page slice so hosts can inspect it. The
// returned slice aliases the pager's internal storage — do not mutate.
func (m Model) Pages() []Page { return m.pages }

// Size returns the last width/height the pager saw via WindowSizeMsg.
// Hosts that build sub-models lazily can consult this to size a
// fresh page before forwarding the first WindowSizeMsg.
func (m Model) Size() (int, int) { return m.width, m.height }
