package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeRecordedPair(t *testing.T, roots, notebooks string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	rp, np := filepath.Join(dir, "roots.toml"), filepath.Join(dir, "notebooks.toml")
	if roots != "" {
		if err := os.WriteFile(rp, []byte(roots), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if notebooks != "" {
		if err := os.WriteFile(np, []byte(notebooks), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return rp, np
}

func TestCompileCodeRootsProjectsRecordedViews(t *testing.T) {
	t.Setenv("NB_ROOT", filepath.Join(t.TempDir(), "notes"))
	rp, np := writeRecordedPair(t, `
[roots.scan]
path = "/code"
scan = true
description = "all code"
depth = 2
exclude = ["tmp"]

[roots.specific]
path = "/code/specific"
notebook = "other"
repos = ["a", "b"]
enabled = false
`, `
default = "nb"
[notebooks.nb]
root = "${NB_ROOT}"
[notebooks.other]
root = "~/other-notes"
`)
	legacy := &Config{Notebooks: &NotebooksConfig{Definitions: map[string]*Notebook{"legacy": {RootDir: "/old"}}}}
	got, err := compileCodeRootsFromPaths(legacy, rp, np)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Notebooks.Definitions) != 2 || got.Notebooks.Definitions["legacy"] != nil {
		t.Fatalf("recorded definitions did not replace legacy: %+v", got.Notebooks.Definitions)
	}
	wantNBRoot := os.Getenv("NB_ROOT")
	if got.Notebooks.Rules.Default != "nb" || got.Notebooks.Definitions["nb"].RootDir != wantNBRoot {
		t.Fatalf("notebook projection = %+v", got.Notebooks)
	}
	scan := got.Groves["scan"]
	if !scan.Scan || scan.Notebook != "nb" || scan.NotebookRoot != wantNBRoot || scan.Depth == nil || *scan.Depth != 2 ||
		scan.Description != "all code" || !reflect.DeepEqual(scan.ExcludeRepos, []string{"tmp"}) {
		t.Fatalf("scan projection = %+v", scan)
	}
	specific := got.Groves["specific"]
	if specific.Scan || specific.Notebook != "other" || specific.NotebookRoot == "" || specific.Enabled == nil || *specific.Enabled ||
		!reflect.DeepEqual(specific.IncludeRepos, []string{"a", "b"}) {
		t.Fatalf("specific projection = %+v", specific)
	}
}

func TestCompileCodeRootsRecordedNotebookPreservesSameNameBehaviorFields(t *testing.T) {
	rp, np := writeRecordedPair(t, `[roots.code]
path = "/code"
`, `default = "nb"
[notebooks.nb]
root = "/recorded-notes"
`)
	legacyNotebook := &Notebook{
		RootDir:           "/legacy-notes",
		NotesPathTemplate: "stale-notes/{{.Workspace}}",
		Types:             map[string]*NoteTypeConfig{"stale": {}},
		Syncthing:         &SyncthingConfig{Devices: []string{"stale-device"}},
		Obsidian:          &ObsidianConfig{VaultName: "stale-vault"},
	}
	cfg := &Config{Notebooks: &NotebooksConfig{Definitions: map[string]*Notebook{"nb": legacyNotebook}}}

	got, err := compileCodeRootsFromPaths(cfg, rp, np)
	if err != nil {
		t.Fatal(err)
	}
	want := *legacyNotebook
	want.RootDir = "/recorded-notes"
	if !reflect.DeepEqual(got.Notebooks.Definitions["nb"], &want) {
		t.Fatalf("recorded root did not preserve same-name behavior fields: got %+v, want %+v", got.Notebooks.Definitions["nb"], &want)
	}
	if legacyNotebook.RootDir != "/legacy-notes" || legacyNotebook.NotesPathTemplate == "" {
		t.Fatalf("compiler mutated legacy source definition: %+v", legacyNotebook)
	}
}

func TestCompileCodeRootsRejectsDanglingNotebook(t *testing.T) {
	rp, np := writeRecordedPair(t, `[roots.bad]
path = "/code"
notebook = "missing"
`, `[notebooks.nb]
root = "/notes"
`)
	_, err := compileCodeRootsFromPaths(&Config{}, rp, np)
	if err == nil || !strings.Contains(err.Error(), "roots.bad") || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("dangling notebook error = %v", err)
	}
}

func TestCompiledRootsDeepestEnabledRootWins(t *testing.T) {
	rp, np := writeRecordedPair(t, `[roots.parent]
path = "/code"
[roots.child]
path = "/code/nested"
[roots.disabled]
path = "/code/nested/deeper"
enabled = false
`, `default = "nb"
[notebooks.nb]
root = "/notes"
`)
	cfg, err := compileCodeRootsFromPaths(&Config{}, rp, np)
	if err != nil {
		t.Fatal(err)
	}
	binding := ResolveNotebook(NotebookQuery{Path: "/code/nested/repo"}, cfg)
	if binding.GroveName != "child" || binding.Notebook != "nb" || binding.NotebookRoot != "/notes" {
		t.Fatalf("binding = %+v", binding)
	}
	binding = ResolveNotebook(NotebookQuery{Path: "/code/nested/deeper/repo"}, cfg)
	if binding.GroveName != "child" {
		t.Fatalf("disabled root won: %+v", binding)
	}
}
