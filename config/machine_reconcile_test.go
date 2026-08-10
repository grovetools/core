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
	for _, table := range []string{"ecosystems", "roots"} {
		path := filepath.Join(t.TempDir(), "machine.toml")
		_, err := ParseMachineConfigContent(path, "[machine."+table+".legacy]\npath = \"/code\"\n")
		if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "[machine."+table+"]") || !strings.Contains(err.Error(), "grove migrate") {
			t.Fatalf("table %s error = %v", table, err)
		}
	}
}
