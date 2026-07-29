// Package panelkit is the Go SDK for building a grove panel.
//
// A panel is a pane of UI the terminal multiplexer hosts. It can be in-tree —
// compiled into the binary, implementing tuimux.Panel — or out-of-process, a
// sidecar speaking embed/v1 over a socket. The point of this SDK is that both
// are the same program: a bubbletea app made of pager.Pages and the widgets
// below, differing only in what feeds it messages.
//
// # What is in the kit
//
// The kit is two things. First, a curated set of core/tui packages that were
// decoupled from the host so a program outside a grove binary can link them:
//
//	core/tui/theme                    palettes, styles, icons
//	core/tui/theme/themecfg           the seam that supplies a theme selection
//	core/tui/keymap                   presets, overrides, chords, which-key
//	core/tui/components/help          the help overlay, from any sectioned keymap
//	core/tui/components/whichkey      the chord popup
//	core/tui/components/table         styled and selectable tables
//	core/tui/components/pager         the tab drawer, pager.Page, Wrap
//	core/tui/hostedkeys               the hosted-key claim/grant shapes
//
// Second, the widgets under this directory, promoted from the best existing
// implementation of a pattern the ecosystem had re-derived many times over:
//
//	panelkit/window                   scroll/cursor viewport arithmetic
//	panelkit/table                    responsive priority-drop column fitting
//	panelkit/tree                     fold state and flat-list tree prefixes
//	panelkit/layout                   small composition helpers
//	panelkit/sidecar                  the out-of-process runtime and its
//	                                  bubbletea adapter
//
// # The sidecar-clean guarantee
//
// Nothing in the first list, and nothing under this directory except
// panelkit/sidecar, may reach core/config, core/pkg/daemon, core/pkg/workspace
// or core/pkg/plan. That is what makes the kit linkable from a program that is
// not a grove binary and has no grove.toml to read. TestKitIsSidecarClean in
// this package holds the line; deps_test.go names the exact forbidden set.
//
// Configuration reaches the kit through seams instead of imports. theme takes a
// selection from themecfg (core/config registers one in-tree; a sidecar
// registers the host's theme tokens). keymap takes a KeybindingSource
// (*config.Config implements it in-tree; a sidecar can implement it over
// delivered settings, or pass nil for the vim defaults).
package panelkit
