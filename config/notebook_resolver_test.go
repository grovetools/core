package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolveNotebook_Precedence walks the ladder one rung at a time: each
// sub-test removes the rung above it and asserts the next one takes over.
func TestResolveNotebook_Precedence(t *testing.T) {
	f := buildNBTaxonomyFixture(t)

	t.Run("compiled binding bypasses stale ecosystem card", func(t *testing.T) {
		got := ResolveNotebook(NotebookQuery{Path: f.ecoSub}, f.config())
		assert.Equal(t, "codenb", got.Notebook)
		assert.Equal(t, f.notebooks["codenb"], got.NotebookRoot)
		assert.Equal(t, NotebookSourceGrove, got.Source)
	})

	t.Run("grove entry when there is no card", func(t *testing.T) {
		got := ResolveNotebook(NotebookQuery{Path: f.ecoNoCardSub}, f.config())
		assert.Equal(t, "codenb", got.Notebook)
		assert.Equal(t, NotebookSourceGrove, got.Source)
	})

	t.Run("raw definition containment is not a binding", func(t *testing.T) {
		got := ResolveNotebook(NotebookQuery{
			Path: filepath.Join(f.notebooks["cardnb"], "workspaces", "x"),
		}, f.config())
		assert.Equal(t, "nb", got.Notebook)
		assert.Equal(t, NotebookSourceDefault, got.Source)
	})

	t.Run("default when nothing matches", func(t *testing.T) {
		got := ResolveNotebook(NotebookQuery{Path: f.outsideRepo}, f.config())
		assert.Equal(t, "nb", got.Notebook)
		assert.Equal(t, NotebookSourceDefault, got.Source)
	})

	t.Run("no binding at all", func(t *testing.T) {
		cfg := f.config()
		cfg.Notebooks.Rules = nil
		got := ResolveNotebook(NotebookQuery{Path: f.outsideRepo}, cfg)
		assert.Empty(t, got.Notebook)
		assert.Equal(t, NotebookSourceNone, got.Source)
	})

	t.Run("empty query resolves nothing", func(t *testing.T) {
		got := ResolveNotebook(NotebookQuery{}, f.config())
		assert.Empty(t, got.Notebook)
		assert.Equal(t, NotebookSourceNone, got.Source)
	})
}

func TestResolveNotebook_CompiledNotebookRootBridge(t *testing.T) {
	recordedRoot := filepath.Join(t.TempDir(), "recorded-notes")
	cfg := &Config{
		Groves: map[string]GroveSourceConfig{
			"code": {
				Path:         filepath.Join(t.TempDir(), "code"),
				Notebook:     "recorded",
				NotebookRoot: recordedRoot,
			},
		},
		// Keep a conflicting same-name definition to prove literal root identity:
		// re-resolving "recorded" by name would choose this decoy.
		Notebooks: &NotebooksConfig{Definitions: map[string]*Notebook{
			"recorded": {RootDir: filepath.Join(t.TempDir(), "same-name-decoy")},
		}},
	}

	got := ResolveNotebook(NotebookQuery{Path: filepath.Join(recordedRoot, "notespaces", "project")}, cfg)
	assert.Equal(t, "recorded", got.Notebook)
	assert.Equal(t, recordedRoot, got.NotebookRoot)
	assert.Equal(t, NotebookSourceNotebookRoot, got.Source)
}

// TestResolveNotebook_OwnerPaths pins the worktree behavior: the query path
// binds nothing, so identity comes from the owner — and the owner's rung
// ordering is the same ladder, not a special case.
func TestResolveNotebook_OwnerPaths(t *testing.T) {
	f := buildNBTaxonomyFixture(t)
	cfg := f.config()

	t.Run("out-of-grove worktree inherits the owner's compiled binding", func(t *testing.T) {
		got := ResolveNotebook(NotebookQuery{
			Path:       f.ecoWorktree,
			OwnerPaths: []string{f.eco},
		}, cfg)
		assert.Equal(t, "codenb", got.Notebook)
		assert.Equal(t, NotebookSourceGrove, got.Source)
		assert.Equal(t, f.eco, got.MatchedPath)
	})

	t.Run("out-of-grove worktree inherits the owner's grove", func(t *testing.T) {
		got := ResolveNotebook(NotebookQuery{
			Path:       f.ecoWorktree,
			OwnerPaths: []string{f.plainRepo},
		}, cfg)
		assert.Equal(t, "codenb", got.Notebook)
		assert.Equal(t, NotebookSourceGrove, got.Source)
	})

	t.Run("candidate order selects among compiled bindings", func(t *testing.T) {
		got := ResolveNotebook(NotebookQuery{
			Path:       f.ecoWorktree,
			OwnerPaths: []string{f.plainRepo, f.eco},
		}, cfg)
		assert.Equal(t, "codenb", got.Notebook)
		assert.Equal(t, f.plainRepo, got.MatchedPath)
	})

	t.Run("the query path's own grove is reported even when an owner decides", func(t *testing.T) {
		got := ResolveNotebook(NotebookQuery{
			Path:       f.ecoNoCardSub,
			OwnerPaths: []string{f.eco},
		}, cfg)
		assert.Equal(t, "codenb", got.Notebook, "the query path's compiled binding is literal rung 0")
		assert.Equal(t, "code", got.GroveName)
		assert.Equal(t, f.codeGrove, got.GroveRoot)
	})
}

