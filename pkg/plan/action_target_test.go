package plan

import (
	"path/filepath"
	"testing"

	"github.com/grovetools/core/pkg/workspace"
)

func TestResolvePlanActionTargetUsesQualifiedContainerAndDeclaredRepos(t *testing.T) {
	root := t.TempDir()
	container := filepath.Join(root, "containers", "feature")
	planDir := filepath.Join(root, "notebook", "plans", "same")
	provider := workspace.NewProviderFromNodes([]*workspace.WorkspaceNode{
		{Name: "feature", Path: container, Kind: workspace.KindEcosystemWorktree},
		{Name: "repo-b", Path: filepath.Join(container, "repo-b"), Kind: workspace.KindEcosystemWorktreeSubProject, ParentEcosystemPath: container},
		{Name: "repo-a", Path: filepath.Join(container, "repo-a"), Kind: workspace.KindEcosystemWorktreeSubProject, ParentEcosystemPath: container},
		{Name: "same", Path: filepath.Join(root, "decoy"), Kind: workspace.KindStandaloneProjectWorktree},
	})
	binding := PlanBinding{
		Key: NewPlanKey(planDir), Health: BindingValid, RegistryID: "qualified/feature",
		ContainerPath: container, WorkspaceRoot: container,
	}

	target, err := ResolvePlanActionTarget(binding, []string{"repo-b", "repo-a"}, provider)
	if err != nil {
		t.Fatal(err)
	}
	if target.PlanDir != planDir || target.ContainerPath != container || target.RegistryID != "qualified/feature" {
		t.Fatalf("wrong qualified target: %+v", target)
	}
	if len(target.Repos) != 2 || target.Repos[0].Name != "repo-b" || target.Repos[1].Name != "repo-a" {
		t.Fatalf("declared repository order/scope lost: %+v", target.Repos)
	}
}

func TestResolvePlanActionTargetRejectsContainingDiscoveryNode(t *testing.T) {
	root := t.TempDir()
	container := filepath.Join(root, "container", "missing-child")
	provider := workspace.NewProviderFromNodes([]*workspace.WorkspaceNode{{Name: "container", Path: filepath.Dir(container), Kind: workspace.KindEcosystemWorktree}})
	binding := PlanBinding{Key: NewPlanKey(filepath.Join(root, "plans", "p")), Health: BindingValid, RegistryID: "id", ContainerPath: container}
	if _, err := ResolvePlanActionTarget(binding, nil, provider); err == nil {
		t.Fatal("expected containing-node resolution to be refused")
	}
}
