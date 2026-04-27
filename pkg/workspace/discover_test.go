package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/grovetools/core/config"
)

// setupMockFS creates a mock filesystem structure for testing.
func setupMockFS(t *testing.T) (string, string) {
	rootDir := t.TempDir()

	// 1. Global config with 'search_paths'
	globalConfigDir := filepath.Join(rootDir, "home", ".config", "grove")
	require.NoError(t, os.MkdirAll(globalConfigDir, 0o755))
	emptyStr := ""
	globalCfg := config.Config{
		SearchPaths: map[string]config.SearchPathConfig{
			"work": {Path: filepath.Join(rootDir, "work"), Enabled: true},
		},
		// Disable cx repo discovery so tests don't pick up real user repos
		Context: &config.ContextConfig{
			ReposDir: &emptyStr,
		},
	}
	globalBytes, _ := yaml.Marshal(globalCfg)
	require.NoError(t, os.WriteFile(filepath.Join(globalConfigDir, "grove.yml"), globalBytes, 0o644))

	// 2. A User Ecosystem with two Projects
	ecoDir := filepath.Join(rootDir, "work", "my-ecosystem")
	require.NoError(t, os.MkdirAll(ecoDir, 0o755))
	ecoCfg := config.Config{Name: "my-ecosystem", Workspaces: []string{"*"}}
	ecoBytes, _ := yaml.Marshal(ecoCfg)
	require.NoError(t, os.WriteFile(filepath.Join(ecoDir, "grove.yml"), ecoBytes, 0o644))

	// Project A with a worktree
	projADir := filepath.Join(ecoDir, "project-a")
	require.NoError(t, os.MkdirAll(projADir, 0o755))
	projACfg := config.Config{Name: "project-a"}
	projABytes, _ := yaml.Marshal(projACfg)
	require.NoError(t, os.WriteFile(filepath.Join(projADir, "grove.yml"), projABytes, 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(projADir, ".grove-worktrees", "feature-branch"), 0o755))

	// Project B
	projBDir := filepath.Join(ecoDir, "project-b")
	require.NoError(t, os.MkdirAll(projBDir, 0o755))
	projBCfg := config.Config{Name: "project-b"}
	projBBytes, _ := yaml.Marshal(projBCfg)
	require.NoError(t, os.WriteFile(filepath.Join(projBDir, "grove.yml"), projBBytes, 0o644))

	// 3. An Orphan Project using .grove.yml
	orphanDir := filepath.Join(rootDir, "work", "orphan-project")
	require.NoError(t, os.MkdirAll(orphanDir, 0o755))
	orphanCfg := config.Config{Name: "orphan-project"}
	orphanBytes, _ := yaml.Marshal(orphanCfg)
	require.NoError(t, os.WriteFile(filepath.Join(orphanDir, ".grove.yml"), orphanBytes, 0o644))

	// 4. A Non-Grove Directory
	nonGroveDir := filepath.Join(rootDir, "work", "other-dir")
	require.NoError(t, os.MkdirAll(nonGroveDir, 0o755))
	require.NoError(t, os.Mkdir(filepath.Join(nonGroveDir, ".git"), 0o755))

	return rootDir, filepath.Join(rootDir, "home")
}

func TestDiscoveryService(t *testing.T) {
	rootDir, homeDir := setupMockFS(t)

	// Isolate from real user environment
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(homeDir, ".config"))
	t.Setenv("HOME", homeDir)
	t.Setenv("GROVE_CONFIG_OVERLAY", filepath.Join(homeDir, ".config", "grove", "grove.yml"))

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

// TestDiscover_PromoteFromEcosystemWorkspaces verifies that a child git repo
// without its own grove.toml is still discovered as a project when the
// enclosing ecosystem's `workspaces` field explicitly enumerates it. This is
// what enables zero-footprint child repos (kitchen-app, kitchen-core) under
// an ecosystem like kitchen-env.
func TestDiscover_PromoteFromEcosystemWorkspaces(t *testing.T) {
	rootDir := t.TempDir()

	// Global config: register the work dir as a grove
	globalConfigDir := filepath.Join(rootDir, "home", ".config", "grove")
	require.NoError(t, os.MkdirAll(globalConfigDir, 0o755))
	emptyStr := ""
	globalCfg := config.Config{
		SearchPaths: map[string]config.SearchPathConfig{
			"work": {Path: filepath.Join(rootDir, "work"), Enabled: true},
		},
		Context: &config.ContextConfig{ReposDir: &emptyStr},
	}
	globalBytes, _ := yaml.Marshal(globalCfg)
	require.NoError(t, os.WriteFile(filepath.Join(globalConfigDir, "grove.yml"), globalBytes, 0o644))

	// Ecosystem with explicit workspaces enumeration — children are submodules
	// without their own grove.toml markers.
	ecoDir := filepath.Join(rootDir, "work", "kitchen-env")
	require.NoError(t, os.MkdirAll(ecoDir, 0o755))
	ecoCfg := config.Config{
		Name:       "kitchen-env",
		Workspaces: []string{"kitchen-app", "kitchen-core"},
	}
	ecoBytes, _ := yaml.Marshal(ecoCfg)
	require.NoError(t, os.WriteFile(filepath.Join(ecoDir, "grove.yml"), ecoBytes, 0o644))

	// kitchen-app: bare git repo, NO grove.toml — should still be promoted
	// because the parent ecosystem lists it in `workspaces`.
	kappDir := filepath.Join(ecoDir, "kitchen-app")
	require.NoError(t, os.MkdirAll(kappDir, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(kappDir, ".git"), 0o755))

	// kitchen-core: same — bare git repo, listed in parent's workspaces.
	kcoreDir := filepath.Join(ecoDir, "kitchen-core")
	require.NoError(t, os.MkdirAll(kcoreDir, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(kcoreDir, ".git"), 0o755))

	// scratch: bare git repo NOT listed in parent's workspaces — should NOT
	// be promoted, must end up in NonGroveDirectories.
	scratchDir := filepath.Join(ecoDir, "scratch")
	require.NoError(t, os.MkdirAll(scratchDir, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(scratchDir, ".git"), 0o755))

	// Run discovery against the mock home
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(rootDir, "home", ".config"))
	t.Setenv("HOME", filepath.Join(rootDir, "home"))

	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)
	svc := NewDiscoveryService(logger)
	result, err := svc.DiscoverAll()
	require.NoError(t, err)

	projects := make(map[string]Project)
	for _, p := range result.Projects {
		projects[p.Name] = p
	}

	assert.Contains(t, projects, "kitchen-app", "kitchen-app should be promoted via parent ecosystem's workspaces field")
	assert.Contains(t, projects, "kitchen-core", "kitchen-core should be promoted via parent ecosystem's workspaces field")
	assert.NotContains(t, projects, "scratch", "scratch should NOT be promoted (not in ecosystem.workspaces)")

	// scratch should be in NonGroveDirectories instead.
	foundScratch := false
	for _, ngd := range result.NonGroveDirectories {
		if filepath.Base(ngd) == "scratch" {
			foundScratch = true
			break
		}
	}
	assert.True(t, foundScratch, "scratch should appear in NonGroveDirectories")

	// Verify the promoted children are linked to the right ecosystem.
	if kapp, ok := projects["kitchen-app"]; ok {
		assert.Equal(t, ecoDir, kapp.ParentEcosystemPath)
	}
	if kcore, ok := projects["kitchen-core"]; ok {
		assert.Equal(t, ecoDir, kcore.ParentEcosystemPath)
	}
}
