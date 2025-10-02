package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	require.NoError(t, os.WriteFile(testFile, []byte("# Test\n"), 0644))

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