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
