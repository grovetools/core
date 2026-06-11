package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grovetools/core/pkg/paths"
	// TODO: Update when testutil is available in grove-core
	// "testutil"
)

// TODO: Re-enable this test when testutil package is available in grove-core
/*
func TestWorktreeManager_ListWorktrees(t *testing.T) {
	tmpDir := t.TempDir()
	testutil.InitGitRepo(t, tmpDir)

	// Create a worktree
	worktreePath := filepath.Join(tmpDir, "feature-wt")
	cmd := exec.Command("git", "worktree", "add", worktreePath, "-b", "feature")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())

	manager := NewWorktreeManager()

	worktrees, err := manager.ListWorktrees(context.Background(), tmpDir)
	require.NoError(t, err)
	assert.Len(t, worktrees, 2) // Main + new worktree

	// Find the feature worktree
	var found bool
	for _, wt := range worktrees {
		if wt.Branch == "feature" {
			found = true
			assert.Contains(t, wt.Path, "feature-wt")
			break
		}
	}
	assert.True(t, found, "feature worktree should be found")
}
*/

// Helper function to initialize git repo for tests
// TODO: Remove this when testutil is available
func initGitRepoForTest(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	// Configure git user
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	// Create initial commit
	testFile := filepath.Join(dir, "README.md")
	require.NoError(t, os.WriteFile(testFile, []byte("# Test\n"), 0o644))

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())
}

// Test with temporary helper function
func TestWorktreeManager_ListWorktrees_Temp(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepoForTest(t, tmpDir)

	// Create a worktree
	worktreePath := filepath.Join(tmpDir, "feature-wt")
	cmd := exec.Command("git", "worktree", "add", worktreePath, "-b", "feature")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())

	manager := NewWorktreeManager()

	worktrees, err := manager.ListWorktrees(context.Background(), tmpDir)
	require.NoError(t, err)
	assert.Len(t, worktrees, 2) // Main + new worktree

	// Find the feature worktree
	var found bool
	for _, wt := range worktrees {
		if wt.Branch == "feature" {
			found = true
			assert.Contains(t, wt.Path, "feature-wt")
			break
		}
	}
	assert.True(t, found, "feature worktree should be found")
}

// setupXDGRepoForTest sandboxes the grove data dir and returns a
// symlink-resolved repo path so candidates compare equal to git's porcelain
// output (t.TempDir is a symlink on macOS).
func setupXDGRepoForTest(t *testing.T) string {
	t.Helper()
	dataHome, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("GROVE_HOME", "")
	repo, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	initGitRepoForTest(t, repo)
	return repo
}

// xdgTargetForTest builds an XDG-shaped target path. The identifier segment
// is arbitrary here — GetOrPrepareWorktreeAt only needs the
// WorktreesDir()/<identifier>/<name> shape (the workspace layer computes the
// real identifier).
func xdgTargetForTest(name string) string {
	return filepath.Join(paths.WorktreesDir(), "repo-fixture-id", name)
}

