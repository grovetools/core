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

// The AUTHOR's half of the same field. `view` is what the user asks for;
// [[...views]] is what the panel says exists — copied out of the manifest by
// `grove plugin install`, in declaration order, because order is the author's
// preference order and a table of tables has none.
func TestDeclaredViewsRoundTripInDeclarationOrder(t *testing.T) {
	const src = `
[tui.drawer.panes.breaktimer]
backend = "sidecar"
command = "grove-panel-breaktimer"

[[tui.drawer.panes.breaktimer.views]]
name        = "full"
description = "clock, history and help"
drawer      = false

[[tui.drawer.panes.breaktimer.views]]
name        = "compact"
description = "one line: state and time remaining"
drawer      = true

[tui.plugins.breaktimer]
command = "grove-panel-breaktimer"

[[tui.plugins.breaktimer.views]]
name   = "full"
drawer = false
`
	var cfg Config
	if _, err := toml.Decode(src, &cfg); err != nil {
		t.Fatalf("decode: %v", err)
	}
	pane := cfg.TUI.Drawer.Panes["breaktimer"]
	if len(pane.Views) != 2 {
		t.Fatalf("declared views = %#v, want 2", pane.Views)
	}
	// Order is the whole reason this is an array. `full` is declared first, and
	// it stays first.
	if pane.Views[0].Name != "full" || pane.Views[1].Name != "compact" {
		t.Errorf("declaration order = %q, %q, want full, compact", pane.Views[0].Name, pane.Views[1].Name)
	}
	if pane.Views[0].Drawer || !pane.Views[1].Drawer {
		t.Errorf("drawer suitability = %v, %v, want false, true", pane.Views[0].Drawer, pane.Views[1].Drawer)
	}
	if pane.Views[1].Description != "one line: state and time remaining" {
		t.Errorf("description = %q", pane.Views[1].Description)
	}
	// The rail carries the same declaration. Nothing reads it there, but the
	// installed fragment is where a drawer pane's copy comes FROM, so it has to
	// decode.
	if plugin := cfg.TUI.Plugins["breaktimer"]; len(plugin.Views) != 1 || plugin.Views[0].Name != "full" {
		t.Errorf("the rail entry's views = %#v", cfg.TUI.Plugins["breaktimer"].Views)
	}
}

// The default, which is the reason the bool exists: a pane that names no view
// gets the author's first drawer-suitable one, so an installed panel is right in
// a drawer before the user has read its docs.
func TestEffectiveViewDefaultsToTheFirstDrawerView(t *testing.T) {
	declared := []PluginView{
		{Name: "full", Description: "clock, history and help"},
		{Name: "compact", Description: "one line", Drawer: true},
		{Name: "mini", Description: "even less", Drawer: true},
	}

	// No view named: the first drawer-suitable one wins, and "first" is
	// declaration order rather than anything alphabetical — `mini` sorts before
	// `compact` and must not win.
	pane := &DrawerPaneConfig{Backend: DrawerBackendSidecar, Command: "bt", Views: declared}
	if got := pane.EffectiveView(); got != "compact" {
		t.Errorf("defaulted view = %q, want compact", got)
	}

	// A view the user named wins over the author's default, including one the
	// author marked unsuitable and one they never declared at all. The user is
	// the only party who knows how wide their drawer is, and an undeclared name
	// is the panel's to forgive.
	for _, want := range []string{"full", "invented"} {
		pane := &DrawerPaneConfig{Backend: DrawerBackendSidecar, Command: "bt", View: want, Views: declared}
		if got := pane.EffectiveView(); got != want {
			t.Errorf("explicit view %q resolved to %q", want, got)
		}
	}

	// Declaring no drawer-suitable view is a legitimate answer — the panel is
	// declining to offer one — and it resolves to empty, which on the wire means
	// the panel's own default rather than a name the host invented.
	none := &DrawerPaneConfig{Backend: DrawerBackendSidecar, Command: "bt", Views: []PluginView{
		{Name: "full"}, {Name: "wide"},
	}}
	if got := none.EffectiveView(); got != "" {
		t.Errorf("a panel with no drawer view resolved to %q, want empty", got)
	}

	// And a declaration with no views at all — every pane that exists today —
	// behaves exactly as it did before this field existed.
	if got := (&DrawerPaneConfig{Backend: DrawerBackendSidecar, Command: "bt"}).EffectiveView(); got != "" {
		t.Errorf("a pane declaring no views resolved to %q, want empty", got)
	}
	var nilCfg *DrawerPaneConfig
	if got := nilCfg.EffectiveView(); got != "" {
		t.Errorf("a nil declaration resolved to %q, want empty", got)
	}
}

// DeclaredView is the other half of what the host may ask, and the second return
// is the point: "declared, not for a drawer" is something a host can report
// honestly, while "never declared" is a name only the panel can judge.
func TestDeclaredViewSeparatesUnsuitableFromUnknown(t *testing.T) {
	pane := &DrawerPaneConfig{Views: []PluginView{
		{Name: "full", Description: "clock, history and help"},
		{Name: "compact", Drawer: true},
	}}

	full, ok := pane.DeclaredView("full")
	if !ok {
		t.Fatal("full is declared and was not found")
	}
	if full.Drawer {
		t.Error("full read as drawer-suitable")
	}
	if full.Description != "clock, history and help" {
		t.Errorf("description = %q", full.Description)
	}
	if compact, ok := pane.DeclaredView("compact"); !ok || !compact.Drawer {
		t.Errorf("compact = %#v, found = %v", compact, ok)
	}
	for _, name := range []string{"invented", ""} {
		if _, ok := pane.DeclaredView(name); ok {
			t.Errorf("%q read as declared", name)
		}
	}
	var nilCfg *DrawerPaneConfig
	if _, ok := nilCfg.DeclaredView("compact"); ok {
		t.Error("a nil declaration answered for a view")
	}
}

// The declaration survives a wholesale pane replacement and a clone owns its own
// slice — the same statement TestDrawerPaneViewMergesWithTheDeclaration makes
// about `view`, for the list the default is read from.
func TestDeclaredViewsMergeAndCloneWithThePane(t *testing.T) {
	base := &Config{TUI: &TUIConfig{Drawer: &DrawerViewsConfig{
		Panes: map[string]*DrawerPaneConfig{
			"breaktimer": {Backend: DrawerBackendSidecar, Command: "bt", Views: []PluginView{
				{Name: "full"}, {Name: "compact", Drawer: true},
			}},
		},
	}}}
	got := mergeConfigs(base, &Config{TUI: &TUIConfig{Drawer: &DrawerViewsConfig{
		Panes: map[string]*DrawerPaneConfig{
			"breaktimer": {Backend: DrawerBackendSidecar, Command: "bt", Views: []PluginView{
				{Name: "wide", Drawer: true},
			}},
		},
	}}})
	if v := got.TUI.Drawer.Panes["breaktimer"].EffectiveView(); v != "wide" {
		t.Errorf("merged default view = %q, want wide", v)
	}
	if v := base.TUI.Drawer.Panes["breaktimer"].EffectiveView(); v != "compact" {
		t.Errorf("merge mutated the base layer: %q", v)
	}

	clone := cloneDrawerPane(base.TUI.Drawer.Panes["breaktimer"])
	if len(clone.Views) != 2 {
		t.Fatalf("clone lost the views: %#v", clone.Views)
	}
	clone.Views[1].Drawer = false
	if !base.TUI.Drawer.Panes["breaktimer"].Views[1].Drawer {
		t.Error("clone shares its views slice with the layer it came from")
	}
}
