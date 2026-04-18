package pager

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/grovetools/core/tui/keymap"
)

// tabBarHeight is the row count the pager reserves above the active
// page: 1 row for the tabs themselves + 1 blank spacer row. Kept as
// a package-local const so ChromeRows() can add it to optional chrome
// without every host re-deriving the magic number.
const tabBarHeight = 2

// Config controls the optional layout chrome the pager accounts for
// when sizing pages and rendering the final view. Zero values
// reproduce the legacy behavior (no padding, no title row, no footer
// reservation) so existing New()/NewAt() call sites keep working.
type Config struct {
	// OuterPadding is top/right/bottom/left padding applied to the
	// entire pager view. Expressed as 4 ints (not a lipgloss.Style)
	// so hosts can't accidentally leak border/foreground styles in.
	OuterPadding [4]int
	// ShowTitleRow reserves one row below the tab bar for an
	// optional page title. When true, the row is always present
	// even if the active page doesn't implement PageWithTitle — a
	// blank spacer is rendered so vertical geometry stays constant
	// across tab switches.
	ShowTitleRow bool
	// FooterHeight reserves N rows at the bottom of the pager area
	// for host-rendered content (help text, status lines, etc.).
	// The pager renders nothing in the footer slot itself — the
	// host composes its own footer below pager.View() — but the
	// reserved rows are subtracted from SubSize so sub-models size
	// their bodies correctly.
	FooterHeight int
}

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
	cfg        Config
	width      int
	height     int
	footer     string // pre-rendered footer pinned below the body
}

// New constructs a pager with the first page active and zero-value
// Config (legacy no-chrome behavior).
func New(pages []Page, keys KeyMap) Model {
	return NewWith(pages, keys, Config{})
}

// NewWith constructs a pager with an explicit Config. Use this when
// the host wants outer padding, a reserved title row, or a footer
// slot accounted for in page sizing.
func NewWith(pages []Page, keys KeyMap, cfg Config) Model {
	return Model{pages: pages, keys: keys, cfg: cfg}
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

// SetConfig replaces the pager's layout config at runtime. Useful
// when the host's footer height is dynamic (e.g. help text that
// wraps at narrow widths). Uses a pointer receiver so the caller can
// mutate a field Model directly.
func (m *Model) SetConfig(cfg Config) {
	m.cfg = cfg
}

// Config returns the current layout config.
func (m Model) Config() Config { return m.cfg }

// ChromeRows returns the total vertical space the pager reserves for
// chrome: tab bar + optional title row + footer slot + top/bottom
// outer padding. Hosts should subtract this from their available
// height when forwarding a WindowSizeMsg.
func (m Model) ChromeRows() int {
	rows := tabBarHeight
	if m.cfg.ShowTitleRow {
		rows++
	}
	rows += m.cfg.FooterHeight
	rows += m.cfg.OuterPadding[0] + m.cfg.OuterPadding[2]
	return rows
}

// ChromeCols returns the horizontal space consumed by outer padding.
func (m Model) ChromeCols() int {
	return m.cfg.OuterPadding[1] + m.cfg.OuterPadding[3]
}

// SubSize computes the WindowSizeMsg that should be passed down to
// sub-models so their bodies fit inside the pager's body region
// after all chrome is subtracted. Bounds are clamped to >= 1.
func (m Model) SubSize(termW, termH int) tea.WindowSizeMsg {
	w := termW - m.ChromeCols()
	h := termH - m.ChromeRows()
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return tea.WindowSizeMsg{Width: w, Height: h}
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

// SetActive programmatically switches to the page at idx, respecting the
// usual Blur/Focus hooks so the target page can initialise itself. Returns
// any tea.Cmd emitted by the new page's Focus(). Out-of-range and unchanged
// indices are no-ops.
func (m *Model) SetActive(idx int) tea.Cmd {
	if idx < 0 || idx >= len(m.pages) || idx == m.activePage {
		return nil
	}
	m.pages[m.activePage].Blur()
	m.activePage = idx
	return m.pages[m.activePage].Focus()
}

// Pages returns the backing slice (do not mutate).
func (m Model) Pages() []Page { return m.pages }

// SetFooter replaces the pager's pinned footer string. The pager
// renders this below the body in View() and force-expands the body to
// fill the remaining vertical space, pinning the footer to the bottom.
// Pass "" to clear.
func (m *Model) SetFooter(s string) { m.footer = s }

// Footer returns the current footer string.
func (m Model) Footer() string { return m.footer }

// Size returns the last (width, height) the pager saw.
func (m Model) Size() (int, int) { return m.width, m.height }
