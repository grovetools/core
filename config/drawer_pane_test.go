package config

import (
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

// [tui.drawer.panes.<name>] is where a pane's BACKEND is chosen. It sits beside
// the page definitions rather than inside one for the reason the Files block
// does: a pane is not owned by a page, so what a pane IS belongs next to the
// pages, never in one of them.

func TestDrawerPaneBackendRoundTripsThroughTOML(t *testing.T) {
	const src = `
[tui.drawer.panes.changes]
backend    = "sidecar"
command    = "git-viewer"
args       = ["panel", "changes"]
label      = "Changes"
icon       = "diff"
min_width  = 30
min_height = 8
restart    = true

[tui.drawer.panes.changes.settings]
base = "working"

[[tui.drawer.panes.changes.keys]]
key         = "ctrl+g"
description = "open the full viewer"

[tui.drawer.panes.notes]
min_width = 20
`
	var cfg Config
	if _, err := toml.Decode(src, &cfg); err != nil {
		t.Fatalf("decode: %v", err)
	}
	panes := cfg.TUI.Drawer.Panes
	changes := panes["changes"]
	if changes == nil {
		t.Fatal("the changes pane declaration did not decode")
	}
	if !changes.SidecarBacked() {
		t.Error("backend = \"sidecar\" did not read as sidecar-backed")
	}
	if changes.Command != "git-viewer" || strings.Join(changes.Args, " ") != "panel changes" {
		t.Errorf("command/args = %q %v", changes.Command, changes.Args)
	}
	if changes.MinWidth != 30 || changes.MinHeight != 8 {
		t.Errorf("minimums = %dx%d, want 30x8", changes.MinWidth, changes.MinHeight)
	}
	if changes.Settings["base"] != "working" {
		t.Errorf("settings = %#v", changes.Settings)
	}
	if len(changes.Keys) != 1 || changes.Keys[0].Key != "ctrl+g" {
		t.Errorf("declared keys = %#v", changes.Keys)
	}

	// An entry that names no backend is IN-PROCESS, so writing only a minimum
	// tunes the built-in widget instead of accidentally asking for a process.
	if panes["notes"].SidecarBacked() {
		t.Error("an entry with no backend key read as sidecar-backed")
	}
}

// A sidecar pane's control plane defaults to embed/v1 — the opposite of
// [tui.plugins], where an empty protocol means a plain PTY plugin.
//
// The asymmetry is deliberate and worth pinning: a rail plugin with no control
// plane is a perfectly good terminal pane, while a drawer pane with none can
// never tell the host it is unavailable or what its empty state says.
func TestDrawerSidecarDefaultsToTheControlPlane(t *testing.T) {
	if got := (&DrawerPaneConfig{}).EffectiveProtocol(); got != "embed/v1" {
		t.Errorf("default protocol = %q, want embed/v1", got)
	}
	if got := (&DrawerPaneConfig{Protocol: "embed/v9"}).EffectiveProtocol(); got != "embed/v9" {
		t.Errorf("explicit protocol = %q, want it kept verbatim", got)
	}
	var nilCfg *DrawerPaneConfig
	if nilCfg.SidecarBacked() {
		t.Error("a nil declaration read as sidecar-backed")
	}
}

// Pane DECLARATIONS replace wholesale, like page definitions and unlike the
// field-wise Files block: a declaration says what a pane IS, and half of one
// layer's command with half of another's args is a process nobody wrote down.
func TestDrawerPaneDeclarationsMergeWholesale(t *testing.T) {
	base := &Config{TUI: &TUIConfig{Drawer: &DrawerViewsConfig{
		Panes: map[string]*DrawerPaneConfig{
			"changes": {Backend: DrawerBackendSidecar, Command: "git-viewer", Args: []string{"panel", "changes"}, MinWidth: 30},
			"probe":   {Backend: DrawerBackendSidecar, Command: "grove-panel-probe"},
		},
	}}}

	// An override that says nothing about panes leaves both standing.
	got := mergeConfigs(base, &Config{TUI: &TUIConfig{Drawer: &DrawerViewsConfig{DefaultPage: "git"}}})
	if len(got.TUI.Drawer.Panes) != 2 {
		t.Fatalf("an unrelated override dropped pane declarations: %#v", got.TUI.Drawer.Panes)
	}

	// An override that restates one pane replaces THAT pane whole and leaves
	// the other alone.
	got = mergeConfigs(base, &Config{TUI: &TUIConfig{Drawer: &DrawerViewsConfig{
		Panes: map[string]*DrawerPaneConfig{"changes": {Backend: DrawerBackendSidecar, Command: "other"}},
	}}})
	if got.TUI.Drawer.Panes["changes"].MinWidth != 0 {
		t.Error("whole-pane replacement retained the base minimum")
	}
	if len(got.TUI.Drawer.Panes["changes"].Args) != 0 {
		t.Error("whole-pane replacement retained the base args")
	}
	if got.TUI.Drawer.Panes["probe"].Command != "grove-panel-probe" {
		t.Error("replacing one pane dropped another")
	}

	// And the base layer is untouched: a merged result is owned by its caller.
	if base.TUI.Drawer.Panes["changes"].Command != "git-viewer" {
		t.Error("merge mutated the base layer")
	}
	if got.TUI.Drawer.Panes["changes"].Command == base.TUI.Drawer.Panes["changes"].Command {
		t.Error("the override did not take effect at all")
	}
}

// A cloned declaration shares no slice or map with the layer it came from, so a
// later append cannot edit the layer it inherited from.
func TestDrawerPaneCloneIsDeep(t *testing.T) {
	src := &DrawerPaneConfig{
		Backend:  DrawerBackendSidecar,
		Command:  "probe",
		Args:     []string{"--one"},
		Env:      []string{"A=1"},
		Keys:     []PluginKey{{Key: "x"}},
		Settings: map[string]interface{}{"k": "v"},
	}
	clone := cloneDrawerPane(src)
	clone.Args[0] = "--two"
	clone.Env[0] = "A=2"
	clone.Keys[0].Key = "y"
	clone.Settings["k"] = "w"

	if src.Args[0] != "--one" || src.Env[0] != "A=1" || src.Keys[0].Key != "x" || src.Settings["k"] != "v" {
		t.Errorf("the clone shares state with its source: %#v", src)
	}
	if cloneDrawerPane(nil) != nil {
		t.Error("cloning nil produced a value")
	}
}

// A drawer sidecar's declaration is exec-bearing and quarantined on the same
// terms as a rail plugin's: it is the same spawn in a different slot, and the
// whole entry is gated because args, env and cwd all steer what actually runs.
func TestDrawerSidecarIsExecGated(t *testing.T) {
	var found *ExecField
	for i, f := range ExecFields() {
		if f.Path == "tui.drawer.panes.*" {
			found = &ExecFields()[i]
			break
		}
	}
	if found == nil {
		t.Fatal("tui.drawer.panes.* is not registered as an exec-bearing field — a workspace layer could spawn a process")
	}
	if found.Risk != RiskImplicit {
		t.Errorf("risk = %v, want implicit (nothing the user typed asks for this spawn)", found.Risk)
	}
}
