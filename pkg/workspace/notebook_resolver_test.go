package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/grovetools/core/config"
)

// setupGroveEcosystem creates a grove directory that IS itself an ecosystem
// (the single-ecosystem grove shape: grove.Path points directly at the
// ecosystem root), plus one sub-project inside it.
func setupGroveEcosystem(t *testing.T) (grovePath, subProjectName string) {
	t.Helper()
	rootDir := t.TempDir()

	// Isolate from the developer's real config so the DiscoverAll fallback
	// (which loads the default config) cannot find anything: if the lookup
	// succeeds, it MUST have come from the fast path under test.
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(rootDir, "home", ".config"))

	grovePath = filepath.Join(rootDir, "groves", "my-ecosystem")
	require.NoError(t, os.MkdirAll(grovePath, 0o755))
	ecoCfg := config.Config{Name: "my-ecosystem", Workspaces: []string{"*"}}
	ecoBytes, err := yaml.Marshal(ecoCfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(grovePath, "grove.yml"), ecoBytes, 0o644))

	subProjectName = "project-a"
	projDir := filepath.Join(grovePath, subProjectName)
	require.NoError(t, os.MkdirAll(projDir, 0o755))
	projCfg := config.Config{Name: subProjectName}
	projBytes, err := yaml.Marshal(projCfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(projDir, "grove.yml"), projBytes, 0o644))

	return grovePath, subProjectName
}

// TestFindProjectByWorkspaceName_GrovePathIsWorkspace locks in the fast-path
// fix for single-ecosystem groves: when the grove path itself IS the workspace
// (Base(grovePath) == workspaceName), the old code only probed
// <grove>/<name>/<name> — which never exists — and fell through to a full
// DiscoverAll walk on EVERY call. That walk can transiently return nothing
// while racing the daemon's workspace collectors, which is how notebook plan
// dirs intermittently failed to resolve to their project.
func TestFindProjectByWorkspaceName_GrovePathIsWorkspace(t *testing.T) {
	grovePath, _ := setupGroveEcosystem(t)

	cfg := &config.Config{
		Groves: map[string]config.GroveSourceConfig{
			"main": {Path: grovePath},
		},
	}

	node := findProjectByWorkspaceName("my-ecosystem", cfg)
	require.NotNil(t, node, "grove path that IS the workspace must resolve via the fast path, without discovery")
	assert.Equal(t, "my-ecosystem", node.Name)
	assert.Equal(t, normalizePath(t, grovePath), node.Path)
	assert.Equal(t, KindEcosystemRoot, node.Kind)
}

// TestFindProjectByWorkspaceName_SubProjectJoinStillWorks confirms the
// original join-based fast path (<grove>/<workspaceName>) is untouched.
func TestFindProjectByWorkspaceName_SubProjectJoinStillWorks(t *testing.T) {
	grovePath, subName := setupGroveEcosystem(t)

	cfg := &config.Config{
		Groves: map[string]config.GroveSourceConfig{
			"main": {Path: grovePath},
		},
	}

	node := findProjectByWorkspaceName(subName, cfg)
	require.NotNil(t, node, "sub-project under the grove must resolve via the join fast path")
	assert.Equal(t, subName, node.Name)
	assert.Equal(t, normalizePath(t, filepath.Join(grovePath, subName)), node.Path)
}
