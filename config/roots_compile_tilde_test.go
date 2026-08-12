package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The declared-spelling regression suite.
//
// roots.toml and notebooks.toml store paths AS DECLARED — `~/notebooks/nb`,
// `${VAR}/code` — and expansion is the consumer's job. Two shipped bugs came
// from consumers forgetting, and both survived review because every fixture in
// the tree writes an absolute t.TempDir() path. A regression test with an
// absolute path cannot catch this class: the bug IS the tilde.
//
// So these tests declare paths the way a real machine's config does, with HOME
// pointed at the sandbox, and assert the compiled view hands out paths that
// can actually be used.

// tildeHome points HOME at a fresh sandbox and returns it, so `~/x` in a
// fixture resolves to a real directory this test owns.
func tildeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

// TestCompiledNotebookRootsAreUsablePathsNotDeclaredSpellings is the load-time
// half of the fix. A consumer reading Definitions[...].RootDir must never
// receive a path it has to remember to expand.
//
// The legacy branch is the one that regressed: when notebooks.toml is absent,
// compileCodeRootTable leaves Definitions exactly as the merged legacy layer
// built them, and notebooks.legacy-compat.toml writes root_dir as
// '~/notebooks/<name>'. That is the shape a pre-migration machine, a sandbox,
// or a satellite seeded without the recorded pair actually has.
func TestCompiledNotebookRootsAreUsablePathsNotDeclaredSpellings(t *testing.T) {
	for _, tc := range []struct {
		name      string
		notebooks string // notebooks.toml body; "" means the file does not exist
	}{
		{name: "recorded notebooks.toml", notebooks: "default = \"nb\"\n[notebooks.nb]\nroot = \"~/notebooks/nb\"\n"},
		{name: "legacy definitions only", notebooks: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := tildeHome(t)
			rp, np := writeRecordedPair(t, "", tc.notebooks)

			// The legacy layer, spelled the way the compat writer spells it.
			legacy := &Config{Notebooks: &NotebooksConfig{
				Definitions: map[string]*Notebook{"nb": {RootDir: "~/notebooks/nb"}},
				Rules:       &NotebookRules{Default: "nb", Global: &GlobalNotebookConfig{RootDir: "~/notebooks/global"}},
			}}

			got, err := compileCodeRootsFromPaths(legacy, rp, np)
			if err != nil {
				t.Fatal(err)
			}
			definition := got.Notebooks.Definitions["nb"]
			if definition == nil {
				t.Fatalf("notebook nb vanished from the compiled view: %+v", got.Notebooks.Definitions)
			}
			if want := filepath.Join(home, "notebooks", "nb"); definition.RootDir != want {
				t.Errorf("compiled RootDir = %q, want %q — a declared spelling reached a consumer", definition.RootDir, want)
			}
			if strings.HasPrefix(definition.RootDir, "~") {
				t.Errorf("compiled RootDir is still a declared spelling: %q", definition.RootDir)
			}
			// filepath.Abs is the trap: it does not reject a tilde path, it
			// prefixes cwd. Prove the compiled value survives it unchanged.
			if abs, err := filepath.Abs(definition.RootDir); err != nil || abs != definition.RootDir {
				t.Errorf("filepath.Abs(%q) = %q, %v; a usable root is already absolute", definition.RootDir, abs, err)
			}
			if global := got.Notebooks.Rules.Global; global != nil {
				if want := filepath.Join(home, "notebooks", "global"); global.RootDir != want {
					t.Errorf("compiled global RootDir = %q, want %q", global.RootDir, want)
				}
			}
		})
	}
}

// TestCompileDoesNotMutateTheLayerItWasGiven guards the copy-on-write the
// normalization has to honor: LoadLayered hands out source-attributed layers
// separately, and a layer rewritten in place would report a path the user
// never wrote in the "where did this come from" surfaces.
func TestCompileDoesNotMutateTheLayerItWasGiven(t *testing.T) {
	tildeHome(t)
	rp, np := writeRecordedPair(t, "", "")

	legacy := &Config{Notebooks: &NotebooksConfig{
		Definitions: map[string]*Notebook{"nb": {RootDir: "~/notebooks/nb"}},
		Rules:       &NotebookRules{Default: "nb"},
	}}
	if _, err := compileCodeRootsFromPaths(legacy, rp, np); err != nil {
		t.Fatal(err)
	}
	if got := legacy.Notebooks.Definitions["nb"].RootDir; got != "~/notebooks/nb" {
		t.Fatalf("compile rewrote the source layer's declared spelling to %q", got)
	}
}

// TestExpandPathIsIdempotent is the property every call site relies on: a
// consumer that expands defensively must not corrupt an already-resolved path,
// which is what makes "expand at load AND at use" safe rather than redundant.
func TestExpandPathIsIdempotent(t *testing.T) {
	home := tildeHome(t)
	t.Setenv("NOTES", filepath.Join(home, "env-notes"))
	for _, declared := range []string{"~/notebooks/nb", "${NOTES}/x", filepath.Join(home, "absolute"), ""} {
		once := expandPath(declared)
		if twice := expandPath(once); twice != once {
			t.Errorf("expandPath(%q) = %q, expanding again = %q; must be idempotent", declared, once, twice)
		}
		if strings.HasPrefix(once, "~") {
			t.Errorf("expandPath(%q) left a tilde: %q", declared, once)
		}
	}
	if _, err := os.Stat(home); err != nil {
		t.Fatalf("sandbox home vanished: %v", err)
	}
}
