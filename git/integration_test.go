//go:build integration
// +build integration

package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	// TODO: Update when testutil is available in grove-core
	// "github.com/mattsolo1/grove-core/testutil"
)

// Temporary helper functions until testutil is available
func runGitCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	require.NoError(t, cmd.Run())
}

func createCommit(t *testing.T, dir, filename, content string) {
	t.Helper()
	filePath := filepath.Join(dir, filename)
	require.NoError(t, os.WriteFile(filePath, []byte(content), 0644))
	runGitCommand(t, dir, "add", filename)
	runGitCommand(t, dir, "commit", "-m", "Add "+filename)
}

func TestGitIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepoForTest(t, tmpDir)

	// Create and checkout branches
	runGitCommand(t, tmpDir, "checkout", "-b", "feature")
	createCommit(t, tmpDir, "feature.txt", "feature content")

	runGitCommand(t, tmpDir, "checkout", "main")
	createCommit(t, tmpDir, "main.txt", "main content")

	// Install hooks
	hookManager := NewHookManager("grove")
	require.NoError(t, hookManager.InstallHooks(context.Background(), tmpDir))

	// Simulate branch switch (hooks would normally run)
	runGitCommand(t, tmpDir, "checkout", "feature")

	// Verify we're on feature branch
	repo, branch, err := GetRepoInfo(tmpDir)
	require.NoError(t, err)
	assert.NotEmpty(t, repo)
	assert.Equal(t, "feature", branch)
}

func TestWorktreeIntegration(t *testing.T) {
	mainDir := t.TempDir()
	initGitRepoForTest(t, mainDir)

	// Create initial commit
	createCommit(t, mainDir, "README.md", "# Test Project")

	// Create worktree
	manager := NewWorktreeManager()
	worktreePath := filepath.Join(mainDir, "..", "feature-worktree")

	err := manager.CreateWorktree(context.Background(), mainDir, worktreePath, "feature", true)
	require.NoError(t, err)

	// Verify worktree created
	assert.DirExists(t, worktreePath)

	// Get worktree info by listing from main repo
	worktrees, err := manager.ListWorktrees(context.Background(), mainDir)
	require.NoError(t, err)
	// Find the feature worktree
	var featureWT *WorktreeInfo
	for _, wt := range worktrees {
		if strings.Contains(wt.Path, "feature-worktree") {
			featureWT = &wt
			break
		}
	}
	require.NotNil(t, featureWT, "feature worktree not found")
	assert.Equal(t, "feature", featureWT.Branch)

	// Clean up
	err = manager.RemoveWorktree(context.Background(), mainDir, worktreePath)
	assert.NoError(t, err)
}