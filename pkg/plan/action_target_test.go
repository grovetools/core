package plan

import (
	"os"
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

func TestResolvePlanActionTargetResolvesFromRegistryMembership(t *testing.T) {
	root := t.TempDir()
	container := filepath.Join(root, "alpha-repo", ".grove-worktrees", "alpha-view")
	repoCheckout := filepath.Join(container, "alpha-repo")
	if err := os.MkdirAll(repoCheckout, 0o755); err != nil {
		t.Fatal(err)
	}
	planDir := filepath.Join(root, "notebook", "workspaces", "alpha-repo", "plans", "alpha-view")
	binding := PlanBinding{
		Key: NewPlanKey(planDir), Health: BindingValid, RegistryID: "alpha-repo/alpha-view",
		ContainerPath: container, WorkspaceRoot: container, Repos: []string{"alpha-repo"},
	}

	// Discovery is blind to the container in both shapes a live portfolio can
	// hit: no provider at all, and a provider whose snapshot has no node for
	// the container. A valid registry-qualified binding must still expand to a
	// complete target in both.
	for name, provider := range map[string]*workspace.Provider{
		"nil provider":      nil,
		"unaware discovery": workspace.NewProviderFromNodes(nil),
	} {
		target, err := ResolvePlanActionTarget(binding, nil, provider)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if target.PlanDir != planDir || target.ContainerPath != container {
			t.Fatalf("%s: wrong qualified target: %+v", name, target)
		}
		if len(target.Repos) != 1 || target.Repos[0].Name != "alpha-repo" || target.Repos[0].Path != repoCheckout {
			t.Fatalf("%s: registry membership lost: %+v", name, target.Repos)
		}
	}
}

func TestResolvePlanActionTargetScansCheckoutsWhenMembershipUnrecorded(t *testing.T) {
	root := t.TempDir()
	container := filepath.Join(root, ".grove-worktrees", "view")
	checkout := filepath.Join(container, "repo")
	if err := os.MkdirAll(filepath.Join(checkout, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Non-checkout children and hidden dirs must not become repo targets.
	if err := os.MkdirAll(filepath.Join(container, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	binding := PlanBinding{
		Key: NewPlanKey(filepath.Join(root, "plans", "view")), Health: BindingValid,
		RegistryID: "id", ContainerPath: container,
	}

	target, err := ResolvePlanActionTarget(binding, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(target.Repos) != 1 || target.Repos[0].Path != checkout {
		t.Fatalf("checkout scan resolved wrong membership: %+v", target.Repos)
	}
}
