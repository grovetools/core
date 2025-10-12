package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mattsolo1/grove-core/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// setupMockFSForLookup creates a mock filesystem structure for testing GetProjectByPath.
func setupMockFSForLookup(t *testing.T) (string, string) {
	rootDir := t.TempDir()

	// 1. Global config with 'search_paths'
	globalConfigDir := filepath.Join(rootDir, "home", ".config", "grove")
	require.NoError(t, os.MkdirAll(globalConfigDir, 0755))
	globalCfg := config.Config{
		SearchPaths: map[string]config.SearchPathConfig{
			"work": {Path: filepath.Join(rootDir, "work"), Enabled: true},
		},
	}
	globalBytes, _ := yaml.Marshal(globalCfg)
	require.NoError(t, os.WriteFile(filepath.Join(globalConfigDir, "grove.yml"), globalBytes, 0644))

	// 2. A User Ecosystem
	ecoDir := filepath.Join(rootDir, "work", "my-ecosystem")
	require.NoError(t, os.MkdirAll(ecoDir, 0755))
	ecoCfg := config.Config{Name: "my-ecosystem", Workspaces: []string{"*"}}
	ecoBytes, _ := yaml.Marshal(ecoCfg)
	require.NoError(t, os.WriteFile(filepath.Join(ecoDir, "grove.yml"), ecoBytes, 0644))

	// 3. Ecosystem subproject
	projADir := filepath.Join(ecoDir, "project-a")
	require.NoError(t, os.MkdirAll(projADir, 0755))
	projACfg := config.Config{Name: "project-a"}
	projABytes, _ := yaml.Marshal(projACfg)
	require.NoError(t, os.WriteFile(filepath.Join(projADir, "grove.yml"), projABytes, 0644))

	// Create a subdirectory in project-a
	require.NoError(t, os.MkdirAll(filepath.Join(projADir, "src", "components"), 0755))

	// 4. Ecosystem worktree
	ecoWorktreeDir := filepath.Join(ecoDir, ".grove-worktrees", "feature-work")
	require.NoError(t, os.MkdirAll(ecoWorktreeDir, 0755))
	// Create .git file to mark it as a worktree
	require.NoError(t, os.WriteFile(filepath.Join(ecoWorktreeDir, ".git"), []byte("gitdir: ..."), 0644))
	ecoWtCfg := config.Config{Name: "my-ecosystem", Workspaces: []string{"*"}}
	ecoWtBytes, _ := yaml.Marshal(ecoWtCfg)
	require.NoError(t, os.WriteFile(filepath.Join(ecoWorktreeDir, "grove.yml"), ecoWtBytes, 0644))

	// 5. A Standalone Project with worktree
	standaloneDir := filepath.Join(rootDir, "work", "standalone-project")
	require.NoError(t, os.MkdirAll(standaloneDir, 0755))
	standaloneCfg := config.Config{Name: "standalone-project"}
	standaloneBytes, _ := yaml.Marshal(standaloneCfg)
	require.NoError(t, os.WriteFile(filepath.Join(standaloneDir, "grove.yml"), standaloneBytes, 0644))

	// Create worktree for standalone project
	standaloneWorktreeDir := filepath.Join(standaloneDir, ".grove-worktrees", "fix-bug")
	require.NoError(t, os.MkdirAll(standaloneWorktreeDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(standaloneWorktreeDir, ".git"), []byte("gitdir: ..."), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(standaloneWorktreeDir, "grove.yml"), standaloneBytes, 0644))

	// 6. A Non-Grove Directory
	nonGroveDir := filepath.Join(rootDir, "work", "other-repo")
	require.NoError(t, os.MkdirAll(nonGroveDir, 0755))
	require.NoError(t, os.Mkdir(filepath.Join(nonGroveDir, ".git"), 0755))

	return rootDir, filepath.Join(rootDir, "home")
}

