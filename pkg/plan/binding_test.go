package plan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/grovetools/core/pkg/worktreeregistry"
)

func TestResolvePlanBindingsQualifiesDuplicateSlugs(t *testing.T) {
	root := t.TempDir()
	containerA := filepath.Join(root, "containers", "a", "same")
	containerB := filepath.Join(root, "containers", "b", "same")
	for _, dir := range []string{containerA, containerB} {
		if err := mkdirAll(dir); err != nil {
			t.Fatal(err)
		}
	}
	planA := filepath.Join(root, "workspace-a", "plans", "same")
	planB := filepath.Join(root, "workspace-b", "plans", "same")
	resolver := func(path string) (*ResolvedTarget, error) {
		if path == containerA {
			return &ResolvedTarget{ContainerPath: path, PlanDir: planA}, nil
		}
		return &ResolvedTarget{ContainerPath: path, PlanDir: planB}, nil
	}
	got := resolvePlanBindings([]BindingRequest{
		{PlanDir: planA, ConfiguredWorktree: "same"},
		{PlanDir: planB, ConfiguredWorktree: "same"},
	}, []*worktreeregistry.Entry{
		{AbsPath: containerA, Plan: "same"},
		{AbsPath: containerB, Plan: "same"},
	}, resolver)
	if got[NewPlanKey(planA).String()].ContainerPath != containerA || got[NewPlanKey(planA).String()].Health != BindingValid {
		t.Fatalf("workspace A resolved incorrectly: %+v", got[NewPlanKey(planA).String()])
	}
	if got[NewPlanKey(planB).String()].ContainerPath != containerB || got[NewPlanKey(planB).String()].Health != BindingValid {
		t.Fatalf("workspace B resolved incorrectly: %+v", got[NewPlanKey(planB).String()])
	}
}

func TestResolvePlanBindingsAcceptsCanonicalPlanDirSpelling(t *testing.T) {
	root := t.TempDir()
	realPlans := filepath.Join(root, "real", "plans")
	if err := os.MkdirAll(filepath.Join(realPlans, "feature"), 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(filepath.Join(root, "real"), alias); err != nil {
		t.Fatal(err)
	}
	container := filepath.Join(root, "feature")
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatal(err)
	}
	requested := filepath.Join(alias, "plans", "feature")
	resolved := filepath.Join(realPlans, "feature")
	got := resolvePlanBindings(
		[]BindingRequest{{PlanDir: requested, ConfiguredWorktree: "feature"}},
		[]*worktreeregistry.Entry{{AbsPath: container, Plan: "feature"}},
		func(string) (*ResolvedTarget, error) { return &ResolvedTarget{PlanDir: resolved}, nil },
	)
	if binding := got[NewPlanKey(requested).String()]; binding.Health != BindingValid {
		t.Fatalf("canonical spellings did not bind: %+v", binding)
	}
}

func TestResolvePlanBindingsHealth(t *testing.T) {
	root := t.TempDir()
	planDir := filepath.Join(root, "plans", "feature")
	missing := filepath.Join(root, "feature")
	resolver := func(path string) (*ResolvedTarget, error) {
		return &ResolvedTarget{ContainerPath: path, PlanDir: planDir}, nil
	}
	got := resolvePlanBindings([]BindingRequest{{PlanDir: planDir, ConfiguredWorktree: "feature"}}, []*worktreeregistry.Entry{{AbsPath: missing, Plan: "feature"}}, resolver)
	if got[NewPlanKey(planDir).String()].Health != BindingMissing {
		t.Fatalf("got %+v", got)
	}

	got = resolvePlanBindings([]BindingRequest{{PlanDir: planDir, ConfiguredWorktree: "other"}}, []*worktreeregistry.Entry{{AbsPath: root, Plan: "feature"}}, resolver)
	if got[NewPlanKey(planDir).String()].Health != BindingMismatch {
		t.Fatalf("got %+v", got)
	}
}

func mkdirAll(path string) error { return os.MkdirAll(path, 0o755) }

