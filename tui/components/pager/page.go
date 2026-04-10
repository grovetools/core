// Package pager provides a reusable tabbed meta-panel component:
// active-tab state, tab bar rendering, numeric 1-9 jump keys, and
// cross-page auto-switch via embed.SwitchTabMsg.
package pager

import tea "github.com/charmbracelet/bubbletea"

// Page is one tab of a pager.
type Page interface {
	Name() string
	Init() tea.Cmd
	Update(tea.Msg) (Page, tea.Cmd)
	View() string
	Focus() tea.Cmd
	Blur()
	SetSize(width, height int)
}

// PageWithTitle is an optional extension a Page can implement to
// provide a bold title row rendered above its body. If the pager is
// configured with ShowTitleRow=true, a page that returns a non-empty
// Title() gets that string rendered; a page without the interface (or
// returning "") renders a blank line so vertical geometry stays
// constant across tab switches.
type PageWithTitle interface {
	Page
	Title() string
}

// PageWithEnabled is an optional extension indicating whether a page
// is currently switchable. Disabled pages are dimmed in the tab bar
// and skipped by numeric jumps and [/] cycling.
type PageWithEnabled interface {
	Page
	Enabled() bool
}

// PageWithReady is an optional extension for async-loading pages. A
// page that returns (false, "Loading wizards…") causes the pager to
// render a centered loading placeholder instead of calling View().
type PageWithReady interface {
	Page
	Ready() (ready bool, loadingMsg string)
}

// PageWithID is an optional extension a Page can implement to expose a
// stable, human-readable identifier (e.g. "stats", "jobs", "browser").
// The pager uses this to resolve embed.SwitchTabMsg.TabID lookups so
// deep-link navigation can target tabs by name instead of brittle
// positional indices.
type PageWithID interface {
	Page
	TabID() string
}

// PageWithTextInput is an optional extension a Page can implement to
// signal when a focused text input should absorb keys that would
// otherwise drive pager navigation. When IsTextEntryActive() returns
// true, the pager does not intercept numeric tab jumps or [/] cycle
// keys — they fall through to the page's own Update so characters
// land in the input field.
type PageWithTextInput interface {
	Page
	IsTextEntryActive() bool
}