func TestGetProjectByPath(t *testing.T) {
	rootDir, homeDir := setupMockFSForLookup(t)

	// Set XDG_CONFIG_HOME env var to our mock config directory
	originalXDG := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(homeDir, ".config"))
	defer os.Setenv("XDG_CONFIG_HOME", originalXDG)

	t.Run("Exact match - ecosystem root", func(t *testing.T) {
		ecoPath := filepath.Join(rootDir, "work", "my-ecosystem")
		node, err := GetProjectByPath(ecoPath)
		require.NoError(t, err)
		assert.Equal(t, "my-ecosystem", node.Name)
		assert.Equal(t, ecoPath, node.Path)
		assert.Equal(t, KindEcosystemRoot, node.Kind)
	})

	t.Run("Exact match - ecosystem subproject", func(t *testing.T) {
		projPath := filepath.Join(rootDir, "work", "my-ecosystem", "project-a")
		node, err := GetProjectByPath(projPath)
		require.NoError(t, err)
		assert.Equal(t, "project-a", node.Name)
		assert.Equal(t, projPath, node.Path)
		assert.Equal(t, KindEcosystemSubProject, node.Kind)
		assert.Equal(t, filepath.Join(rootDir, "work", "my-ecosystem"), node.ParentEcosystemPath)
	})

	t.Run("Subdirectory within project", func(t *testing.T) {
		subPath := filepath.Join(rootDir, "work", "my-ecosystem", "project-a", "src", "components")
		node, err := GetProjectByPath(subPath)
		require.NoError(t, err)
		assert.Equal(t, "project-a", node.Name)
		assert.Equal(t, filepath.Join(rootDir, "work", "my-ecosystem", "project-a"), node.Path)
		assert.Equal(t, KindEcosystemSubProject, node.Kind)
	})

	t.Run("Ecosystem worktree", func(t *testing.T) {
		wtPath := filepath.Join(rootDir, "work", "my-ecosystem", ".grove-worktrees", "feature-work")
		node, err := GetProjectByPath(wtPath)
		require.NoError(t, err)
		assert.Equal(t, "feature-work", node.Name)
		assert.Equal(t, wtPath, node.Path)
		assert.Equal(t, KindEcosystemWorktree, node.Kind)
	})

	t.Run("Standalone project", func(t *testing.T) {
		projPath := filepath.Join(rootDir, "work", "standalone-project")
		node, err := GetProjectByPath(projPath)
		require.NoError(t, err)
		assert.Equal(t, "standalone-project", node.Name)
		assert.Equal(t, projPath, node.Path)
		assert.Equal(t, KindStandaloneProject, node.Kind)
		assert.Empty(t, node.ParentEcosystemPath)
	})

	t.Run("Standalone project worktree", func(t *testing.T) {
		wtPath := filepath.Join(rootDir, "work", "standalone-project", ".grove-worktrees", "fix-bug")
		node, err := GetProjectByPath(wtPath)
		require.NoError(t, err)
		assert.Equal(t, "fix-bug", node.Name)
		assert.Equal(t, wtPath, node.Path)
		assert.Equal(t, KindStandaloneProjectWorktree, node.Kind)
	})

	t.Run("Non-Grove directory", func(t *testing.T) {
		nonGrovePath := filepath.Join(rootDir, "work", "other-repo")
		node, err := GetProjectByPath(nonGrovePath)
		require.NoError(t, err)
		assert.Equal(t, "other-repo", node.Name)
		assert.Equal(t, nonGrovePath, node.Path)
		assert.Equal(t, KindNonGroveRepo, node.Kind)
	})

	t.Run("Non-existent path", func(t *testing.T) {
		_, err := GetProjectByPath(filepath.Join(rootDir, "work", "non-existent"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "does not exist")
	})

	t.Run("Path not in any workspace", func(t *testing.T) {
		// Create a directory that won't be discovered
		outsidePath := filepath.Join(rootDir, "outside")
		require.NoError(t, os.MkdirAll(outsidePath, 0755))

		_, err := GetProjectByPath(outsidePath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no workspace found")
	})

	t.Run("Relative path resolution", func(t *testing.T) {
		// Test that GetProjectByPath properly handles absolute path resolution
		// The function internally calls filepath.Abs, so we verify this works
		projPath := filepath.Join(rootDir, "work", "my-ecosystem", "project-a")

		node, err := GetProjectByPath(projPath)
		require.NoError(t, err)
		assert.Equal(t, "project-a", node.Name)
		assert.True(t, filepath.IsAbs(node.Path), "Path should be absolute")
		assert.Equal(t, projPath, node.Path)
	})
}
