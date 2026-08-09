package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// A named TOML beside grove.toml is a global fragment. TUI preferences are
// intentionally split into tui.toml on real machines, so values parsed from
// that fragment must survive the same merge path treemux uses at startup.
func TestLoadLayeredReadsExperimentalPagesFromTUIFragment(t *testing.T) {
	ResetLoadCache()
	t.Cleanup(ResetLoadCache)

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".state"))
	t.Setenv("GROVE_HOME", "")
	t.Setenv("GROVE_CONFIG_OVERLAY", "")

	configDir := filepath.Join(home, ".config", "grove")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "grove.toml"), []byte(`[tui]
theme = "dark"
`), 0o644); err != nil {
		t.Fatalf("write grove.toml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "tui.toml"), []byte(`[tui]
experimental_pages = ["inspector", "git", "logs", "plugins"]
`), 0o644); err != nil {
		t.Fatalf("write tui.toml: %v", err)
	}

	workDir := filepath.Join(home, "work")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("mkdir work dir: %v", err)
	}
	layered, err := LoadLayered(workDir)
	if err != nil {
		t.Fatalf("LoadLayered: %v", err)
	}
	if layered.Final == nil || layered.Final.TUI == nil {
		t.Fatal("expected merged [tui] config")
	}
	want := []string{"inspector", "git", "logs", "plugins"}
	if got := layered.Final.TUI.ExperimentalPages; !reflect.DeepEqual(got, want) {
		t.Errorf("experimental_pages = %v, want %v from tui.toml", got, want)
	}
	if got := layered.Final.TUI.Theme; got != "dark" {
		t.Errorf("theme = %q, want base grove.toml value", got)
	}
}