// TestResolvePlanBindingsMissingContainerBeatsConfigDrift covers the
// retained-metadata deleted-container shape: the registry still names the
// container, but the container is gone from disk and the plan config's
// worktree may already have been cleaned up. Container absence must be
// detected BEFORE the qualification/config-agreement comparisons, so the
// binding surfaces the distinct "missing container" health instead of
// collapsing into unbound or binding mismatch.
func TestResolvePlanBindingsMissingContainerBeatsConfigDrift(t *testing.T) {
	root := t.TempDir()
	planDir := filepath.Join(root, "notespaces", "alpha-repo", "plans", "missing-view")
	gone := filepath.Join(root, "alpha-repo", ".grove-worktrees", "missing-view")
	resolver := func(path string) (*ResolvedTarget, error) {
		return &ResolvedTarget{ContainerPath: path, PlanDir: planDir}, nil
	}
	entries := []*worktreeregistry.Entry{{AbsPath: gone, Owner: filepath.Join(root, "alpha-repo"), Plan: "missing-view"}}

	// Config still names the worktree: missing, not mismatch.
	got := resolvePlanBindings([]BindingRequest{{PlanDir: planDir, ConfiguredWorktree: "missing-view"}}, entries, resolver)
	if binding := got[NewPlanKey(planDir).String()]; binding.Health != BindingMissing || binding.ContainerPath != gone {
		t.Fatalf("configured worktree: %+v", binding)
	}

	// Config already cleaned (no worktree): still missing, never the
	// "registry has a container but plan config is unbound" mismatch.
	got = resolvePlanBindings([]BindingRequest{{PlanDir: planDir}}, entries, resolver)
	if binding := got[NewPlanKey(planDir).String()]; binding.Health != BindingMissing {
		t.Fatalf("cleaned config: %+v", binding)
	}

	// A gone container whose derived identity is underivable (no plan dir)
	// still attributes to the only same-named plan and surfaces missing.
	blind := func(path string) (*ResolvedTarget, error) { return &ResolvedTarget{ContainerPath: path}, nil }
	got = resolvePlanBindings([]BindingRequest{{PlanDir: planDir}}, entries, blind)
	if binding := got[NewPlanKey(planDir).String()]; binding.Health != BindingMissing {
		t.Fatalf("underivable identity: %+v", binding)
	}
}

// TestResolvePlanBindingsMissingContainerStaysQualified proves a deleted
// container positively belonging to ANOTHER workspace's same-named plan does
// not leak "missing container" into this plan, and that plans with no entry at
// all keep their distinct unbound state.
func TestResolvePlanBindingsMissingContainerStaysQualified(t *testing.T) {
	root := t.TempDir()
	alphaPlan := filepath.Join(root, "notespaces", "alpha-repo", "plans", "view")
	betaPlan := filepath.Join(root, "notespaces", "beta-repo", "plans", "view")
	goneBeta := filepath.Join(root, "beta-repo", ".grove-worktrees", "view")
	resolver := func(path string) (*ResolvedTarget, error) {
		return &ResolvedTarget{ContainerPath: path, PlanDir: betaPlan}, nil
	}
	entries := []*worktreeregistry.Entry{{AbsPath: goneBeta, Owner: filepath.Join(root, "beta-repo"), Plan: "view"}}

	got := resolvePlanBindings([]BindingRequest{
		{PlanDir: alphaPlan, ConfiguredWorktree: "view"},
		{PlanDir: betaPlan, ConfiguredWorktree: "view"},
		{PlanDir: filepath.Join(root, "notespaces", "alpha-repo", "plans", "solo")},
	}, entries, resolver)

	if binding := got[NewPlanKey(alphaPlan).String()]; binding.Health != BindingMismatch {
		t.Fatalf("alpha must not adopt beta's gone container: %+v", binding)
	}
	if binding := got[NewPlanKey(betaPlan).String()]; binding.Health != BindingMissing {
		t.Fatalf("beta: %+v", binding)
	}
	if binding := got[NewPlanKey(filepath.Join(root, "notespaces", "alpha-repo", "plans", "solo")).String()]; binding.Health != BindingUnbound {
		t.Fatalf("solo: %+v", binding)
	}
}
