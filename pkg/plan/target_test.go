package plan_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grovetools/core/pkg/plan"
	"github.com/grovetools/core/pkg/worktreeregistry"
)

func TestResolveTarget(t *testing.T) {
	t.Run("enriches a registered plan", func(t *testing.T) {
		t.Setenv("GROVE_HOME", t.TempDir())
		dir := t.TempDir()
		require.NoError(t, worktreeregistry.Save(&worktreeregistry.Entry{
			AbsPath: dir,
			Owner:   "/some/owner",
			Repos:   []string{"core", "flow"},
			Plan:    "my-plan",
		}))

		target, err := plan.ResolveTarget(dir)
		require.NoError(t, err)
		require.NotNil(t, target)
		assert.Equal(t, dir, target.ContainerPath)
		assert.Equal(t, "my-plan", target.PlanName)
		assert.Equal(t, "/some/owner", target.Owner)
		assert.Equal(t, []string{"core", "flow"}, target.Repos)
		// WorkspaceRoot defaults to the container path when it is not under a
		// recognized worktree base.
		assert.Equal(t, dir, target.WorkspaceRoot)
	})

	t.Run("resolves by bare plan name", func(t *testing.T) {
		t.Setenv("GROVE_HOME", t.TempDir())
		dir := t.TempDir()
		require.NoError(t, worktreeregistry.Save(&worktreeregistry.Entry{
			AbsPath: dir,
			Plan:    "named-plan",
		}))

		target, err := plan.ResolveTarget("named-plan")
		require.NoError(t, err)
		require.NotNil(t, target)
		assert.Equal(t, dir, target.ContainerPath)
		assert.Equal(t, "named-plan", target.PlanName)
	})

	t.Run("errors on unknown reference", func(t *testing.T) {
		t.Setenv("GROVE_HOME", t.TempDir())
		_, err := plan.ResolveTarget("nope")
		require.Error(t, err)
	})
}
