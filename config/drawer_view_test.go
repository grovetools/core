package config

import (
	"testing"

	"github.com/BurntSushi/toml"
)

// Named views: `view` on a pane declaration is an OPAQUE string the host
// carries to the panel and never interprets, and `digest` is the third value of
// the closed backend enum. The two axes are independent on purpose — a backend
// says what is behind the pane and only the host can add one; a view says which
// of its own renderings the panel should draw and only the panel can name one.

func TestDrawerPaneViewRoundTripsVerbatim(t *testing.T) {
	const src = `
[tui.drawer.panes.breaktimer]
backend = "sidecar"
command = "grove-panel-breaktimer"
view    = "compact"

[tui.drawer.panes.outline]
backend = "sidecar"
command = "outliner"
view    = "tree"

[tui.drawer.panes.notes]
min_width = 20
`
	var cfg Config
	if _, err := toml.Decode(src, &cfg); err != nil {
		t.Fatalf("decode: %v", err)
	}
	panes := cfg.TUI.Drawer.Panes

	// Verbatim, and unrelated to any vocabulary this package holds: "compact"
	// and "tree" are not two members of one scale, they are two panels' own
	// names for two unrelated things.
	if got := panes["breaktimer"].View; got != "compact" {
		t.Errorf("view = %q, want compact", got)
	}
	if got := panes["outline"].View; got != "tree" {
		t.Errorf("view = %q, want tree", got)
	}

	// Absent decodes as empty, which is what every existing declaration and
	// every existing panel relies on: empty means the panel's own default.
	if got := panes["notes"].View; got != "" {
		t.Errorf("an undeclared view decoded as %q, want empty", got)
	}
}

// The plugin half of the same field. A rail entry declares a mounted panel too,
// and which of its layouts to draw is a fact about the panel rather than about
// where it sits.
func TestPluginViewRoundTripsVerbatim(t *testing.T) {
	const src = `
[tui.plugins.breaktimer]
command = "grove-panel-breaktimer"
view    = "full"

[tui.plugins.hello]
command = "grove-panel-hello"
`
	var cfg Config
	if _, err := toml.Decode(src, &cfg); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := cfg.TUI.Plugins["breaktimer"].View; got != "full" {
		t.Errorf("view = %q, want full", got)
	}
	if got := cfg.TUI.Plugins["hello"].View; got != "" {
		t.Errorf("an undeclared view decoded as %q, want empty", got)
	}
}

// digest joins the backend enum and parses, and it is mutually exclusive with
// sidecar: a digest pane spawns nothing, so nothing may read it as a process
// declaration.
func TestDigestBackendParsesAndSpawnsNothing(t *testing.T) {
	const src = `
[tui.drawer.panes.breaktimer]
backend = "digest"

[tui.drawer.panes.probe]
backend = "sidecar"
command = "grove-panel-probe"
`
	var cfg Config
	if _, err := toml.Decode(src, &cfg); err != nil {
		t.Fatalf("decode: %v", err)
	}
	panes := cfg.TUI.Drawer.Panes

	if !panes["breaktimer"].DigestBacked() {
		t.Error(`backend = "digest" did not read as digest-backed`)
	}
	if panes["breaktimer"].SidecarBacked() {
		t.Error("a digest pane read as sidecar-backed — it would be spawned")
	}
	if !panes["probe"].SidecarBacked() || panes["probe"].DigestBacked() {
		t.Error("a sidecar pane read as digest-backed")
	}

	// The default is still in-process in both directions, and nil is neither.
	if (&DrawerPaneConfig{}).DigestBacked() {
		t.Error("an entry with no backend key read as digest-backed")
	}
	var nilCfg *DrawerPaneConfig
	if nilCfg.DigestBacked() {
		t.Error("a nil declaration read as digest-backed")
	}
}

// The view survives a wholesale pane replacement the same way every other field
// does, and a clone does not share it. Cheap to state, and the field would
// otherwise be the one thing a new layer silently dropped.
func TestDrawerPaneViewMergesWithTheDeclaration(t *testing.T) {
	base := &Config{TUI: &TUIConfig{Drawer: &DrawerViewsConfig{
		Panes: map[string]*DrawerPaneConfig{
			"breaktimer": {Backend: DrawerBackendSidecar, Command: "bt", View: "compact"},
		},
	}}}
	got := mergeConfigs(base, &Config{TUI: &TUIConfig{Drawer: &DrawerViewsConfig{
		Panes: map[string]*DrawerPaneConfig{
			"breaktimer": {Backend: DrawerBackendSidecar, Command: "bt", View: "full"},
		},
	}}})
	if v := got.TUI.Drawer.Panes["breaktimer"].View; v != "full" {
		t.Errorf("merged view = %q, want full", v)
	}
	if v := base.TUI.Drawer.Panes["breaktimer"].View; v != "compact" {
		t.Errorf("merge mutated the base layer's view: %q", v)
	}
	if v := cloneDrawerPane(base.TUI.Drawer.Panes["breaktimer"]).View; v != "compact" {
		t.Errorf("clone lost the view: %q", v)
	}
}
