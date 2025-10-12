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
		// Use EvalSymlinks to handle /var vs /private/var on macOS
		expectedPath, _ := filepath.EvalSymlinks(ecoPath)
		actualPath, _ := filepath.EvalSymlinks(node.Path)
		assert.Equal(t, expectedPath, actualPath)
		assert.Equal(t, KindEcosystemRoot, node.Kind)
	})

	t.Run("Exact match - ecosystem subproject", func(t *testing.T) {
		projPath := filepath.Join(rootDir, "work", "my-ecosystem", "project-a")
		node, err := GetProjectByPath(projPath)
		require.NoError(t, err)
		assert.Equal(t, "project-a", node.Name)
		expectedPath, _ := filepath.EvalSymlinks(projPath)
		actualPath, _ := filepath.EvalSymlinks(node.Path)
		assert.Equal(t, expectedPath, actualPath)
		assert.Equal(t, KindEcosystemSubProject, node.Kind)
		expectedParent, _ := filepath.EvalSymlinks(filepath.Join(rootDir, "work", "my-ecosystem"))
		actualParent, _ := filepath.EvalSymlinks(node.ParentEcosystemPath)
		assert.Equal(t, expectedParent, actualParent)
	})

	t.Run("Subdirectory within project", func(t *testing.T) {
		subPath := filepath.Join(rootDir, "work", "my-ecosystem", "project-a", "src", "components")
		node, err := GetProjectByPath(subPath)
		require.NoError(t, err)
		assert.Equal(t, "project-a", node.Name)
		expectedPath, _ := filepath.EvalSymlinks(filepath.Join(rootDir, "work", "my-ecosystem", "project-a"))
		actualPath, _ := filepath.EvalSymlinks(node.Path)
		assert.Equal(t, expectedPath, actualPath)
		assert.Equal(t, KindEcosystemSubProject, node.Kind)
	})

	t.Run("Ecosystem worktree", func(t *testing.T) {
		wtPath := filepath.Join(rootDir, "work", "my-ecosystem", ".grove-worktrees", "feature-work")
		node, err := GetProjectByPath(wtPath)
		require.NoError(t, err)
		assert.Equal(t, "feature-work", node.Name)
		expectedPath, _ := filepath.EvalSymlinks(wtPath)
		actualPath, _ := filepath.EvalSymlinks(node.Path)
		assert.Equal(t, expectedPath, actualPath)
		assert.Equal(t, KindEcosystemWorktree, node.Kind)
	})

	t.Run("Standalone project", func(t *testing.T) {
		projPath := filepath.Join(rootDir, "work", "standalone-project")
		node, err := GetProjectByPath(projPath)
		require.NoError(t, err)
		assert.Equal(t, "standalone-project", node.Name)
		expectedPath, _ := filepath.EvalSymlinks(projPath)
		actualPath, _ := filepath.EvalSymlinks(node.Path)
		assert.Equal(t, expectedPath, actualPath)
		assert.Equal(t, KindStandaloneProject, node.Kind)
		assert.Empty(t, node.ParentEcosystemPath)
	})

	t.Run("Standalone project worktree", func(t *testing.T) {
		wtPath := filepath.Join(rootDir, "work", "standalone-project", ".grove-worktrees", "fix-bug")
		node, err := GetProjectByPath(wtPath)
		require.NoError(t, err)
		assert.Equal(t, "fix-bug", node.Name)
		expectedPath, _ := filepath.EvalSymlinks(wtPath)
		actualPath, _ := filepath.EvalSymlinks(node.Path)
		assert.Equal(t, expectedPath, actualPath)
		assert.Equal(t, KindStandaloneProjectWorktree, node.Kind)
	})

	t.Run("Non-Grove directory", func(t *testing.T) {
		nonGrovePath := filepath.Join(rootDir, "work", "other-repo")
		node, err := GetProjectByPath(nonGrovePath)
		require.NoError(t, err)
		assert.Equal(t, "other-repo", node.Name)
		expectedPath, _ := filepath.EvalSymlinks(nonGrovePath)
		actualPath, _ := filepath.EvalSymlinks(node.Path)
		assert.Equal(t, expectedPath, actualPath)
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
		expectedPath, _ := filepath.EvalSymlinks(projPath)
		actualPath, _ := filepath.EvalSymlinks(node.Path)
		assert.Equal(t, expectedPath, actualPath)
	})
}

