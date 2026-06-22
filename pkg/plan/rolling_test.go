package plan_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grovetools/core/pkg/plan"
)

// newRepoWorkspace returns a temp directory that GetProjectByPath classifies as
// a (non-grove) repo workspace by planting a .git marker, so ResolvePlansDir
// resolves a plans dir for it. The default notebook layout is centralized under
// ~/.grove/notebooks/nb, so HOME (and GROVE_HOME) are pointed at scratch dirs to
// keep the rolling plan the test creates out of the developer's real home.
func newRepoWorkspace(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GROVE_HOME", t.TempDir())
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	return dir
}

func TestEnsureRollingPlan(t *testing.T) {
	t.Run("creates dir and marker, then is idempotent", func(t *testing.T) {
		dir := newRepoWorkspace(t)

		planDir, created, err := plan.EnsureRollingPlan(dir)
		require.NoError(t, err)
		assert.True(t, created, "first call should report created")
		assert.Equal(t, plan.RollingPlanName, filepath.Base(planDir))

		marker := filepath.Join(planDir, ".grove-plan.yml")
		info, statErr := os.Stat(marker)
		require.NoError(t, statErr)
		assert.False(t, info.IsDir(), "marker should be a file")

		// Second call: idempotent — created==false, same dir, marker intact.
		planDir2, created2, err2 := plan.EnsureRollingPlan(dir)
		require.NoError(t, err2)
		assert.False(t, created2, "second call must not re-create")
		assert.Equal(t, planDir, planDir2)

		body, readErr := os.ReadFile(marker)
		require.NoError(t, readErr)
		assert.Contains(t, string(body), "Rolling plan")
	})

	t.Run("heals a dir that lost its .grove-plan.yml", func(t *testing.T) {
		dir := newRepoWorkspace(t)

		planDir, created, err := plan.EnsureRollingPlan(dir)
		require.NoError(t, err)
		require.True(t, created)

		// Delete the marker but keep the directory — simulates a partial state.
		marker := filepath.Join(planDir, ".grove-plan.yml")
		require.NoError(t, os.Remove(marker))

		planDir2, created2, err2 := plan.EnsureRollingPlan(dir)
		require.NoError(t, err2)
		assert.True(t, created2, "missing marker must be re-written (heal)")
		assert.Equal(t, planDir, planDir2)
		_, statErr := os.Stat(marker)
		require.NoError(t, statErr, "marker should exist again after heal")
	})

	t.Run("errors when not inside a workspace", func(t *testing.T) {
		t.Setenv("GROVE_HOME", t.TempDir())
		dir := t.TempDir() // no .git, no grove config => unresolvable workspace

		_, created, err := plan.EnsureRollingPlan(dir)
		require.Error(t, err)
		assert.False(t, created)
	})
}

func TestRollingPlanDir(t *testing.T) {
	dir := newRepoWorkspace(t)

	got := plan.RollingPlanDir(dir)
	require.NotEmpty(t, got)
	assert.Equal(t, plan.RollingPlanName, filepath.Base(got))
}
