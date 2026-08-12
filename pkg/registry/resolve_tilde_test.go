package registry

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/grovetools/core/config"
)

// TestResolveWorkspaceRootExpandsADeclaredDefaultNotebookRoot covers the
// default rung, which returns a RECORDED notebook root while the rung above it
// returns an already-resolved one. The two must agree in kind.
//
// The failure mode is silent and total: ResolveWorkspaceRoot joins whatever it
// gets onto "notespaces/<name>", so a declared "~/notebooks/nb" produces
// "~/notebooks/nb/notespaces/alpha" — a path filepath.Join is perfectly happy
// to build, os.Stat reports as merely absent, and every caller then treats as
// "this workspace does not exist yet".
//
// The fixture spells the root the way notebooks.toml and the legacy compat
// file actually spell it. An absolute t.TempDir() fixture — the shape of every
// other test in this tree — cannot fail against the broken code, which is why
// this bug shipped.
func TestResolveWorkspaceRootExpandsADeclaredDefaultNotebookRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := &config.Config{Notebooks: &config.NotebooksConfig{
		Definitions: map[string]*config.Notebook{"nb": {RootDir: "~/notebooks/nb"}},
		Rules:       &config.NotebookRules{Default: "nb"},
	}}

	got, err := ResolveWorkspaceRoot(cfg, "alpha")
	if err != nil {
		t.Fatalf("ResolveWorkspaceRoot: %v", err)
	}
	want := filepath.Join(home, "notebooks", "nb", "notespaces", "alpha")
	if got != want {
		t.Errorf("ResolveWorkspaceRoot = %q, want %q", got, want)
	}
	if strings.Contains(got, "~") {
		t.Errorf("resolved workspace root still carries a declared spelling: %q", got)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("resolved workspace root is not absolute: %q", got)
	}
}

// TestResolveWorkspaceRootAgreesAcrossRungs pins the invariant behind the fix:
// the compiled code-root rung and the default-notebook rung must resolve the
// same notebook to the same directory. A machine where they disagree is one
// where a workspace's location depends on which rung answered — the wrong-root
// class P2 exists to eliminate.
func TestResolveWorkspaceRootAgreesAcrossRungs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	resolved := filepath.Join(home, "notebooks", "nb")

	viaDefault := &config.Config{Notebooks: &config.NotebooksConfig{
		Definitions: map[string]*config.Notebook{"nb": {RootDir: "~/notebooks/nb"}},
		Rules:       &config.NotebookRules{Default: "nb"},
	}}
	viaGrove := &config.Config{
		Groves: map[string]config.GroveSourceConfig{
			"alpha": {Notebook: "nb", NotebookRoot: resolved},
		},
		Notebooks: viaDefault.Notebooks,
	}

	fromDefault, err := ResolveWorkspaceRoot(viaDefault, "alpha")
	if err != nil {
		t.Fatalf("default rung: %v", err)
	}
	fromGrove, err := ResolveWorkspaceRoot(viaGrove, "alpha")
	if err != nil {
		t.Fatalf("compiled rung: %v", err)
	}
	if fromDefault != fromGrove {
		t.Errorf("rungs disagree: default rung = %q, compiled rung = %q", fromDefault, fromGrove)
	}
}
