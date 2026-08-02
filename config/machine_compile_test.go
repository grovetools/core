package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
)

// writeFileIn is writeFile plus the parent directories, for fixtures that
// build a whole tree.
func writeFileIn(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	writeFile(t, path, content)
}

// The full machine.toml shape: name, ecosystem subscriptions, bare roots.
func TestLoadMachineConfigFullShape(t *testing.T) {
	dir := sandboxConfig(t)
	writeFile(t, filepath.Join(dir, "machine.toml"), `[machine]
name = "mbp"

[machine.ecosystems.grovetools]
path = "~/code/grovetools"
notebook = "grovetools"
description = "Grove ecosystem tools"

[machine.ecosystems.solutils]
path = "~/code/solutils"

[machine.roots.chickens]
path = "~/code/chickens"
notebook = "nb"
enabled = false
`)

	cfg, err := LoadMachineConfig()
	if err != nil {
		t.Fatalf("LoadMachineConfig: %v", err)
	}
	if cfg == nil {
		t.Fatal("LoadMachineConfig returned nil")
	}
	if got := len(cfg.Machine.Ecosystems); got != 2 {
		t.Fatalf("ecosystems = %d, want 2: %+v", got, cfg.Machine.Ecosystems)
	}
	gt := cfg.Machine.Ecosystems["grovetools"]
	if gt.Path != "~/code/grovetools" || gt.Notebook != "grovetools" || gt.Description != "Grove ecosystem tools" {
		t.Fatalf("grovetools subscription = %+v", gt)
	}
	// The notebook override is optional: absent means "defer to the card".
	if nb := cfg.Machine.Ecosystems["solutils"].Notebook; nb != "" {
		t.Fatalf("solutils notebook = %q, want empty (card default)", nb)
	}
	chickens := cfg.Machine.Roots["chickens"]
	if chickens.Path != "~/code/chickens" || chickens.Notebook != "nb" {
		t.Fatalf("chickens root = %+v", chickens)
	}
	if chickens.Enabled == nil || *chickens.Enabled {
		t.Fatalf("chickens enabled = %v, want explicit false", chickens.Enabled)
	}
}

func TestMachineConfigValidateRejectsBadEntries(t *testing.T) {
	cases := map[string]string{
		"ecosystem without a path": `[machine.ecosystems.x]
notebook = "nb"
`,
		"root without a path": `[machine.roots.x]
notebook = "nb"
`,
		"name used by both kinds": `[machine.ecosystems.x]
path = "/a"

[machine.roots.x]
path = "/b"
`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			dir := sandboxConfig(t)
			path := filepath.Join(dir, "machine.toml")
			writeFile(t, path, content)
			if _, err := LoadMachineConfigFrom(path); err == nil {
				t.Fatalf("LoadMachineConfigFrom accepted an invalid config")
			}
		})
	}
}

// The typed-loader guard: machine.toml is read by LoadMachineConfig alone, so
// a table it does not model can never reach the config cascade. This is F5
// containment — the whole reason machine.toml is not a fragment.
func TestStrayTableInMachineTOMLNeverLeaksIntoTheCascade(t *testing.T) {
	dir := sandboxConfig(t)
	writeFile(t, filepath.Join(dir, "grove.toml"), "name = \"test\"\n")
	writeFile(t, filepath.Join(dir, "machine.toml"), `[machine]
name = "mbp"

[machine.ecosystems.grovetools]
path = "/tmp/grovetools"

[tui]
theme = "leaked-theme"

[some_extension]
key = "leaked-value"
`)

	assertNoLeak := func(t *testing.T, cfg *Config) {
		t.Helper()
		if cfg.TUI != nil && cfg.TUI.Theme == "leaked-theme" {
			t.Fatalf("machine.toml's [tui] table leaked into the cascade: %+v", cfg.TUI)
		}
		if _, leaked := cfg.Extensions["some_extension"]; leaked {
			t.Fatalf("machine.toml's stray table leaked into Extensions: %v", cfg.Extensions)
		}
		if _, leaked := cfg.Extensions["machine"]; leaked {
			t.Fatalf("machine.toml's [machine] table leaked into Extensions: %v", cfg.Extensions)
		}
		// The parts the typed loader DOES model still arrive, via compilation.
		if _, ok := cfg.Groves["grovetools"]; !ok {
			t.Fatalf("compiled subscription missing from Groves: %v", cfg.Groves)
		}
	}

	cfg, err := LoadFromWithLogger(t.TempDir(), logrus.New())
	if err != nil {
		t.Fatalf("LoadFromWithLogger: %v", err)
	}
	assertNoLeak(t, cfg)

	ResetLoadCache()
	layered, err := LoadLayered(t.TempDir())
	if err != nil {
		t.Fatalf("LoadLayered: %v", err)
	}
	assertNoLeak(t, layered.Final)
}

