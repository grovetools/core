package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runGitCommand is a test helper to execute git commands.
func runGitCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s failed with output: %s", strings.Join(args, " "), string(output))
}

// setupGitRepo creates a test git repository.
func setupGitRepo(t *testing.T, dir string) {
	t.Helper()
	runGitCommand(t, dir, "init")
	runGitCommand(t, dir, "config", "user.email", "test@example.com")
	runGitCommand(t, dir, "config", "user.name", "Test User")
}

func TestGetStatus(t *testing.T) {
	t.Run("invalid path", func(t *testing.T) {
		_, err := GetStatus("/non/existent/path")
		assert.Error(t, err)
	})

	t.Run("non-git directory", func(t *testing.T) {
		tempDir := t.TempDir()
		_, err := GetStatus(tempDir)
		assert.Error(t, err)
	})

	t.Run("clean repo", func(t *testing.T) {
		tempDir := t.TempDir()
		setupGitRepo(t, tempDir)
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "file.txt"), []byte("content"), 0644))
		runGitCommand(t, tempDir, "add", "file.txt")
		runGitCommand(t, tempDir, "commit", "-m", "initial commit")

		status, err := GetStatus(tempDir)
		require.NoError(t, err)
		assert.False(t, status.IsDirty)
		assert.Equal(t, 0, status.ModifiedCount)
		assert.Equal(t, 0, status.StagedCount)
		assert.Equal(t, 0, status.UntrackedCount)
		assert.Equal(t, 0, status.AheadCount)
		assert.Equal(t, 0, status.BehindCount)
		assert.False(t, status.HasUpstream)
		assert.NotEmpty(t, status.Branch)
	})

	t.Run("with changes", func(t *testing.T) {
		tempDir := t.TempDir()
		setupGitRepo(t, tempDir)

		// Create initial commit first
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "initial.txt"), []byte("initial"), 0644))
		runGitCommand(t, tempDir, "add", "initial.txt")
		runGitCommand(t, tempDir, "commit", "-m", "initial commit")

		// Staged file (new file that's staged but not committed)
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "staged.txt"), []byte("staged"), 0644))
		runGitCommand(t, tempDir, "add", "staged.txt")

		// Modified file (modify the initial file)
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "initial.txt"), []byte("modified"), 0644))

		// Untracked file
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "untracked.txt"), []byte("untracked"), 0644))

		status, err := GetStatus(tempDir)
		require.NoError(t, err)
		assert.True(t, status.IsDirty)
		assert.Equal(t, 1, status.StagedCount, "staged.txt should be staged")
		assert.Equal(t, 1, status.ModifiedCount, "initial.txt should be modified")
		assert.Equal(t, 1, status.UntrackedCount, "untracked.txt should be untracked")
	})

	t.Run("with upstream", func(t *testing.T) {
		// Setup remote and local repos
		baseDir := t.TempDir()
		remoteDir := filepath.Join(baseDir, "remote.git")
		localDir := filepath.Join(baseDir, "local")

		// Init bare remote with main branch
		require.NoError(t, os.Mkdir(remoteDir, 0755))
		runGitCommand(t, remoteDir, "init", "--bare", "--initial-branch=main")

		// Clone local
		runGitCommand(t, baseDir, "clone", "remote.git", "local")
		setupGitRepo(t, localDir) // to set user config

		// Initial commit and push
		require.NoError(t, os.WriteFile(filepath.Join(localDir, "file.txt"), []byte("1"), 0644))
		runGitCommand(t, localDir, "add", ".")
		runGitCommand(t, localDir, "commit", "-m", "c1")
		runGitCommand(t, localDir, "push", "origin", "main")

		// Test ahead
		require.NoError(t, os.WriteFile(filepath.Join(localDir, "file2.txt"), []byte("2"), 0644))
		runGitCommand(t, localDir, "add", ".")
		runGitCommand(t, localDir, "commit", "-m", "c2")

		status, err := GetStatus(localDir)
		require.NoError(t, err)
		assert.True(t, status.HasUpstream)
		assert.Equal(t, 1, status.AheadCount)
		assert.Equal(t, 0, status.BehindCount)

		// Test behind - push local changes, then make new changes in another clone
		runGitCommand(t, localDir, "push", "origin", "main")

		// In another clone, push a different commit to remote to make local behind
		anotherLocalDir := filepath.Join(baseDir, "another")
		runGitCommand(t, baseDir, "clone", "remote.git", "another")
		setupGitRepo(t, anotherLocalDir)
		require.NoError(t, os.WriteFile(filepath.Join(anotherLocalDir, "another-file.txt"), []byte("remote change"), 0644))
		runGitCommand(t, anotherLocalDir, "add", ".")
		runGitCommand(t, anotherLocalDir, "commit", "-m", "remote-c3")
		runGitCommand(t, anotherLocalDir, "push", "origin", "main")

		// Now fetch in original local repo
		runGitCommand(t, localDir, "fetch")

		status, err = GetStatus(localDir)
		require.NoError(t, err)
		assert.True(t, status.HasUpstream)
		assert.Equal(t, 0, status.AheadCount, "Should be 0 ahead after pushing")
		assert.Equal(t, 1, status.BehindCount, "Should be 1 behind remote")
	})
}