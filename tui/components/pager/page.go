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

// PageWithFooter is an optional extension a Page can implement to
// supply a footer string that the pager pins below the body. When the
// active page implements this interface, the pager calls Footer() in
// View() and renders the result in the footer slot, overriding any
// previously set footer via SetFooter(). Pages that don't implement
// this interface leave the footer unchanged.
type PageWithFooter interface {
	Page
	Footer() string
}

// PageWithFooterHeight is an optional extension a Page can implement to
// override Config.FooterHeight for itself. Config.FooterHeight is a
// single reservation shared by every tab, so it has to be sized for the
// tallest footer in the set; a page that renders its own footer inside
// its body (or none at all) would otherwise lose those rows to blank
// space. Returning 0 hands the whole reservation back to the body.
// Negative values are treated as 0.
type PageWithFooterHeight interface {
	Page
	FooterHeight() int
}

// PageWithMinSize is an optional extension a Page can implement to declare the
// smallest body it can draw anything meaningful in. Below either dimension the
// pager renders a placeholder instead of calling View().
//
// It exists because there was no min-width contract anywhere in the ecosystem,
// and the failure mode without one is bad in a specific way: a table or tree
// squeezed under its usable width does not fail visibly, it renders a column
// of punctuation that looks like a rendering bug. A page that says "I need 40
// columns" gets told, in words, that the pane is too narrow — which is
// actionable, where a mangled frame is not.
//
// Declaring a minimum is a last resort, not a first one. A page that can
// reflow — dropping columns, flipping a split from side-by-side to stacked —
// should do that first and declare a minimum only for the size below which
// even the degraded layout is meaningless. Zero or negative values mean "no
// minimum in that dimension".
type PageWithMinSize interface {
	Page
	MinSize() (width, height int)
}
