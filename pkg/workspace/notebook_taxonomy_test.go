package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grovetools/core/config"
)

// This file is the workspace-side half of the repo→notebook resolver
// characterization suite. Its twin is core/config/notebook_taxonomy_test.go,
// which drives the SAME taxonomy through the config-side resolver. The two
// tables are deliberately parallel: the case ids match one-for-one so a
// divergence between the two resolvers is a diff between two named rows, not a
// judgement call.
//
// The node shapes mirror what GetProjectByPath / transform.go produce for each
// worktree kind — resolution consumes the node, so the node is the fixture.

// notebookTaxonomyFixture is the directory layout both halves of the suite
// build. Every case in the taxonomy names paths inside it.
type notebookTaxonomyFixture struct {
	root string

	// Groves.
	codeGrove   string // grove "code",   notebook "codenb", enabled
	silentGrove string // grove "silent", notebook "",       enabled
	offGrove    string // grove "off",    notebook "offnb",  DISABLED

	// Inhabitants.
	plainRepo    string // plain repo inside the "code" grove
	eco          string // ecosystem root inside "code", carrying a card (cardnb)
	ecoSub       string // sub-project of the carded ecosystem
	ecoNoCard    string // ecosystem root inside "code", no card
	ecoNoCardSub string
	silentRepo   string
	offRepo      string
	outsideRepo  string // repo under no grove at all

	// XDG worktrees (outside every grove path).
	ecoWorktree    string // ecosystem worktree container
	ecoWorktreeSub string // sub-repo worktree inside the container
	plainWorktree  string // standalone-project worktree

	// Notebook storage trees.
	notebooks map[string]string
}

func buildNotebookTaxonomyFixture(t *testing.T) *notebookTaxonomyFixture {
	t.Helper()

	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)

	f := &notebookTaxonomyFixture{
		root:      root,
		notebooks: map[string]string{},
	}

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
	f.ecoWorktreeSub = mk("worktrees", "eco-0bd46c64", "plan", "eco")
	f.plainWorktree = mk("worktrees", "plain-0bd46c64", "feature")

	for _, name := range []string{"codenb", "cardnb", "nb", "offnb", "silentnb"} {
		f.notebooks[name] = mk("notebooks", name)
	}

	// The carded ecosystem: an ecosystem manifest whose [ecosystem] table binds
	// a DIFFERENT notebook ("cardnb") than the grove entry ("codenb"). That
	// disagreement is the whole point — it is what makes the card precedence
	// rung observable.
	writeFile(t, filepath.Join(f.eco, "grove.toml"), `name = "eco"
workspaces = ["*"]

[ecosystem]
id = "01J8CARDCARDCARDCARDCARDCA"
layout = "superrepo"

[ecosystem.notebooks.cardnb]
default = true
`)
	writeFile(t, filepath.Join(f.ecoNoCard, "grove.toml"), `name = "eco-nocard"
workspaces = ["*"]
`)
	// The worktree container is a system-written aggregator: workspaces, no
	// card. Its identity has to come from its owner.
	writeFile(t, filepath.Join(f.ecoWorktree, "grove.toml"), "workspaces = [\"*\"]\n")

	return f
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func (f *notebookTaxonomyFixture) config() *config.Config {
	enabled := true
	disabled := false

	defs := map[string]*config.Notebook{}
	for name, dir := range f.notebooks {
		defs[name] = &config.Notebook{RootDir: dir}
	}

	return &config.Config{
		Groves: map[string]config.GroveSourceConfig{
			"code":   {Path: f.codeGrove, Notebook: "codenb", NotebookRoot: f.notebooks["codenb"], Enabled: &enabled},
			"silent": {Path: f.silentGrove, Enabled: &enabled},
			"off":    {Path: f.offGrove, Notebook: "offnb", NotebookRoot: f.notebooks["offnb"], Enabled: &disabled},
		},
		Notebooks: &config.NotebooksConfig{
			Definitions: defs,
			Rules:       &config.NotebookRules{Default: "nb"},
		},
	}
}

// notebookTaxonomyCase is one row of the shared taxonomy. The node fields are
// what GetProjectByPath would produce for that shape.
type notebookTaxonomyCase struct {
	id   string
	node *WorkspaceNode
	// want is the notebook name the unified resolver must produce.
	want string
	// why documents the rung that decides the row.
	why string
}

