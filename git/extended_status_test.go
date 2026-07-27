package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetStatusNotARepository(t *testing.T) {
	// git prints "fatal: not a git repository" to STDERR; this guards the
	// ExitError.Stderr sniffing in GetStatus (there is no IsGitRepo pre-check
	// in front of it anymore).
	_, err := GetStatus(t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a git repository")
}

func TestGetExtendedStatusNotARepository(t *testing.T) {
	// Callers rely on a non-nil error to skip non-repo workspaces.
	_, err := GetExtendedStatus(t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a git repository")
}

func TestGetExtendedStatusIgnoresSubmodulePointerChanges(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	repo := filepath.Join(root, "repo")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	require.NoError(t, os.MkdirAll(repo, 0o755))
	setupGitRepo(t, sub)
	commitFile(t, sub, "file.txt", "one\n", "initial submodule commit")
	setupGitRepo(t, repo)
	runGitCommand(t, repo, "-c", "protocol.file.allow=always", "submodule", "add", sub, "child")
	runGitCommand(t, repo, "commit", "-m", "add submodule")

	commitFile(t, sub, "file.txt", "two\n", "move submodule")
	runGitCommand(t, filepath.Join(repo, "child"), "fetch")
	runGitCommand(t, filepath.Join(repo, "child"), "checkout", revParse(t, sub, "HEAD"))

	ext, err := GetExtendedStatus(repo)
	require.NoError(t, err)
	assert.False(t, ext.IsDirty)
	assert.Equal(t, 0, ext.LinesAdded)
	assert.Equal(t, 0, ext.LinesDeleted)
}

func TestGetExtendedStatusNumstat(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)
	commitFile(t, dir, "file.txt", "one\ntwo\nthree\n", "initial commit")

	t.Run("clean repo skips numstat and reports zero", func(t *testing.T) {
		ext, err := GetExtendedStatus(dir)
		require.NoError(t, err)
		assert.False(t, ext.IsDirty)
		assert.Equal(t, 0, ext.ModifiedCount)
		assert.Equal(t, 0, ext.StagedCount)
		assert.Equal(t, 0, ext.LinesAdded)
		assert.Equal(t, 0, ext.LinesDeleted)
	})

	t.Run("untracked-only repo still reports zero line counts", func(t *testing.T) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("new\n"), 0o644))
		defer func() { require.NoError(t, os.Remove(filepath.Join(dir, "untracked.txt"))) }()

		ext, err := GetExtendedStatus(dir)
		require.NoError(t, err)
		assert.True(t, ext.IsDirty)
		assert.Equal(t, 1, ext.UntrackedCount)
		assert.Equal(t, 0, ext.ModifiedCount)
		assert.Equal(t, 0, ext.StagedCount)
		assert.Equal(t, 0, ext.LinesAdded)
		assert.Equal(t, 0, ext.LinesDeleted)
	})

	t.Run("working-tree changes report line counts", func(t *testing.T) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("one\ntwo\nthree\nfour\nfive\n"), 0o644))

		ext, err := GetExtendedStatus(dir)
		require.NoError(t, err)
		assert.True(t, ext.IsDirty)
		assert.Equal(t, 1, ext.ModifiedCount)
		assert.Equal(t, 2, ext.LinesAdded)
		assert.Equal(t, 0, ext.LinesDeleted)
	})

	t.Run("staged changes report line counts", func(t *testing.T) {
		runGitCommand(t, dir, "add", "file.txt")

		ext, err := GetExtendedStatus(dir)
		require.NoError(t, err)
		assert.True(t, ext.IsDirty)
		assert.Equal(t, 1, ext.StagedCount)
		assert.Equal(t, 0, ext.ModifiedCount)
		assert.Equal(t, 2, ext.LinesAdded)
		assert.Equal(t, 0, ext.LinesDeleted)

		// Restore a clean tree for good measure.
		runGitCommand(t, dir, "reset", "--hard", "HEAD")
	})
}
