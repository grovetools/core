// Package themecfg is the seam between core/tui/theme and whatever supplies
// the non-environment half of a theme selection.
//
// In-tree that supplier is grove.toml (core/config registers a resolver in its
// package init). Out-of-tree — a sidecar panel, a plugin, an editor bridge —
// it is whatever the host hands over, typically the theme tokens delivered on
// the embed/v1 wire. Keeping the seam in its own stdlib-only package is what
// lets core/tui/theme stay free of core/config: theme imports themecfg,
// core/config imports themecfg, and neither imports the other.
//
// Environment precedence is not this package's business. GROVE_THEME and
// GROVE_ICONS are read by core/tui/theme and win over whatever a resolver
// reports; a resolver only fills in what the environment left unset.
package themecfg

import "sync"

// Selection is the theme and icon choice a Resolver reports. An empty field
// means "not configured here" and leaves the consumer's own default in place —
// it does not mean "reset to default".
type Selection struct {
	// Theme is a theme name as understood by theme.SetTheme, e.g. "kanagawa".
	Theme string
	// Icons is an icon mode: "ascii" selects the ASCII-safe set, anything
	// else (including "nerd") selects the Nerd Font set.
	Icons string
}

// Resolver reports the current selection. It is called lazily, so an
// implementation is free to do real work such as reading a config file.
type Resolver func() Selection

var (
	mu        sync.Mutex
	resolver  Resolver
	listeners []func(Selection)
)

// SetResolver installs the selection source, replacing any previous one, and
// notifies every registered listener with the new selection.
//
// The notification exists because package initialisation order between
// core/tui/theme and the package that registers the resolver is unspecified:
// when theme initialises first it applies environment-plus-default, and this
// call is what replays the configured selection over it.
func SetResolver(r Resolver) {
	mu.Lock()
	resolver = r
	subscribers := append([]func(Selection){}, listeners...)
	mu.Unlock()

	sel := Selection{}
	if r != nil {
		sel = r()
	}
	for _, fn := range subscribers {
		fn(sel)
	}
}

// Resolve returns the current selection, or the zero Selection when no
// resolver is installed.
func Resolve() Selection {
	mu.Lock()
	r := resolver
	mu.Unlock()
	if r == nil {
		return Selection{}
	}
	return r()
}

// OnResolve registers fn to be called whenever a resolver is installed. It
// does not fire for a resolver installed earlier — callers that need the
// current value at registration time should call Resolve themselves.
func OnResolve(fn func(Selection)) {
	if fn == nil {
		return
	}
	mu.Lock()
	listeners = append(listeners, fn)
	mu.Unlock()
}
