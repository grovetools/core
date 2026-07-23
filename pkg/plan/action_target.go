package plan

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/core/util/pathutil"
)

// RepoTarget is one repository checkout in a qualified plan container.
type RepoTarget struct {
	Name string
	Path string
}

// PlanActionTarget is the fully-resolved, CWD-independent input to plan actions.
// PlanDir comes from the daemon plan index; the remaining identity is derived
// through the qualified registry binding and canonical workspace discovery.
type PlanActionTarget struct {
	PlanDir       string
	WorkspaceRoot string
	RegistryID    string
	ContainerPath string
	Repos         []RepoTarget
}

// ResolvePlanActionTarget expands a valid qualified binding with a previously
// discovered workspace snapshot. It performs no filesystem discovery itself:
// callers must share the canonical Provider they already use for workspace
// projection instead of starting a competing scan or consulting process CWD.
func ResolvePlanActionTarget(binding PlanBinding, declaredRepos []string, provider *workspace.Provider) (PlanActionTarget, error) {
	if !binding.Valid() {
		return PlanActionTarget{}, fmt.Errorf("plan binding is %s", binding.Health)
	}
	if binding.Key.String() == "" || binding.ContainerPath == "" || binding.RegistryID == "" {
		return PlanActionTarget{}, fmt.Errorf("plan binding is incomplete")
	}
	if provider == nil {
		return PlanActionTarget{}, fmt.Errorf("workspace discovery is unavailable")
	}

	node := provider.FindByPath(binding.ContainerPath)
	if node == nil {
		return PlanActionTarget{}, fmt.Errorf("container is absent from canonical workspace discovery: %s", binding.ContainerPath)
	}
	if same, err := pathutil.ComparePaths(node.Path, binding.ContainerPath); err != nil || !same {
		return PlanActionTarget{}, fmt.Errorf("workspace discovery resolved %s to containing node %s", binding.ContainerPath, node.Path)
	}

	target := PlanActionTarget{
		PlanDir:       binding.Key.String(),
		WorkspaceRoot: binding.WorkspaceRoot,
		RegistryID:    binding.RegistryID,
		ContainerPath: node.Path,
	}
	if target.WorkspaceRoot == "" {
		target.WorkspaceRoot = node.Path
	}

	candidates := map[string]RepoTarget{}
	if node.IsEcosystem() {
		for _, child := range node.GetDirectChildren(provider.All()) {
			if child == nil || child.Path == "" || child.IsEcosystem() {
				continue
			}
			candidates[child.Name] = RepoTarget{Name: child.Name, Path: child.Path}
		}
	} else {
		candidates[node.Name] = RepoTarget{Name: node.Name, Path: node.Path}
	}

	if len(declaredRepos) > 0 {
		seen := make(map[string]bool, len(declaredRepos))
		for _, name := range declaredRepos {
			if seen[name] {
				continue
			}
			seen[name] = true
			repo, ok := candidates[name]
			if !ok {
				return PlanActionTarget{}, fmt.Errorf("declared repository %q is absent from container %s", name, node.Path)
			}
			target.Repos = append(target.Repos, repo)
		}
	} else {
		for _, repo := range candidates {
			target.Repos = append(target.Repos, repo)
		}
		sort.Slice(target.Repos, func(i, j int) bool {
			if target.Repos[i].Name != target.Repos[j].Name {
				return target.Repos[i].Name < target.Repos[j].Name
			}
			return target.Repos[i].Path < target.Repos[j].Path
		})
	}
	if len(target.Repos) == 0 {
		return PlanActionTarget{}, fmt.Errorf("container %s has no discovered repositories", node.Path)
	}
	for i := range target.Repos {
		target.Repos[i].Path = filepath.Clean(target.Repos[i].Path)
	}
	return target, nil
}