func notebookTaxonomyCases(f *notebookTaxonomyFixture) []notebookTaxonomyCase {
	return []notebookTaxonomyCase{
		{
			id:   "plain-repo-in-grove",
			node: &WorkspaceNode{Name: "plain", Path: f.plainRepo, Kind: KindStandaloneProject},
			want: "codenb",
			why:  "grove entry for ~/code",
		},
		{
			id:   "grove-root-itself",
			node: &WorkspaceNode{Name: "code", Path: f.codeGrove, Kind: KindStandaloneProject},
			want: "codenb",
			why:  "the grove path itself matches its own entry",
		},
		{
			id:   "repo-outside-any-grove",
			node: &WorkspaceNode{Name: "repo", Path: f.outsideRepo, Kind: KindNonGroveRepo},
			want: "nb",
			why:  "no rung matches; Notebooks.Rules.Default",
		},
		{
			id: "ecosystem-root-with-card",
			node: &WorkspaceNode{
				Name: "eco", Path: f.eco, Kind: KindEcosystemRoot,
				RootEcosystemPath: f.eco,
			},
			want: "codenb",
			why:  "compiled code-root binding bypasses the stale ecosystem card",
		},
		{
			id: "ecosystem-sub-project-with-card",
			node: &WorkspaceNode{
				Name: "sub", Path: f.ecoSub, Kind: KindEcosystemSubProject,
				ParentEcosystemPath: f.eco, RootEcosystemPath: f.eco,
			},
			want: "codenb",
			why:  "compiled code-root binding bypasses the enclosing stale card",
		},
		{
			id: "ecosystem-root-without-card",
			node: &WorkspaceNode{
				Name: "eco-nocard", Path: f.ecoNoCard, Kind: KindEcosystemRoot,
				RootEcosystemPath: f.ecoNoCard,
			},
			want: "codenb",
			why:  "no card; grove entry",
		},
		{
			id: "ecosystem-sub-project-without-card",
			node: &WorkspaceNode{
				Name: "sub", Path: f.ecoNoCardSub, Kind: KindEcosystemSubProject,
				ParentEcosystemPath: f.ecoNoCard, RootEcosystemPath: f.ecoNoCard,
			},
			want: "codenb",
			why:  "no card; grove entry",
		},
		{
			id: "ecosystem-worktree-xdg",
			node: &WorkspaceNode{
				Name: "plan", Path: f.ecoWorktree, Kind: KindEcosystemWorktree,
				ParentProjectPath: f.eco, ParentEcosystemPath: f.eco, RootEcosystemPath: f.eco,
			},
			want: "codenb",
			why:  "worktree inherits its owner's compiled code-root binding",
		},
		{
			id: "ecosystem-worktree-xdg-anchored",
			node: &WorkspaceNode{
				Name: "plan", Path: f.ecoWorktree, Kind: KindEcosystemWorktree,
				ParentProjectPath: f.ecoSub, ParentEcosystemPath: f.eco, RootEcosystemPath: f.eco,
			},
			want: "codenb",
			why:  "anchored container inherits the owner's compiled code-root binding",
		},
		{
			id: "ecosystem-worktree-sub-project",
			node: &WorkspaceNode{
				Name: "eco", Path: f.ecoWorktreeSub, Kind: KindEcosystemWorktreeSubProject,
				ParentEcosystemPath: f.ecoWorktree, RootEcosystemPath: f.eco,
			},
			want: "codenb",
			why:  "root ecosystem's compiled code-root binding",
		},
		{
			id: "standalone-project-worktree-xdg",
			node: &WorkspaceNode{
				Name: "feature", Path: f.plainWorktree, Kind: KindStandaloneProjectWorktree,
				ParentProjectPath: f.plainRepo,
			},
			want: "codenb",
			why:  "owner repo is under the grove",
		},
		{
			id: "bare-notebook-storage-dir",
			node: &WorkspaceNode{
				Name: "cardnb", Path: filepath.Join(f.notebooks["cardnb"], "workspaces", "x"),
				Kind: KindNonGroveRepo,
			},
			want: "nb",
			why:  "raw notebook-root containment is not a binding; recorded default wins",
		},
		{
			id:   "repo-in-disabled-grove",
			node: &WorkspaceNode{Name: "repo", Path: f.offRepo, Kind: KindStandaloneProject},
			want: "nb",
			why:  "a disabled grove binds nothing; Notebooks.Rules.Default",
		},
		{
			id:   "repo-in-grove-with-no-notebook",
			node: &WorkspaceNode{Name: "repo", Path: f.silentRepo, Kind: KindStandaloneProject},
			want: "nb",
			why:  "grove declares no notebook; Notebooks.Rules.Default",
		},
	}
}

// TestNotebookTaxonomy_WorkspaceSide pins the notebook name the workspace-side
// resolver assigns for every shape in the taxonomy.
func TestNotebookTaxonomy_WorkspaceSide(t *testing.T) {
	f := buildNotebookTaxonomyFixture(t)
	cfg := f.config()

	for _, tc := range notebookTaxonomyCases(f) {
		t.Run(tc.id, func(t *testing.T) {
			node := *tc.node
			applyNotebookBinding(&node, cfg)
			assert.Equal(t, tc.want, node.NotebookName, "%s: %s", tc.id, tc.why)
		})
	}
}

// TestNotebookTaxonomy_WorkspaceSideEdges covers the config-shape edges that
// are not per-node: no groves at all, and a nil config.
func TestNotebookTaxonomy_WorkspaceSideEdges(t *testing.T) {
	f := buildNotebookTaxonomyFixture(t)

	t.Run("no groves configured still resolves the default", func(t *testing.T) {
		cfg := &config.Config{
			Notebooks: &config.NotebooksConfig{
				Rules: &config.NotebookRules{Default: "nb"},
			},
		}
		node := &WorkspaceNode{Name: "plain", Path: f.plainRepo, Kind: KindStandaloneProject}
		applyNotebookBinding(node, cfg)
		assert.Equal(t, "nb", node.NotebookName)
	})

	t.Run("no groves configured ignores a stale card", func(t *testing.T) {
		cfg := &config.Config{
			Notebooks: &config.NotebooksConfig{
				Rules: &config.NotebookRules{Default: "nb"},
			},
		}
		node := &WorkspaceNode{
			Name: "eco", Path: f.eco, Kind: KindEcosystemRoot,
			RootEcosystemPath: f.eco,
		}
		applyNotebookBinding(node, cfg)
		assert.Equal(t, "nb", node.NotebookName)
	})

	t.Run("nil config leaves the node untouched", func(t *testing.T) {
		node := &WorkspaceNode{Name: "plain", Path: f.plainRepo, Kind: KindStandaloneProject, NotebookName: "keep"}
		applyNotebookBinding(node, nil)
		assert.Equal(t, "keep", node.NotebookName)
	})
}