// TestGetOrPrepareWorktreeAt_Idempotency pins the four reuse rules: branch
// match anywhere (legacy preferred), candidate-path reuse, stale-entry
// cleanup in both layouts, and parent-dir creation for XDG targets.
func TestGetOrPrepareWorktreeAt_Idempotency(t *testing.T) {
	ctx := context.Background()

	t.Run("existing legacy worktree wins over XDG request", func(t *testing.T) {
		repo := setupXDGRepoForTest(t)
		m := NewWorktreeManager()

		legacyPath, created, err := m.GetOrPrepareWorktree(ctx, repo, "wt1", "branch1")
		require.NoError(t, err)
		require.True(t, created)

		target := xdgTargetForTest("wt1")
		got, created, err := m.GetOrPrepareWorktreeAt(ctx, repo, target, "branch1")
		require.NoError(t, err)
		assert.False(t, created, "must reuse the legacy worktree, not create a duplicate")
		assert.Equal(t, legacyPath, got)
		_, err = os.Stat(target)
		assert.True(t, os.IsNotExist(err), "no XDG directory may be created when legacy exists")
	})

	t.Run("existing XDG worktree reused on re-prepare", func(t *testing.T) {
		repo := setupXDGRepoForTest(t)
		m := NewWorktreeManager()

		target := xdgTargetForTest("wt1")
		got, created, err := m.GetOrPrepareWorktreeAt(ctx, repo, target, "branch1")
		require.NoError(t, err)
		require.True(t, created)
		require.Equal(t, target, got)

		again, created, err := m.GetOrPrepareWorktreeAt(ctx, repo, target, "branch1")
		require.NoError(t, err)
		assert.False(t, created)
		assert.Equal(t, target, again)

		worktrees, err := m.ListWorktrees(ctx, repo)
		require.NoError(t, err)
		assert.Len(t, worktrees, 2, "main + one worktree; re-prepare must not duplicate")
	})

	t.Run("existing XDG worktree found when legacy requested", func(t *testing.T) {
		repo := setupXDGRepoForTest(t)
		m := NewWorktreeManager()

		target := xdgTargetForTest("wt2")
		_, created, err := m.GetOrPrepareWorktreeAt(ctx, repo, target, "branch2")
		require.NoError(t, err)
		require.True(t, created)

		got, created, err := m.GetOrPrepareWorktree(ctx, repo, "wt2", "branch2")
		require.NoError(t, err)
		assert.False(t, created, "branch match must reuse the XDG worktree")
		assert.Equal(t, target, got)
	})

	t.Run("stale legacy entry cleaned when XDG requested", func(t *testing.T) {
		repo := setupXDGRepoForTest(t)
		m := NewWorktreeManager()

		legacyPath, _, err := m.GetOrPrepareWorktree(ctx, repo, "wt1", "branch1")
		require.NoError(t, err)
		require.NoError(t, os.RemoveAll(legacyPath))

		target := xdgTargetForTest("wt1")
		got, created, err := m.GetOrPrepareWorktreeAt(ctx, repo, target, "branch1")
		require.NoError(t, err)
		assert.True(t, created)
		assert.Equal(t, target, got)

		worktrees, err := m.ListWorktrees(ctx, repo)
		require.NoError(t, err)
		for _, wt := range worktrees {
			assert.NotEqual(t, legacyPath, wt.Path, "stale legacy entry must be pruned")
		}
	})

	t.Run("stale XDG entry cleaned on re-prepare", func(t *testing.T) {
		repo := setupXDGRepoForTest(t)
		m := NewWorktreeManager()

		target := xdgTargetForTest("wt1")
		_, created, err := m.GetOrPrepareWorktreeAt(ctx, repo, target, "branch1")
		require.NoError(t, err)
		require.True(t, created)
		require.NoError(t, os.RemoveAll(target))

		got, created, err := m.GetOrPrepareWorktreeAt(ctx, repo, target, "branch1")
		require.NoError(t, err)
		assert.True(t, created, "stale XDG entry must be cleaned and the worktree recreated")
		assert.Equal(t, target, got)

		worktrees, err := m.ListWorktrees(ctx, repo)
		require.NoError(t, err)
		assert.Len(t, worktrees, 2, "main + the recreated worktree")
	})
}

func TestWorktreeManager_ParseWorktreeList(t *testing.T) {
	manager := NewWorktreeManager()

	output := `worktree /path/to/main
HEAD abcdef1234567890
branch refs/heads/main

worktree /path/to/feature
HEAD 1234567890abcdef
branch refs/heads/feature

`

	worktrees := manager.parseWorktreeList(output)

	assert.Len(t, worktrees, 2)
	assert.Equal(t, "/path/to/main", worktrees[0].Path)
	assert.Equal(t, "main", worktrees[0].Branch)
	assert.Equal(t, "abcdef1234567890", worktrees[0].Commit)

	assert.Equal(t, "/path/to/feature", worktrees[1].Path)
	assert.Equal(t, "feature", worktrees[1].Branch)
}