// TestResolveNotebook_DisabledGrove pins the rule the two predecessors
// disagreed on: `enabled = false` binds nothing.
func TestResolveNotebook_DisabledGrove(t *testing.T) {
	f := buildNBTaxonomyFixture(t)
	got := ResolveNotebook(NotebookQuery{Path: f.offRepo}, f.config())
	assert.Equal(t, "nb", got.Notebook)
	assert.Equal(t, NotebookSourceDefault, got.Source)
}

// TestResolveNotebook_GroveRootSpelling guards the field that feeds workspace
// directory names: GroveRoot must come back in its original spelling, never in
// the lowercased comparison form.
func TestResolveNotebook_GroveRootSpelling(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)

	grove := filepath.Join(root, "CodeGrove")
	project := filepath.Join(grove, "MyApp")
	require.NoError(t, os.MkdirAll(project, 0o755))

	cfg := &Config{
		Groves: map[string]GroveSourceConfig{
			"code": {Path: grove, Notebook: "codenb"},
		},
		Notebooks: &NotebooksConfig{
			Definitions: map[string]*Notebook{"codenb": {RootDir: filepath.Join(root, "nb")}},
		},
	}

	got := ResolveNotebook(NotebookQuery{Path: project}, cfg)
	assert.Equal(t, "codenb", got.Notebook)
	assert.Equal(t, grove, got.GroveRoot)

	ctx := notebookWorkspaceContext(project, cfg)
	require.NotNil(t, ctx)
	assert.Equal(t, "MyApp", ctx.workspaceName, "the workspace name keeps the project's own casing")
}

// TestResolveNotebook_NonExistentPaths covers the ordinary case of resolving a
// path that is not on disk yet — a grove entry pointing at a directory nobody
// has created, or a project about to be cloned.
func TestResolveNotebook_NonExistentPaths(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	grove := filepath.Join(root, "code")
	require.NoError(t, os.MkdirAll(grove, 0o755))

	cfg := &Config{
		Groves: map[string]GroveSourceConfig{
			"code":    {Path: grove, Notebook: "codenb"},
			"missing": {Path: filepath.Join(root, "not-created"), Notebook: "ghostnb"},
		},
		Notebooks: &NotebooksConfig{Rules: &NotebookRules{Default: "nb"}},
	}

	t.Run("uncreated project under an existing grove", func(t *testing.T) {
		got := ResolveNotebook(NotebookQuery{Path: filepath.Join(grove, "future")}, cfg)
		assert.Equal(t, "codenb", got.Notebook)
	})

	t.Run("uncreated project under an uncreated grove", func(t *testing.T) {
		got := ResolveNotebook(NotebookQuery{Path: filepath.Join(root, "not-created", "future")}, cfg)
		assert.Equal(t, "ghostnb", got.Notebook)
	})
}

// TestResolveNotebook_StaleCardIgnored proves repository-side routing no longer
// participates after the recorded-routing cutover, whether valid or broken.
func TestResolveNotebook_StaleCardIgnored(t *testing.T) {
	f := buildNBTaxonomyFixture(t)
	cfg := f.config()

	require.NoError(t, os.WriteFile(filepath.Join(f.eco, "grove.toml"), []byte("this is not = = toml\n"), 0o644))
	got := ResolveNotebook(NotebookQuery{Path: f.ecoSub}, cfg)
	assert.Equal(t, "codenb", got.Notebook)
	assert.Equal(t, NotebookSourceGrove, got.Source)
}
