package workspace

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper to check if git is available
func skipIfNoGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}
}

// Helper to setup a git repository for testing
func setupTestRepo(t *testing.T, dir string) {
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	err := cmd.Run()
	require.NoError(t, err)
	
	// Configure git user
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = dir
	err = cmd.Run()
	require.NoError(t, err)
	
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = dir
	err = cmd.Run()
	require.NoError(t, err)
	
	// Create initial commit
	readmePath := filepath.Join(dir, "README.md")
	err = os.WriteFile(readmePath, []byte("test repo"), 0644)
	require.NoError(t, err)
	
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = dir
	err = cmd.Run()
	require.NoError(t, err)
	
	cmd = exec.Command("git", "commit", "-m", "initial commit")
	cmd.Dir = dir
	err = cmd.Run()
	require.NoError(t, err)
}

func TestPrepare(t *testing.T) {
	skipIfNoGit(t)
	
	t.Run("single repo workspace creation", func(t *testing.T) {
		// Setup test repository
		tempDir := t.TempDir()
		repoDir := filepath.Join(tempDir, "test-repo")
		require.NoError(t, os.MkdirAll(repoDir, 0755))
		setupTestRepo(t, repoDir)
		
		ctx := context.Background()
		opts := PrepareOptions{
			GitRoot:      repoDir,
			WorktreeName: "test-workspace",
			BranchName:   "feature/test",
		}
		
		// Create workspace
		worktreePath, err := Prepare(ctx, opts)
		require.NoError(t, err)
		assert.NotEmpty(t, worktreePath)
		
		// Verify worktree exists
		expectedPath := filepath.Join(repoDir, ".grove-worktrees", "test-workspace")
		assert.Equal(t, expectedPath, worktreePath)
		
		info, err := os.Stat(worktreePath)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
		
		// Verify branch was created
		cmd := exec.Command("git", "branch", "--list", "feature/test")
		cmd.Dir = repoDir
		output, err := cmd.Output()
		require.NoError(t, err)
		assert.Contains(t, string(output), "feature/test")
	})
	
	t.Run("ecosystem worktree with repos", func(t *testing.T) {
		// Setup ecosystem repository with submodules
		tempDir := t.TempDir()
		ecosystemDir := filepath.Join(tempDir, "ecosystem")
		require.NoError(t, os.MkdirAll(ecosystemDir, 0755))
		setupTestRepo(t, ecosystemDir)
		
		// Create .gitmodules file
		gitmodulesContent := `[submodule "repo1"]
	path = repo1
	url = https://github.com/example/repo1.git
[submodule "repo2"]
	path = repo2
	url = https://github.com/example/repo2.git
`
		gitmodulesPath := filepath.Join(ecosystemDir, ".gitmodules")
		err := os.WriteFile(gitmodulesPath, []byte(gitmodulesContent), 0644)
		require.NoError(t, err)
		
		// Commit .gitmodules
		cmd := exec.Command("git", "add", ".gitmodules")
		cmd.Dir = ecosystemDir
		err = cmd.Run()
		require.NoError(t, err)
		
		cmd = exec.Command("git", "commit", "-m", "add gitmodules")
		cmd.Dir = ecosystemDir
		err = cmd.Run()
		require.NoError(t, err)
		
		ctx := context.Background()
		opts := PrepareOptions{
			GitRoot:      ecosystemDir,
			WorktreeName: "eco-workspace",
			BranchName:   "feature/ecosystem",
			Repos:        []string{"repo1"}, // Only setup repo1
		}
		
		// Mock DiscoverLocalWorkspaces to avoid actual discovery
		oldFunc := discoverLocalWorkspacesFunc
		discoverLocalWorkspacesFunc = func(ctx context.Context) (map[string]string, error) {
			return map[string]string{}, nil
		}
		defer func() { discoverLocalWorkspacesFunc = oldFunc }()
		
		// Create workspace
		worktreePath, err := Prepare(ctx, opts)
		require.NoError(t, err)
		assert.NotEmpty(t, worktreePath)
		
		// Verify worktree exists
		expectedPath := filepath.Join(ecosystemDir, ".grove-worktrees", "eco-workspace")
		assert.Equal(t, expectedPath, worktreePath)
		
		// Verify go.work was created
		goWorkPath := filepath.Join(worktreePath, "go.work")
		_, err = os.Stat(goWorkPath)
		// go.work might not exist if there are no Go modules
		if err == nil {
			content, err := os.ReadFile(goWorkPath)
			require.NoError(t, err)
			assert.Contains(t, string(content), "go 1.")
		}
	})
	
	t.Run("error cases", func(t *testing.T) {
		ctx := context.Background()
		
		// Test with invalid git root
		opts := PrepareOptions{
			GitRoot:      "/nonexistent/path",
			WorktreeName: "test",
			BranchName:   "test",
		}
		
		_, err := Prepare(ctx, opts)
		assert.Error(t, err)
		
		// Test with empty worktree name
		tempDir := t.TempDir()
		opts = PrepareOptions{
			GitRoot:      tempDir,
			WorktreeName: "",
			BranchName:   "test",
		}
		
		_, err = Prepare(ctx, opts)
		assert.Error(t, err)
	})
	
	t.Run("prepare with existing branch", func(t *testing.T) {
		// Setup test repository
		tempDir := t.TempDir()
		repoDir := filepath.Join(tempDir, "test-repo")
		require.NoError(t, os.MkdirAll(repoDir, 0755))
		setupTestRepo(t, repoDir)

		// Create an existing branch
		cmd := exec.Command("git", "checkout", "-b", "existing-branch")
		cmd.Dir = repoDir
		err := cmd.Run()
		require.NoError(t, err)

		// Switch back to main
		cmd = exec.Command("git", "checkout", "main")
		cmd.Dir = repoDir
		err = cmd.Run()
		require.NoError(t, err)

		ctx := context.Background()
		opts := PrepareOptions{
			GitRoot:      repoDir,
			WorktreeName: "test-workspace",
			BranchName:   "existing-branch",
		}

		// Should use existing branch without error
		worktreePath, err := Prepare(ctx, opts)
		require.NoError(t, err)
		assert.NotEmpty(t, worktreePath)
	})

	t.Run("prepare with branch already checked out in worktree", func(t *testing.T) {
		// Setup test repository
		tempDir := t.TempDir()
		repoDir := filepath.Join(tempDir, "test-repo")
		require.NoError(t, os.MkdirAll(repoDir, 0755))
		setupTestRepo(t, repoDir)

		ctx := context.Background()

		// First, create a worktree with a branch
		opts1 := PrepareOptions{
			GitRoot:      repoDir,
			WorktreeName: "first-workspace",
			BranchName:   "shared-branch",
		}

		firstWorktreePath, err := Prepare(ctx, opts1)
		require.NoError(t, err)
		assert.NotEmpty(t, firstWorktreePath)

		// Now try to prepare another worktree for the same branch
		// This should return the existing worktree path instead of failing
		opts2 := PrepareOptions{
			GitRoot:      repoDir,
			WorktreeName: "second-workspace",
			BranchName:   "shared-branch",
		}

		secondWorktreePath, err := Prepare(ctx, opts2)
		require.NoError(t, err)
		assert.Equal(t, firstWorktreePath, secondWorktreePath, "Should return existing worktree path when branch is already checked out")
	})
}