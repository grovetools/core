package git

import (
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
	require.NoError(t, os.WriteFile(testFile, []byte("test"), 0644))

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())

	cmd = exec.Command("git", "github.com/mattsolo1/grove-core/config", "user.email", "test@example.com")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())

	cmd = exec.Command("git", "github.com/mattsolo1/grove-core/config", "user.name", "Test User")
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