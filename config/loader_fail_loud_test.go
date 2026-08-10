package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type layerFailureFixture struct {
	start  string
	target string
}

func failLoudFixture(t *testing.T, layer string) layerFailureFixture {
	t.Helper()
	ResetLoadCache()
	t.Cleanup(ResetLoadCache)

	root := t.TempDir()
	xdg := filepath.Join(root, "xdg")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	configDir := filepath.Join(xdg, "grove")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	global := filepath.Join(configDir, "grove.toml")
	if err := os.WriteFile(global, []byte("name = \"global\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(root, "code", "eco", "app")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	projectConfig := filepath.Join(project, "grove.toml")
	if err := os.WriteFile(projectConfig, []byte("name = \"app\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var target string
	switch layer {
	case "global":
		target = global
	case "fragment":
		target = filepath.Join(configDir, "20-broken.toml")
	case "plugin":
		target = filepath.Join(configDir, "plugins", "broken.toml")
	case "global override":
		target = filepath.Join(configDir, "grove.override.toml")
	case "project":
		target = projectConfig
	case "project override":
		target = filepath.Join(project, "grove.override.toml")
	case "ecosystem":
		target = filepath.Join(filepath.Dir(project), "grove.toml")
	case "notebook-local":
		notebookRoot := filepath.Join(root, "notes")
		if err := os.MkdirAll(filepath.Join(notebookRoot, "workspaces", "app"), 0o755); err != nil {
			t.Fatal(err)
		}
		writeNotebookTestRouting(t, configDir, filepath.Join(root, "code", "eco"), notebookRoot)
		target = filepath.Join(notebookRoot, "workspaces", "app", "grove.toml")
	default:
		t.Fatalf("unknown layer %q", layer)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	return layerFailureFixture{start: project, target: target}
}

func TestHierarchyLoadersFailLoudOnMalformedLayers(t *testing.T) {
	layers := []string{"global", "fragment", "plugin", "global override", "project", "project override", "ecosystem", "notebook-local"}
	loaders := []struct {
		name string
		load func(string) error
	}{
		{"LoadFrom", func(start string) error { _, err := LoadFrom(start); return err }},
		{"LoadLayered", func(start string) error { _, err := LoadLayered(start); return err }},
	}
	for _, layer := range layers {
		for _, loader := range loaders {
			t.Run(layer+"/"+loader.name, func(t *testing.T) {
				fixture := failLoudFixture(t, layer)
				if err := os.WriteFile(fixture.target, []byte("[broken\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				err := loader.load(fixture.start)
				if err == nil || !strings.Contains(err.Error(), fixture.target) {
					t.Fatalf("error = %v, want malformed layer path %s", err, fixture.target)
				}
			})
		}
	}
}

func TestHierarchyLoadersFailLoudOnUnreadableLayers(t *testing.T) {
	layers := []string{"global", "fragment", "plugin", "global override", "project", "project override", "ecosystem", "notebook-local"}
	loaders := []struct {
		name string
		load func(string) error
	}{
		{"LoadFrom", func(start string) error { _, err := LoadFrom(start); return err }},
		{"LoadLayered", func(start string) error { _, err := LoadLayered(start); return err }},
	}
	for _, layer := range layers {
		for _, loader := range loaders {
			t.Run(layer+"/"+loader.name, func(t *testing.T) {
				fixture := failLoudFixture(t, layer)
				if err := os.WriteFile(fixture.target, []byte("name = \"valid\"\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(fixture.target, 0); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(fixture.target, 0o600) })
				err := loader.load(fixture.start)
				if err == nil || !strings.Contains(err.Error(), fixture.target) {
					t.Fatalf("error = %v, want unreadable layer path %s", err, fixture.target)
				}
			})
		}
	}
}

func TestExplicitLoadIgnoresAmbientRecordedTopology(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	configDir := filepath.Join(xdg, "grove")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"roots.toml", "notebooks.toml"} {
		if err := os.WriteFile(filepath.Join(configDir, name), []byte("not valid topology"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(t.TempDir(), "grove.toml")
	if err := os.WriteFile(path, []byte("name = \"explicit\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if cfg, err := Load(path); err != nil || cfg.Name != "explicit" {
		t.Fatalf("explicit Load consulted ambient topology: cfg=%+v err=%v", cfg, err)
	}
}

func TestByteOnlyLoadersIgnoreAmbientRecordedTopology(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	configDir := filepath.Join(xdg, "grove")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	bad := []byte("this is not recorded topology")
	for _, name := range []string{"roots.toml", "notebooks.toml"} {
		if err := os.WriteFile(filepath.Join(configDir, name), bad, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if cfg, err := LoadFromTOMLBytes([]byte("name = \"toml\"\n")); err != nil || cfg.Name != "toml" {
		t.Fatalf("TOML bytes consulted ambient topology: cfg=%+v err=%v", cfg, err)
	}
	if cfg, err := LoadFromBytes([]byte("name: yaml\n")); err != nil || cfg.Name != "yaml" {
		t.Fatalf("YAML bytes consulted ambient topology: cfg=%+v err=%v", cfg, err)
	}
}
