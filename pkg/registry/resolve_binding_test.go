package registry

import (
	"path/filepath"
	"testing"

	"github.com/grovetools/core/config"
)

// TestPlannedBindingNamesTheNotebookRoutingChose pins the reason the binding
// form exists. A caller that needs the notebook NAME (join records it in
// machine.toml `[sync.registry]`) used to re-derive it from
// notebooks.rules.default while the ROOT came from the rung above — so a
// workspace routed by a compiled binding was recorded under the default
// notebook's name and a root belonging to another notebook entirely.
func TestPlannedBindingNamesTheNotebookRoutingChose(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	workRoot := filepath.Join(home, "notebooks", "work")

	cfg := &config.Config{
		Groves: map[string]config.GroveSourceConfig{
			"alpha": {Notebook: "work", NotebookRoot: workRoot},
		},
		Notebooks: &config.NotebooksConfig{
			Definitions: map[string]*config.Notebook{
				"nb":   {RootDir: "~/notebooks/nb"},
				"work": {RootDir: workRoot},
			},
			Rules: &config.NotebookRules{Default: "nb"},
		},
	}

	binding, err := ResolvePlannedBinding(cfg, "alpha")
	if err != nil {
		t.Fatalf("ResolvePlannedBinding: %v", err)
	}
	if binding.Notebook != "work" {
		t.Errorf("binding notebook = %q, want the notebook the root came from (%q)", binding.Notebook, "work")
	}
	if binding.NotebookRoot != workRoot {
		t.Errorf("binding notebook root = %q, want %q", binding.NotebookRoot, workRoot)
	}
	if want := filepath.Join(workRoot, "notespaces", "alpha"); binding.Root != want {
		t.Errorf("binding root = %q, want %q", binding.Root, want)
	}
}

// TestPlannedBindingRootMatchesResolveWorkspaceRoot: the two must never drift,
// because creation uses one and every later read uses the other.
func TestPlannedBindingRootMatchesResolveWorkspaceRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := &config.Config{Notebooks: &config.NotebooksConfig{
		Definitions: map[string]*config.Notebook{"nb": {RootDir: "~/notebooks/nb"}},
		Rules:       &config.NotebookRules{Default: "nb"},
	}}

	binding, err := ResolvePlannedBinding(cfg, "alpha")
	if err != nil {
		t.Fatalf("ResolvePlannedBinding: %v", err)
	}
	root, err := ResolveWorkspaceRoot(cfg, "alpha")
	if err != nil {
		t.Fatalf("ResolveWorkspaceRoot: %v", err)
	}
	if binding.Root != root {
		t.Errorf("binding root = %q, ResolveWorkspaceRoot = %q", binding.Root, root)
	}
	if binding.Notebook != "nb" {
		t.Errorf("binding notebook = %q, want %q", binding.Notebook, "nb")
	}
}

// TestPlannedBindingRefusesAnUnroutableWorkspace: an absent binding must be an
// error, not an empty notebook name a caller would record verbatim.
func TestPlannedBindingRefusesAnUnroutableWorkspace(t *testing.T) {
	binding, err := ResolvePlannedBinding(&config.Config{}, "alpha")
	if err == nil {
		t.Fatalf("ResolvePlannedBinding accepted a config with no routing: %+v", binding)
	}
	if binding.Notebook != "" || binding.Root != "" {
		t.Errorf("failed resolution returned a usable binding: %+v", binding)
	}
}