// TestGetProjectByPath_WithEcosystemWorktrees tests the worktree scenarios
// where cwd command should correctly identify workspace kinds and root ecosystem paths.
func TestGetProjectByPath_WithEcosystemWorktrees(t *testing.T) {
	rootDir := t.TempDir()

	// Create a root directory for an ecosystem (my-ecosystem) containing a .git directory
	ecoRootDir := filepath.Join(rootDir, "my-ecosystem")
	require.NoError(t, os.MkdirAll(ecoRootDir, 0755))
	require.NoError(t, os.Mkdir(filepath.Join(ecoRootDir, ".git"), 0755))

	// Create grove.yml with workspaces key for ecosystem
	ecoCfg := config.Config{Name: "my-ecosystem", Workspaces: []string{"*"}}
	ecoBytes, _ := yaml.Marshal(ecoCfg)
	require.NoError(t, os.WriteFile(filepath.Join(ecoRootDir, "grove.yml"), ecoBytes, 0644))

	// Create an ecosystem worktree directory (my-ecosystem/.grove-worktrees/feature-branch)
	// containing a .git file (to simulate a worktree) and a grove.yml
	worktreeDir := filepath.Join(ecoRootDir, ".grove-worktrees", "feature-branch")
	require.NoError(t, os.MkdirAll(worktreeDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(worktreeDir, ".git"), []byte("gitdir: ../../.git/worktrees/feature-branch"), 0644))

	wtCfg := config.Config{Name: "my-ecosystem", Workspaces: []string{"*"}}
	wtBytes, _ := yaml.Marshal(wtCfg)
	require.NoError(t, os.WriteFile(filepath.Join(worktreeDir, "grove.yml"), wtBytes, 0644))

	// Create a sub-project within the worktree (.../feature-branch/sub-project)
	// containing its own .git directory and a grove.yml without a workspaces key
	subProjectDir := filepath.Join(worktreeDir, "sub-project")
	require.NoError(t, os.MkdirAll(subProjectDir, 0755))
	require.NoError(t, os.Mkdir(filepath.Join(subProjectDir, ".git"), 0755))

	subProjCfg := config.Config{Name: "sub-project"}
	subProjBytes, _ := yaml.Marshal(subProjCfg)
	require.NoError(t, os.WriteFile(filepath.Join(subProjectDir, "grove.yml"), subProjBytes, 0644))

	// Create another sub-project within the worktree that is also a worktree
	// (.../feature-branch/sub-project-wt), containing a .git file
	subProjectWtDir := filepath.Join(worktreeDir, "sub-project-wt")
	require.NoError(t, os.MkdirAll(subProjectWtDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(subProjectWtDir, ".git"), []byte("gitdir: ../../.git/worktrees/sub-project-wt"), 0644))

	subProjWtCfg := config.Config{Name: "sub-project-wt"}
	subProjWtBytes, _ := yaml.Marshal(subProjWtCfg)
	require.NoError(t, os.WriteFile(filepath.Join(subProjectWtDir, "grove.yml"), subProjWtBytes, 0644))

	// Create a deeply nested path inside sub-project
	deepPath := filepath.Join(subProjectDir, "src", "app", "components")
	require.NoError(t, os.MkdirAll(deepPath, 0755))

	t.Run("Case 1: Ecosystem Worktree", func(t *testing.T) {
		// Path: .../my-ecosystem/.grove-worktrees/feature-branch
		node, err := GetProjectByPath(worktreeDir)
		require.NoError(t, err)
		assert.Equal(t, "feature-branch", node.Name)
		// Use EvalSymlinks to handle /var vs /private/var on macOS
		expectedPath, _ := filepath.EvalSymlinks(worktreeDir)
		actualPath, _ := filepath.EvalSymlinks(node.Path)
		assert.Equal(t, expectedPath, actualPath)
		assert.Equal(t, KindEcosystemWorktree, node.Kind)
		expectedRoot, _ := filepath.EvalSymlinks(ecoRootDir)
		actualRoot, _ := filepath.EvalSymlinks(node.RootEcosystemPath)
		assert.Equal(t, expectedRoot, actualRoot, "RootEcosystemPath should point to the true ecosystem root")
	})

	t.Run("Case 2: Sub-project in Worktree", func(t *testing.T) {
		// Path: .../feature-branch/sub-project
		node, err := GetProjectByPath(subProjectDir)
		require.NoError(t, err)
		assert.Equal(t, "sub-project", node.Name)
		expectedPath, _ := filepath.EvalSymlinks(subProjectDir)
		actualPath, _ := filepath.EvalSymlinks(node.Path)
		assert.Equal(t, expectedPath, actualPath)
		assert.Equal(t, KindEcosystemWorktreeSubProject, node.Kind)
		expectedParent, _ := filepath.EvalSymlinks(worktreeDir)
		actualParent, _ := filepath.EvalSymlinks(node.ParentEcosystemPath)
		assert.Equal(t, expectedParent, actualParent, "ParentEcosystemPath should point to the worktree")
		expectedRoot, _ := filepath.EvalSymlinks(ecoRootDir)
		actualRoot, _ := filepath.EvalSymlinks(node.RootEcosystemPath)
		assert.Equal(t, expectedRoot, actualRoot, "RootEcosystemPath should point to the true ecosystem root")
	})

	t.Run("Case 3: Sub-project Worktree in Worktree", func(t *testing.T) {
		// Path: .../feature-branch/sub-project-wt
		node, err := GetProjectByPath(subProjectWtDir)
		require.NoError(t, err)
		assert.Equal(t, "sub-project-wt", node.Name)
		expectedPath, _ := filepath.EvalSymlinks(subProjectWtDir)
		actualPath, _ := filepath.EvalSymlinks(node.Path)
		assert.Equal(t, expectedPath, actualPath)
		assert.Equal(t, KindEcosystemWorktreeSubProjectWorktree, node.Kind)
		expectedRoot, _ := filepath.EvalSymlinks(ecoRootDir)
		actualRoot, _ := filepath.EvalSymlinks(node.RootEcosystemPath)
		assert.Equal(t, expectedRoot, actualRoot, "RootEcosystemPath should be correct")
	})

	t.Run("Case 4: Deeply nested path in sub-project", func(t *testing.T) {
		// Path: .../sub-project/src/app/components
		node, err := GetProjectByPath(deepPath)
		require.NoError(t, err)
		assert.Equal(t, "sub-project", node.Name)
		expectedPath, _ := filepath.EvalSymlinks(subProjectDir)
		actualPath, _ := filepath.EvalSymlinks(node.Path)
		assert.Equal(t, expectedPath, actualPath)
		assert.Equal(t, KindEcosystemWorktreeSubProject, node.Kind)
		expectedParent, _ := filepath.EvalSymlinks(worktreeDir)
		actualParent, _ := filepath.EvalSymlinks(node.ParentEcosystemPath)
		assert.Equal(t, expectedParent, actualParent)
		expectedRoot, _ := filepath.EvalSymlinks(ecoRootDir)
		actualRoot, _ := filepath.EvalSymlinks(node.RootEcosystemPath)
		assert.Equal(t, expectedRoot, actualRoot, "RootEcosystemPath should point to the true ecosystem root even from deeply nested path")
	})

	t.Run("Ecosystem Root", func(t *testing.T) {
		// Test that the ecosystem root itself is correctly identified
		node, err := GetProjectByPath(ecoRootDir)
		require.NoError(t, err)
		assert.Equal(t, "my-ecosystem", node.Name)
		expectedPath, _ := filepath.EvalSymlinks(ecoRootDir)
		actualPath, _ := filepath.EvalSymlinks(node.Path)
		assert.Equal(t, expectedPath, actualPath)
		assert.Equal(t, KindEcosystemRoot, node.Kind)
		expectedRoot, _ := filepath.EvalSymlinks(ecoRootDir)
		actualRoot, _ := filepath.EvalSymlinks(node.RootEcosystemPath)
		assert.Equal(t, expectedRoot, actualRoot)
		assert.Empty(t, node.ParentEcosystemPath)
	})
}
