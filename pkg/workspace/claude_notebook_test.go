package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grovetools/core/config"
)

// centralizedConfig builds a config whose notebook "grovetools" is centralized
// at rootDir, so GetNotesDir renders the default workspaces/<name>/<type>
// template. This lets us exercise notebookRootForNode without real discovery.
func centralizedConfig(rootDir string) *config.Config {
	return &config.Config{
		Notebooks: &config.NotebooksConfig{
			Definitions: map[string]*config.Notebook{
				"grovetools": {RootDir: rootDir},
			},
		},
	}
}

// notebookRootForNode should return the paired notebook ROOT (workspaces/<repo>),
// i.e. the parent of the per-note-type dirs.
func TestNotebookRootForNode_CentralizedRoot(t *testing.T) {
	rootDir := t.TempDir()
	locator := NewNotebookLocator(centralizedConfig(rootDir))

	node := &WorkspaceNode{
		Name:         "core",
		Path:         "/somewhere/core",
		Kind:         KindEcosystemSubProject,
		NotebookName: "grovetools",
	}

	root := notebookRootForNode(locator, node)
	want := filepath.Join(rootDir, "workspaces", "core")
	assert.Equal(t, want, root, "root is the parent of workspaces/<repo>/inbox")
}

// Two distinct repos resolve to distinct notebook roots.
func TestNotebookRootForNode_DistinctPerRepo(t *testing.T) {
	rootDir := t.TempDir()
	locator := NewNotebookLocator(centralizedConfig(rootDir))

	core := &WorkspaceNode{Name: "core", Kind: KindEcosystemSubProject, NotebookName: "grovetools"}
	nb := &WorkspaceNode{Name: "nb", Kind: KindEcosystemSubProject, NotebookName: "grovetools"}

	assert.Equal(t, filepath.Join(rootDir, "workspaces", "core"), notebookRootForNode(locator, core))
	assert.Equal(t, filepath.Join(rootDir, "workspaces", "nb"), notebookRootForNode(locator, nb))
}

// resolveRepoNode (and therefore resolveNotebookDirsForRepos) silently skips an
// unresolvable repo: a subdir that doesn't exist on disk yields no node and no
// dir, never an error.
func TestResolveNotebookDirsForRepos_SkipsUnresolvableAndEmpty(t *testing.T) {
	worktree := t.TempDir() // empty: no member-repo subdirs exist on disk

	// "" is skipped by name; "ghost" can't be discovered (no dir) -> skipped.
	dirs := resolveNotebookDirsForRepos(worktree, []string{"", "ghost"}, nil)
	assert.Empty(t, dirs, "empty names and unresolvable repos contribute nothing")
}

// resolveRepoNode returns nil for a non-existent subdir (the skip path that
// keeps seeding best-effort). Provider is nil so it falls through to discovery.
func TestResolveRepoNode_NilForMissingPath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	assert.Nil(t, resolveRepoNode(missing, nil), "missing repo path resolves to no node")
}

// SeedNotebookDirsForWorktree is a no-op (no error, no file) when no repo
// resolves to a notebook dir — the common best-effort path for an empty or
// not-yet-populated worktree.
func TestSeedNotebookDirsForWorktree_NoResolvableReposNoOp(t *testing.T) {
	worktree := t.TempDir()
	require.NoError(t, SeedNotebookDirsForWorktree(worktree, []string{"ghost"}, nil))

	_, err := os.Stat(filepath.Join(worktree, ".claude", "settings.local.json"))
	assert.True(t, os.IsNotExist(err), "no settings file created when nothing resolves")
}

// NOTE ON COVERAGE: full end-to-end resolution (member subdir -> WorkspaceNode
// with the anchored-worktree NotebookName assignment -> notebook root) depends
// on real grove discovery (GetProjectByPath walks grove.toml / .git on disk),
// which is too heavy for a focused unit test. We test the pure pieces here:
// notebookRootForNode (the locator math) and the dedup/skip behavior of
// resolveNotebookDirsForRepos / resolveRepoNode. The dedupe-and-sort of the
// final union is covered by claudenotebook's seeder tests.
