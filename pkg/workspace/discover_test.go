package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/grovetools/core/config"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// setupMockFS creates a mock filesystem structure for testing.
func setupMockFS(t *testing.T) (string, string) {
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

	// 2. A User Ecosystem with two Projects
	ecoDir := filepath.Join(rootDir, "work", "my-ecosystem")
	require.NoError(t, os.MkdirAll(ecoDir, 0755))
	ecoCfg := config.Config{Name: "my-ecosystem", Workspaces: []string{"*"}}
	ecoBytes, _ := yaml.Marshal(ecoCfg)
	require.NoError(t, os.WriteFile(filepath.Join(ecoDir, "grove.yml"), ecoBytes, 0644))

	// Project A with a worktree
	projADir := filepath.Join(ecoDir, "project-a")
	require.NoError(t, os.MkdirAll(projADir, 0755))
	projACfg := config.Config{Name: "project-a"}
	projABytes, _ := yaml.Marshal(projACfg)
	require.NoError(t, os.WriteFile(filepath.Join(projADir, "grove.yml"), projABytes, 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(projADir, ".grove-worktrees", "feature-branch"), 0755))

	// Project B
	projBDir := filepath.Join(ecoDir, "project-b")
	require.NoError(t, os.MkdirAll(projBDir, 0755))
	projBCfg := config.Config{Name: "project-b"}
	projBBytes, _ := yaml.Marshal(projBCfg)
	require.NoError(t, os.WriteFile(filepath.Join(projBDir, "grove.yml"), projBBytes, 0644))

	// 3. An Orphan Project using .grove.yml
	orphanDir := filepath.Join(rootDir, "work", "orphan-project")
	require.NoError(t, os.MkdirAll(orphanDir, 0755))
	orphanCfg := config.Config{Name: "orphan-project"}
	orphanBytes, _ := yaml.Marshal(orphanCfg)
	require.NoError(t, os.WriteFile(filepath.Join(orphanDir, ".grove.yml"), orphanBytes, 0644))

	// 4. A Non-Grove Directory
	nonGroveDir := filepath.Join(rootDir, "work", "other-dir")
	require.NoError(t, os.MkdirAll(nonGroveDir, 0755))
	require.NoError(t, os.Mkdir(filepath.Join(nonGroveDir, ".git"), 0755))

	return rootDir, filepath.Join(rootDir, "home")
}

func TestDiscoveryService(t *testing.T) {
	rootDir, homeDir := setupMockFS(t)

	// Set XDG_CONFIG_HOME env var to our mock config directory
	originalXDG := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(homeDir, ".config"))
	defer os.Setenv("XDG_CONFIG_HOME", originalXDG)

	logger := logrus.New()
	logger.SetOutput(os.Stdout)
	logger.SetLevel(logrus.DebugLevel)

	service := NewDiscoveryService(logger)
	result, err := service.DiscoverAll()

	require.NoError(t, err)
	require.NotNil(t, result)

	t.Run("Ecosystem Discovery", func(t *testing.T) {
		assert.Len(t, result.Ecosystems, 1, "Should find one ecosystem")
		if len(result.Ecosystems) > 0 {
			assert.Equal(t, "my-ecosystem", result.Ecosystems[0].Name)
			assert.Equal(t, filepath.Join(rootDir, "work", "my-ecosystem"), result.Ecosystems[0].Path)
		}
	})

	t.Run("Project Discovery", func(t *testing.T) {
		assert.Len(t, result.Projects, 3, "Should find three projects")

		// Create a map for easier lookup
		projects := make(map[string]Project)
		for _, p := range result.Projects {
			projects[p.Name] = p
		}

		// Test Project A (in ecosystem, with worktree)
		projA, ok := projects["project-a"]
		assert.True(t, ok, "Project A should be found")
		assert.Equal(t, filepath.Join(rootDir, "work", "my-ecosystem"), projA.ParentEcosystemPath)
		assert.Len(t, projA.Workspaces, 2, "Project A should have two workspaces (primary + worktree)")

		// Test Project B (in ecosystem, no worktree)
		projB, ok := projects["project-b"]
		assert.True(t, ok, "Project B should be found")
		assert.Equal(t, filepath.Join(rootDir, "work", "my-ecosystem"), projB.ParentEcosystemPath)
		assert.Len(t, projB.Workspaces, 1, "Project B should have one workspace")

		// Test Orphan Project
		orphan, ok := projects["orphan-project"]
		assert.True(t, ok, "Orphan project should be found")
		assert.Empty(t, orphan.ParentEcosystemPath, "Orphan project should have no parent ecosystem")
		assert.Len(t, orphan.Workspaces, 1, "Orphan project should have one workspace")
	})

	t.Run("Non-Grove Directory Discovery", func(t *testing.T) {
		assert.Len(t, result.NonGroveDirectories, 1, "Should find one non-grove directory")
		if len(result.NonGroveDirectories) > 0 {
			assert.Equal(t, filepath.Join(rootDir, "work", "other-dir"), result.NonGroveDirectories[0])
		}
	})
}
