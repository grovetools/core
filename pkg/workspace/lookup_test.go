package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetProjectByPath(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "test-get-project-by-path-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a test project directory
	projectPath := filepath.Join(tmpDir, "test-project")
	err = os.Mkdir(projectPath, 0755)
	require.NoError(t, err)

	// Create a .git directory to make it look like a git repo
	gitDir := filepath.Join(projectPath, ".git")
	err = os.Mkdir(gitDir, 0755)
	require.NoError(t, err)

	t.Run("Basic project path", func(t *testing.T) {
		projInfo, err := GetProjectByPath(projectPath)
		require.NoError(t, err)
		assert.Equal(t, "test-project", projInfo.Name)
		assert.Equal(t, projectPath, projInfo.Path)
		assert.False(t, projInfo.IsWorktree)
		assert.False(t, projInfo.IsEcosystem)
		assert.Equal(t, "", projInfo.ParentEcosystemPath)
	})

	t.Run("Non-existent path", func(t *testing.T) {
		_, err := GetProjectByPath(filepath.Join(tmpDir, "non-existent"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "does not exist")
	})

	t.Run("Relative path resolution", func(t *testing.T) {
		// Save current directory
		cwd, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(cwd)

		// Change to tmp directory
		err = os.Chdir(tmpDir)
		require.NoError(t, err)

		// Test with relative path
		projInfo, err := GetProjectByPath("./test-project")
		require.NoError(t, err)
		assert.Equal(t, "test-project", projInfo.Name)
		// Path should be absolute and end with test-project
		assert.True(t, filepath.IsAbs(projInfo.Path), "Path should be absolute")
		assert.Equal(t, "test-project", filepath.Base(projInfo.Path))
	})
}
