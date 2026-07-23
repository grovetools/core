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
