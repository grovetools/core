package widget

import "github.com/grovetools/core/tui/embed"

// SetWorkspaceMsg is the workspace-change intake. It is the host's existing
// embed message rather than a drawer-specific one: a widget package that
// already handles it as a full-screen embedded TUI must not need a second code
// path to handle it as a drawer pane.
//
// Aliased here so the whole intake vocabulary can be read in one place; the
// canonical definition stays in the embed contract.
type SetWorkspaceMsg = embed.SetWorkspaceMsg

// UpdateMsg tells widgets that the host's tracker state changed and they should
// re-derive whatever they render from it. It carries no payload on purpose: it
// is a "look again" tick, and a widget reads the tracker it was built with.
//
// It is broadcast on the host's own cadence, which is neither per-frame nor
// bounded, so handling it must be cheap — a re-render, not a fetch.
type UpdateMsg struct{}

// VisibilityMsg tells widgets whether the drawer is currently expanded.
//
// A collapsed drawer keeps its page MOUNTED — widgets stay alive and keep
// receiving every broadcast — so a widget that refreshes off background signals
// needs this to avoid paying for data nobody can see. The honest handling is to
// record the state, remember that a signal was missed, and reconcile on the
// next reveal; going silent without remembering shows yesterday's data when the
// drawer comes back.
//
// The host emits it on every layout change, which covers expand/collapse and
// page switches alike.
type VisibilityMsg struct {
	Expanded bool
}

// LeaveFocusMsg asks the host to hand keyboard focus back to the main area,
// leaving the drawer mounted and expanded.
//
// It is what a widget emits for its "I'm done here" key (conventionally q/esc)
// instead of the embed.CloseRequestMsg a full-screen embedding would send: a
// drawer pane has no window of its own to close, so tearing anything down would
// be wrong. A widget package that embeds an existing full-screen model
// translates that model's close request into this on the way out.
type LeaveFocusMsg struct{}
