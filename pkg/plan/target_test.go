package plan_test

import (
	"os"
	"path/filepath"
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

// newLegacyPlanFixture builds the on-disk shape `flow plan init <name>
// --worktree --layout legacy` produces for a STANDALONE repo: the owner repo,
// a unified container at <owner>/.grove-worktrees/<plan> (synthetic grove.toml
// plus .grove/workspace marker), and the registry entry binding them.
func newLegacyPlanFixture(t *testing.T, root, repoName, planName string) (owner, container string) {
	t.Helper()
	owner = filepath.Join(root, repoName)
	require.NoError(t, os.MkdirAll(filepath.Join(owner, ".git"), 0o755))
	container = filepath.Join(owner, ".grove-worktrees", planName)
	require.NoError(t, os.MkdirAll(filepath.Join(container, ".grove"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(container, "grove.toml"), []byte("workspaces = [\"*\"]\n"), 0o644))
	marker := "branch: " + planName + "\nplan: " + planName + "\nowner: " + owner + "\necosystem: true\n"
	require.NoError(t, os.WriteFile(filepath.Join(container, ".grove", "workspace"), []byte(marker), 0o644))
	require.NoError(t, worktreeregistry.Save(&worktreeregistry.Entry{AbsPath: container, Owner: owner, Plan: planName}))
	return owner, container
}

// notebookPlanDir is the default-notebook plans path GetPlansDir derives when
// no notebook is configured (root_dir ~/.grove/notebooks/nb).
func notebookPlanDir(home, workspaceName, planName string) string {
	return filepath.Join(home, ".grove", "notebooks", "nb", "workspaces", workspaceName, "plans", planName)
}

func canonicalTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return dir
}

func TestResolveTargetOwnerQualifiedPlanDir(t *testing.T) {
	t.Run("legacy standalone-repo container qualifies by the owner workspace", func(t *testing.T) {
		t.Setenv("GROVE_HOME", t.TempDir())
		home := t.TempDir()
		t.Setenv("HOME", home)
		root := canonicalTempDir(t)

		_, container := newLegacyPlanFixture(t, root, "alpha-repo", "alpha-view")

		target, err := plan.ResolveTarget(container)
		require.NoError(t, err)
		// The plan dir must be qualified by the OWNER repo's workspace — the
		// place `flow plan init` created it — never by the container basename
		// (which is the plan name and can therefore never match).
		assert.Equal(t, notebookPlanDir(home, "alpha-repo", "alpha-view"), target.PlanDir)
		assert.NotEqual(t, notebookPlanDir(home, "alpha-view", "alpha-view"), target.PlanDir)
		assert.Equal(t, container, target.ContainerPath)
	})

	t.Run("deleted container still derives the plan dir through the owner", func(t *testing.T) {
		t.Setenv("GROVE_HOME", t.TempDir())
		home := t.TempDir()
		t.Setenv("HOME", home)
		root := canonicalTempDir(t)

		owner := filepath.Join(root, "alpha-repo")
		require.NoError(t, os.MkdirAll(filepath.Join(owner, ".git"), 0o755))
		container := filepath.Join(owner, ".grove-worktrees", "gone-view")
		require.NoError(t, worktreeregistry.Save(&worktreeregistry.Entry{AbsPath: container, Owner: owner, Plan: "gone-view"}))

		target, err := plan.ResolveTarget(container)
		require.NoError(t, err)
		assert.Equal(t, notebookPlanDir(home, "alpha-repo", "gone-view"), target.PlanDir)
	})
}

func TestResolvePlanBindingsOwnerQualified(t *testing.T) {
	t.Setenv("GROVE_HOME", t.TempDir())
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := canonicalTempDir(t)

	// Two standalone repos each host a legacy plan with the SAME leaf name.
	_, alphaContainer := newLegacyPlanFixture(t, root, "alpha-repo", "view")
	_, betaContainer := newLegacyPlanFixture(t, root, "beta-repo", "view")
	alphaPlanDir := notebookPlanDir(home, "alpha-repo", "view")
	betaPlanDir := notebookPlanDir(home, "beta-repo", "view")
	require.NoError(t, os.MkdirAll(alphaPlanDir, 0o755))
	require.NoError(t, os.MkdirAll(betaPlanDir, 0o755))

	// A registered plan whose container was deleted must qualify through the
	// owner and surface BindingMissing — not collapse to unbound/mismatch.
	goneOwner := filepath.Join(root, "gone-repo")
	require.NoError(t, os.MkdirAll(filepath.Join(goneOwner, ".git"), 0o755))
	goneContainer := filepath.Join(goneOwner, ".grove-worktrees", "gone-view")
	require.NoError(t, worktreeregistry.Save(&worktreeregistry.Entry{AbsPath: goneContainer, Owner: goneOwner, Plan: "gone-view"}))
	gonePlanDir := notebookPlanDir(home, "gone-repo", "gone-view")
	require.NoError(t, os.MkdirAll(gonePlanDir, 0o755))

	unboundPlanDir := notebookPlanDir(home, "alpha-repo", "solo-plan")
	require.NoError(t, os.MkdirAll(unboundPlanDir, 0o755))

	bindings := plan.ResolvePlanBindings([]plan.BindingRequest{
		{PlanDir: alphaPlanDir, ConfiguredWorktree: "view"},
		{PlanDir: betaPlanDir, ConfiguredWorktree: "view"},
		{PlanDir: gonePlanDir, ConfiguredWorktree: "gone-view"},
		{PlanDir: unboundPlanDir, ConfiguredWorktree: "solo-view"},
		{PlanDir: alphaPlanDir + "-archived", Archived: true},
	})

	// A freshly generated, registered legacy plan is a VALID binding that
	// resolves to its exact container.
	alpha := bindings[plan.NewPlanKey(alphaPlanDir).String()]
	require.Equal(t, plan.BindingValid, alpha.Health, "alpha binding: %+v", alpha)
	assert.Equal(t, alphaContainer, alpha.ContainerPath)

	// Two same-leaf plans under different owners resolve independently.
	beta := bindings[plan.NewPlanKey(betaPlanDir).String()]
	require.Equal(t, plan.BindingValid, beta.Health, "beta binding: %+v", beta)
	assert.Equal(t, betaContainer, beta.ContainerPath)
	assert.NotEqual(t, alpha.ContainerPath, beta.ContainerPath)

	// Genuine refusals keep their distinct health states.
	assert.Equal(t, plan.BindingMissing, bindings[plan.NewPlanKey(gonePlanDir).String()].Health)
	assert.Equal(t, plan.BindingUnbound, bindings[plan.NewPlanKey(unboundPlanDir).String()].Health)
	assert.Equal(t, plan.BindingArchived, bindings[plan.NewPlanKey(alphaPlanDir+"-archived").String()].Health)
}

func TestResolvePlanBindingsOwnerQualifiedMismatch(t *testing.T) {
	t.Setenv("GROVE_HOME", t.TempDir())
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := canonicalTempDir(t)

	_, _ = newLegacyPlanFixture(t, root, "alpha-repo", "mm-view")
	planDir := notebookPlanDir(home, "alpha-repo", "mm-view")
	require.NoError(t, os.MkdirAll(planDir, 0o755))

	// The registry container qualifies, but the plan config names a different
	// worktree — a TRUE mismatch must still refuse.
	binding := plan.ResolvePlanBinding(plan.NewPlanKey(planDir), "other-worktree", false)
	assert.Equal(t, plan.BindingMismatch, binding.Health)
}
