package config

import (
	"os"
	"path/filepath"
	"testing"
)

// The ~/.config/grove/plugins/*.toml drop-in directory is where `grove plugin
// install` writes one [tui.plugins.<name>] fragment per installed panel. Its
// only consumer — treemux — reads the config through LoadLayered, so a drop-in
// that LoadFrom honors and LoadLayered ignores is an install that silently
// does nothing.
//
// These tests pin both loaders on the same file.

// pluginDropInFixture builds a HOME whose grove config is a single plugin
// drop-in, and returns a working directory with no project config above it.
func pluginDropInFixture(t *testing.T, name, body string) string {
	t.Helper()
	ResetLoadCache()
	t.Cleanup(ResetLoadCache)

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".state"))
	t.Setenv("GROVE_HOME", "")
	t.Setenv("GROVE_CONFIG_OVERLAY", "")

	dropInDir := filepath.Join(home, ".config", "grove", "plugins")
	if err := os.MkdirAll(dropInDir, 0o755); err != nil {
		t.Fatalf("mkdir plugin drop-in dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dropInDir, name), []byte(body), 0o644); err != nil {
		t.Fatalf("write plugin drop-in: %v", err)
	}

	workDir := filepath.Join(home, "work")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("mkdir work: %v", err)
	}
	return workDir
}

const installedPanelDropIn = `[tui.plugins.hello]
command = "/home/u/.local/share/grove/bin/grove-panel-hello"
icon = "H"
protocol = "embed/v1"
`

func TestLoadLayeredReadsThePluginDropInDirectory(t *testing.T) {
	workDir := pluginDropInFixture(t, "hello.toml", installedPanelDropIn)

	layered, err := LoadLayered(workDir)
	if err != nil {
		t.Fatalf("LoadLayered: %v", err)
	}
	if layered.Final == nil || layered.Final.TUI == nil {
		t.Fatal("expected a merged config with a [tui] section")
	}
	plugin, ok := layered.Final.TUI.Plugins["hello"]
	if !ok {
		t.Fatalf("the installed panel never reached the merged config: %+v", layered.Final.TUI.Plugins)
	}
	if plugin.Command != "/home/u/.local/share/grove/bin/grove-panel-hello" {
		t.Errorf("command = %q", plugin.Command)
	}
	if plugin.Protocol != "embed/v1" || plugin.Icon != "H" {
		t.Errorf("the drop-in lost fields on the way in: %+v", plugin)
	}
}

// A drop-in is the user's own config, not a repository's, so the
// exec-provenance gate must never quarantine it — otherwise installing a
// plugin would require trusting it a second time.
func TestPluginDropInIsAUserLayerAndIsNotGated(t *testing.T) {
	workDir := pluginDropInFixture(t, "hello.toml", installedPanelDropIn)

	layered, err := LoadLayered(workDir)
	if err != nil {
		t.Fatalf("LoadLayered: %v", err)
	}
	if report := layered.Final.ExecGate; report != nil {
		for _, f := range report.Files {
			if filepath.Base(f.Path) == "hello.toml" {
				t.Errorf("the plugin drop-in %s was gated as layer %q", f.Path, f.Layer)
			}
		}
	}
	if _, ok := layered.Final.TUI.Plugins["hello"]; !ok {
		t.Error("the drop-in was stripped from the merged config")
	}
}

// Both loaders must agree about the drop-in directory: LoadFrom is what most
// grove commands use, LoadLayered is what treemux uses.
func TestBothLoadersSeeThePluginDropIn(t *testing.T) {
	workDir := pluginDropInFixture(t, "hello.toml", installedPanelDropIn)

	cfg, err := LoadFrom(workDir)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if cfg.TUI == nil || cfg.TUI.Plugins["hello"] == nil {
		t.Fatalf("LoadFrom did not see the drop-in: %+v", cfg.TUI)
	}

	ResetLoadCache()
	layered, err := LoadLayered(workDir)
	if err != nil {
		t.Fatalf("LoadLayered: %v", err)
	}
	if layered.Final.TUI == nil || layered.Final.TUI.Plugins["hello"] == nil {
		t.Fatalf("LoadLayered did not see the drop-in: %+v", layered.Final.TUI)
	}
}

// The lockfile `grove plugin` keeps lives in the same directory and is
// deliberately not a .toml. If it ever becomes one, this glob would try to
// parse it as configuration.
func TestPluginDropInGlobIgnoresTheLockfile(t *testing.T) {
	workDir := pluginDropInFixture(t, "hello.toml", installedPanelDropIn)
	lock := filepath.Join(filepath.Dir(workDir), ".config", "grove", "plugins", "plugins.lock.json")
	if err := os.WriteFile(lock, []byte(`{"version":1,"plugins":{}}`), 0o600); err != nil {
		t.Fatalf("write lockfile: %v", err)
	}

	layered, err := LoadLayered(workDir)
	if err != nil {
		t.Fatalf("LoadLayered: %v", err)
	}
	if _, ok := layered.Final.TUI.Plugins["hello"]; !ok {
		t.Error("the lockfile's presence broke the drop-in load")
	}
}
