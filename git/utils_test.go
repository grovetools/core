package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractRepoName(t *testing.T) {
	testCases := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "SSH URL with .git",
			url:      "git@github.com:user/my-project.git",
			expected: "my-project",
		},
		{
			name:     "HTTPS URL with .git",
			url:      "https://github.com/user/my-project.git",
			expected: "my-project",
		},
		{
			name:     "HTTPS URL without .git",
			url:      "https://github.com/user/my-project",
			expected: "my-project",
		},
		{
			name:     "GitLab nested groups",
			url:      "https://gitlab.com/group/subgroup/project.git",
			expected: "project",
		},
		{
			name:     "SSH URL without .git",
			url:      "git@github.com:user/repo",
			expected: "repo",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := extractRepoName(tc.url)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestGetRepoInfo(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())

	// Create initial commit
	testFile := filepath.Join(tmpDir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("test"), 0o644))

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())

	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())

	// Test without remote (should use directory name)
	repo, branch, err := GetRepoInfo(tmpDir)
	assert.NoError(t, err)
	assert.Equal(t, filepath.Base(tmpDir), repo)
	assert.NotEmpty(t, branch)

	// Add remote
	cmd = exec.Command("git", "remote", "add", "origin", "git@github.com:user/test-repo.git")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())

	// Test with remote
	repo, branch, err = GetRepoInfo(tmpDir)
	assert.NoError(t, err)
	assert.Equal(t, "test-repo", repo)
	assert.NotEmpty(t, branch)
}

func TestIsGitRepo(t *testing.T) {
	// Test with git repo
	tmpDir := t.TempDir()
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())

	assert.True(t, IsGitRepo(tmpDir))

	// Test with non-git directory
	nonGitDir := t.TempDir()
	assert.False(t, IsGitRepo(nonGitDir))
}

// gitInitWithCommit initializes a git repo in dir with one commit so that linked
// worktrees can be created from it.
func gitInitWithCommit(t *testing.T, dir string) {
	t.Helper()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		require.NoError(t, cmd.Run(), "git %v", args)
	}
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test User")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.txt"), []byte("test"), 0o644))
	run("add", ".")
	run("commit", "-m", "Initial commit")
}

func TestResolveGitDirs(t *testing.T) {
	ctx := context.Background()

	t.Run("primary checkout", func(t *testing.T) {
		// EvalSymlinks normalizes macOS /var -> /private/var so we can compare
		// against git's absolute output.
		tmpDir, err := filepath.EvalSymlinks(t.TempDir())
		require.NoError(t, err)
		gitInitWithCommit(t, tmpDir)

		gitDir, commonDir, err := ResolveGitDirs(ctx, tmpDir)
		require.NoError(t, err)

		want := filepath.Join(tmpDir, ".git")
		assert.Equal(t, want, gitDir)
		assert.Equal(t, want, commonDir)
	})

	t.Run("linked worktree", func(t *testing.T) {
		ownerDir, err := filepath.EvalSymlinks(t.TempDir())
		require.NoError(t, err)
		gitInitWithCommit(t, ownerDir)

		// Create a linked worktree in a sibling location.
		wtParent, err := filepath.EvalSymlinks(t.TempDir())
		require.NoError(t, err)
		wtPath := filepath.Join(wtParent, "linked")

		cmd := exec.Command("git", "worktree", "add", "-b", "feature", wtPath)
		cmd.Dir = ownerDir
		require.NoError(t, cmd.Run())

		gitDir, commonDir, err := ResolveGitDirs(ctx, wtPath)
		require.NoError(t, err)

		// gitDir points under the owner's .git/worktrees/<name>; commonDir is the
		// shared owner .git.
		ownerGit := filepath.Join(ownerDir, ".git")
		assert.Equal(t, filepath.Join(ownerGit, "worktrees", "linked"), gitDir)
		assert.Equal(t, ownerGit, commonDir)
	})

	t.Run("non-repo dir", func(t *testing.T) {
		nonGitDir := t.TempDir()
		_, _, err := ResolveGitDirs(ctx, nonGitDir)
		assert.Error(t, err)
	})
}
