package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grovetools/core/pkg/coderoot"
)

func TestReconcileCodeRootsSpecificOnly(t *testing.T) {
	root := t.TempDir()
	manifested := filepath.Join(root, "manifested")
	if err := os.MkdirAll(manifested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(manifested, "grove.toml"), []byte("name = \"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	table := coderoot.Table{
		Default: "nb", Notebooks: map[string]coderoot.Notebook{"nb": {Root: "/notes"}},
		Roots: map[string]coderoot.Root{
			"present": {Path: manifested},
			"missing": {Path: filepath.Join(root, "missing")},
			"scan":    {Path: root, Scan: true},
		},
	}
	states := ReconcileCodeRoots(table)
	if len(states) != 2 || states[0].Name != "missing" || states[0].State != CodeRootDeclaredMissing ||
		states[1].Name != "present" || states[1].State != CodeRootPresent || states[1].Notebook != "nb" {
		t.Fatalf("states = %+v", states)
	}
}

func TestMachineTopologyTablesRequireMigration(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, table := range []string{"ecosystems", "roots"} {
		path := filepath.Join(t.TempDir(), "machine.toml")
		_, err := ParseMachineConfigContent(path, "[machine."+table+".legacy]\npath = \"/code\"\n")
		if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "[machine."+table+"]") || !strings.Contains(err.Error(), "grove migrate") || !strings.Contains(err.Error(), "roots.toml is absent") {
			t.Fatalf("table %s error = %v", table, err)
		}
	}
}

func TestCanonicalLoadersRejectLegacyMachineTopology(t *testing.T) {
	for _, tc := range []struct {
		name string
		load func(string) error
	}{
		{name: "Load", load: func(project string) error { _, err := Load(filepath.Join(project, "grove.toml")); return err }},
		{name: "LoadFrom", load: func(project string) error { _, err := LoadFrom(project); return err }},
		{name: "LoadLayered", load: func(project string) error { _, err := LoadLayered(project); return err }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ResetLoadCache()
			t.Cleanup(ResetLoadCache)
			configHome := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", configHome)
			configDir := filepath.Join(configHome, "grove")
			if err := os.MkdirAll(configDir, 0o755); err != nil {
				t.Fatal(err)
			}
			machinePath := filepath.Join(configDir, MachineConfigFileName)
			if err := os.WriteFile(machinePath, []byte("[machine.ecosystems.old]\npath = \"/code\"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			project := t.TempDir()
			if err := os.WriteFile(filepath.Join(project, "grove.toml"), []byte("name = \"project\"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			err := tc.load(project)
			if err == nil || !strings.Contains(err.Error(), machinePath) || !strings.Contains(err.Error(), "roots.toml is absent") || !strings.Contains(err.Error(), "grove migrate") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestCanonicalMachineTopologyRejectsMixedState(t *testing.T) {
	ResetLoadCache()
	t.Cleanup(ResetLoadCache)
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	configDir := filepath.Join(configHome, "grove")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, coderoot.RootsFileName), []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	machinePath := filepath.Join(configDir, MachineConfigFileName)
	if err := os.WriteFile(machinePath, []byte("[machine.roots.old]\npath = \"/code\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(t.TempDir(), "grove.toml")
	if err := os.WriteFile(projectPath, []byte("name = \"project\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(projectPath)
	if err == nil || !strings.Contains(err.Error(), "forbidden mixed state") || !strings.Contains(err.Error(), machinePath) {
		t.Fatalf("error = %v", err)
	}
}