// Compilation is fill-absent-only: an explicit [groves.*] entry wins for the
// whole migration window, so a user can declare intent in machine.toml before
// deleting the old entry.
func TestCompileMachineGrovesFillsAbsentOnly(t *testing.T) {
	dir := sandboxConfig(t)
	writeFile(t, filepath.Join(dir, "grove.toml"), `name = "test"

[groves.grovetools]
path = "/explicit/grovetools"
notebook = "explicit"
`)
	writeFile(t, filepath.Join(dir, "machine.toml"), `[machine]
name = "mbp"

[machine.ecosystems.grovetools]
path = "/compiled/grovetools"
notebook = "compiled"

[machine.roots.chickens]
path = "/compiled/chickens"
notebook = "nb"
`)

	cfg, err := LoadFromWithLogger(t.TempDir(), logrus.New())
	if err != nil {
		t.Fatalf("LoadFromWithLogger: %v", err)
	}
	if got := cfg.Groves["grovetools"].Path; got != "/explicit/grovetools" {
		t.Fatalf("explicit [groves.grovetools] lost to the compiled entry: %q", got)
	}
	if got := cfg.Groves["chickens"].Path; got != "/compiled/chickens" {
		t.Fatalf("compiled root missing: %+v", cfg.Groves)
	}
	// SetDefaults runs after compilation, so compiled entries are enabled by
	// default like every other grove entry.
	if e := cfg.Groves["chickens"].Enabled; e == nil || !*e {
		t.Fatalf("compiled root Enabled = %v, want defaulted true", e)
	}
}

// LoadLayered must not attribute compiled entries to a config FILE: with no
// fragments, finalConfig would otherwise be the very same *Config as the
// Global layer, and `grove config audit` would report machine.toml's
// subscriptions as if grove.toml had declared them.
func TestCompileMachineGrovesDoesNotMutateTheGlobalLayer(t *testing.T) {
	dir := sandboxConfig(t)
	writeFile(t, filepath.Join(dir, "grove.toml"), "name = \"test\"\n")
	writeFile(t, filepath.Join(dir, "machine.toml"), `[machine.ecosystems.grovetools]
path = "/compiled/grovetools"
`)

	layered, err := LoadLayered(t.TempDir())
	if err != nil {
		t.Fatalf("LoadLayered: %v", err)
	}
	if _, ok := layered.Final.Groves["grovetools"]; !ok {
		t.Fatalf("Final.Groves missing the compiled entry: %v", layered.Final.Groves)
	}
	if layered.Global == nil {
		t.Fatal("Global layer was not loaded")
	}
	if _, leaked := layered.Global.Groves["grovetools"]; leaked {
		t.Fatalf("compiled entry leaked into the Global layer: %v", layered.Global.Groves)
	}
}

// The notebook layer is resolved from groves (findNotebookConfigPath →
// resolveNotebookContext), so a machine whose only declaration lives in
// machine.toml must still resolve it — the reason compilation runs on
// lookupConfig too, not just on the final merge.
func TestCompiledGrovesDriveNotebookLayerResolution(t *testing.T) {
	dir := sandboxConfig(t)
	root := t.TempDir()
	groveRoot := filepath.Join(root, "code")
	project := filepath.Join(groveRoot, "myproject")
	notebookWS := filepath.Join(root, "notebooks", "nb", "workspaces", "myproject")

	writeFileIn(t, filepath.Join(project, "grove.toml"), "name = \"myproject\"\n")
	writeFileIn(t, filepath.Join(notebookWS, "grove.toml"), "build_cmd = \"from-notebook-layer\"\n")
	writeFile(t, filepath.Join(dir, "grove.toml"), `name = "global"

[notebooks.definitions.nb]
root_dir = "`+filepath.Join(root, "notebooks", "nb")+`"
`)
	writeFile(t, filepath.Join(dir, "machine.toml"), `[machine.roots.code]
path = "`+groveRoot+`"
notebook = "nb"
`)

	layered, err := LoadLayered(project)
	if err != nil {
		t.Fatalf("LoadLayered: %v", err)
	}
	if layered.ProjectNotebook == nil {
		t.Fatalf("notebook layer not resolved through the compiled grove: %v", layered.FilePaths)
	}
	if got := layered.Final.BuildCmd; got != "from-notebook-layer" {
		t.Fatalf("BuildCmd = %q, want the notebook layer's value", got)
	}
}
