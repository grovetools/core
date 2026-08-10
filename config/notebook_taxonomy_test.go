package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file is the config-side half of the repo→notebook resolver
// characterization suite; its twin is core/pkg/workspace/notebook_taxonomy_test.go.
// The case ids are shared between the two so that a behavior difference between
// the config-side and workspace-side entry points is a diff between two named
// rows rather than a judgement call.
//
// The config-side entry point answers a different question than the
// workspace-side one — "which notebook workspace directory holds this project's
// config?" — so it additionally requires a GROVE match (that is what supplies
// the workspace name). Rows that resolve a notebook name but no grove therefore
// legitimately return nil here while resolving fine on the workspace side.

type nbTaxonomyFixture struct {
	root string

	codeGrove   string
	silentGrove string
	offGrove    string

	plainRepo    string
	eco          string
	ecoSub       string
	ecoNoCard    string
	ecoNoCardSub string
	silentRepo   string
	offRepo      string
	outsideRepo  string

	ecoWorktree string

	notebooks map[string]string
}

func buildNBTaxonomyFixture(t *testing.T) *nbTaxonomyFixture {
	t.Helper()

	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)

	f := &nbTaxonomyFixture{root: root, notebooks: map[string]string{}}

	mk := func(parts ...string) string {
		p := filepath.Join(append([]string{root}, parts...)...)
		require.NoError(t, os.MkdirAll(p, 0o755))
		return p
	}

	f.codeGrove = mk("code")
	f.silentGrove = mk("silent")
	f.offGrove = mk("off")

	f.plainRepo = mk("code", "plain")
	f.eco = mk("code", "eco")
	f.ecoSub = mk("code", "eco", "sub")
	f.ecoNoCard = mk("code", "eco-nocard")
	f.ecoNoCardSub = mk("code", "eco-nocard", "sub")
	f.silentRepo = mk("silent", "repo")
	f.offRepo = mk("off", "repo")
	f.outsideRepo = mk("outside", "repo")
	f.ecoWorktree = mk("worktrees", "eco-0bd46c64", "plan")

	for _, name := range []string{"codenb", "cardnb", "nb", "offnb", "silentnb"} {
		f.notebooks[name] = mk("notebooks", name)
	}

	require.NoError(t, os.WriteFile(filepath.Join(f.eco, "grove.toml"), []byte(`name = "eco"
workspaces = ["*"]

[ecosystem]
id = "01J8CARDCARDCARDCARDCARDCA"
layout = "superrepo"

[ecosystem.notebooks.cardnb]
default = true
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(f.ecoNoCard, "grove.toml"), []byte(`name = "eco-nocard"
workspaces = ["*"]
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(f.ecoWorktree, "grove.toml"), []byte("workspaces = [\"*\"]\n"), 0o644))

	return f
}

func (f *nbTaxonomyFixture) config() *Config {
	enabled := true
	disabled := false

	defs := map[string]*Notebook{}
	for name, dir := range f.notebooks {
		defs[name] = &Notebook{RootDir: dir}
	}

	return &Config{
		Groves: map[string]GroveSourceConfig{
			"code":   {Path: f.codeGrove, Notebook: "codenb", NotebookRoot: f.notebooks["codenb"], Enabled: &enabled},
			"silent": {Path: f.silentGrove, Enabled: &enabled},
			"off":    {Path: f.offGrove, Notebook: "offnb", NotebookRoot: f.notebooks["offnb"], Enabled: &disabled},
		},
		Notebooks: &NotebooksConfig{
			Definitions: defs,
			Rules:       &NotebookRules{Default: "nb"},
		},
	}
}

// TestNotebookTaxonomy_ConfigSide pins the notebook workspace directory the
// config-side entry point resolves for every shape in the taxonomy.
func TestNotebookTaxonomy_ConfigSide(t *testing.T) {
	f := buildNBTaxonomyFixture(t)
	cfg := f.config()

	cases := []struct {
		id           string
		path         string
		wantNil      bool
		wantNotebook string // key into f.notebooks
		wantWorkspac string
		why          string
	}{
		{
			id: "plain-repo-in-grove", path: f.plainRepo,
			wantNotebook: "codenb", wantWorkspac: "plain",
			why: "grove entry for ~/code",
		},
		{
			id: "grove-root-itself", path: f.codeGrove, wantNil: true,
			why: "the grove root has no workspace name of its own",
		},
		{
			id: "repo-outside-any-grove", path: f.outsideRepo, wantNil: true,
			why: "no grove match, so no workspace name",
		},
		{
			id: "ecosystem-root-with-stale-card", path: f.eco,
			wantNotebook: "codenb", wantWorkspac: "eco",
			why: "compiled code-root binding bypasses the stale ecosystem card",
		},
		{
			id: "ecosystem-sub-project-with-stale-card", path: f.ecoSub,
			wantNotebook: "codenb", wantWorkspac: filepath.Join("eco", "sub"),
			why: "compiled code-root binding bypasses the enclosing ecosystem card",
		},
		{
			id: "ecosystem-root-without-card", path: f.ecoNoCard,
			wantNotebook: "codenb", wantWorkspac: "eco-nocard",
			why: "no card; grove entry",
		},
		{
			id: "ecosystem-sub-project-without-card", path: f.ecoNoCardSub,
			wantNotebook: "codenb", wantWorkspac: filepath.Join("eco-nocard", "sub"),
			why: "no card; grove entry",
		},
		{
			id: "ecosystem-worktree-xdg", path: f.ecoWorktree, wantNil: true,
			why: "the config-side caller supplies no owner candidates, so an out-of-grove worktree binds nothing",
		},
		{
			id: "bare-notebook-storage-dir", path: filepath.Join(f.notebooks["cardnb"], "workspaces", "x"),
			wantNil: true,
			why:     "a notebook root_dir match names a notebook but not a grove workspace",
		},
		{
			id: "repo-in-disabled-grove", path: f.offRepo, wantNil: true,
			why: "a disabled grove binds nothing",
		},
		{
			id: "repo-in-grove-with-no-notebook", path: f.silentRepo,
			wantNotebook: "nb", wantWorkspac: "repo",
			why: "grove declares no notebook; Notebooks.Rules.Default",
		},
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			ctx := notebookWorkspaceContext(tc.path, cfg)
			if tc.wantNil {
				assert.Nil(t, ctx, "%s: %s", tc.id, tc.why)
				return
			}
			require.NotNil(t, ctx, "%s: %s", tc.id, tc.why)
			assert.Equal(t, f.notebooks[tc.wantNotebook], ctx.notebookRootDir, "%s: %s", tc.id, tc.why)
			assert.Equal(t, tc.wantWorkspac, ctx.workspaceName, "%s: %s", tc.id, tc.why)
		})
	}
}
