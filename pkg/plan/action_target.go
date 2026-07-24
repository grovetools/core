package plan

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/core/util/pathutil"
)

// RepoTarget is one repository checkout in a qualified plan container.
type RepoTarget struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// PlanActionTarget is the fully-resolved, CWD-independent input to plan actions.
// PlanDir comes from the daemon plan index; the remaining identity is derived
// through the qualified registry binding and canonical workspace discovery.
type PlanActionTarget struct {
	PlanDir       string       `json:"planDir"`
	WorkspaceRoot string       `json:"workspaceRoot,omitempty"`
	RegistryID    string       `json:"registryId,omitempty"`
	ContainerPath string       `json:"containerPath"`
	Repos         []RepoTarget `json:"repos"`
}

// ResolvePlanActionTarget expands a valid qualified binding into a complete
// action target. The previously discovered workspace snapshot is an enrichment
// source: when it covers the bound container, member repos come from canonical
// discovery. When it does not (nil provider, container outside configured
// discovery roots, partial snapshot), membership is enumerated from the
// registry-qualified binding itself — the same canonical identity the binding
// health was resolved from. Neither path consults process CWD or starts a
// competing filesystem scan.
func ResolvePlanActionTarget(binding PlanBinding, declaredRepos []string, provider *workspace.Provider) (PlanActionTarget, error) {
	if !binding.Valid() {
		return PlanActionTarget{}, fmt.Errorf("plan binding is %s", binding.Health)
	}
	if binding.Key.String() == "" || binding.ContainerPath == "" || binding.RegistryID == "" {
		return PlanActionTarget{}, fmt.Errorf("plan binding is incomplete")
	}

	target := PlanActionTarget{
		PlanDir:       binding.Key.String(),
		WorkspaceRoot: binding.WorkspaceRoot,
		RegistryID:    binding.RegistryID,
		ContainerPath: binding.ContainerPath,
	}

	candidates := map[string]RepoTarget{}
	if provider != nil {
		if node := provider.FindByPath(binding.ContainerPath); node != nil {
			// A containing-node match means discovery does not actually know
			// the container; fall through to registry membership rather than
			// targeting the wrong directory.
			if same, err := pathutil.ComparePaths(node.Path, binding.ContainerPath); err == nil && same {
				target.ContainerPath = node.Path
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
			}
		}
	}
	if len(candidates) == 0 {
		candidates = registryRepoCandidates(binding)
	}
	if len(candidates) == 0 {
		return PlanActionTarget{}, fmt.Errorf("container %s has no discovered repositories", target.ContainerPath)
	}

	if target.WorkspaceRoot == "" {
		target.WorkspaceRoot = target.ContainerPath
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
				return PlanActionTarget{}, fmt.Errorf("declared repository %q is absent from container %s", name, target.ContainerPath)
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
	for i := range target.Repos {
		target.Repos[i].Path = filepath.Clean(target.Repos[i].Path)
	}
	return target, nil
}

// registryRepoCandidates enumerates a container's member checkouts from its
// registry-backed binding: every recorded repo name is a direct child checkout
// of the container (the shape workspace.Prepare provisions). Entries that
// predate recorded membership fall back to the container's on-disk direct
// children that are git checkouts. Membership is rooted at the qualified
// container path only — never process CWD.
func registryRepoCandidates(binding PlanBinding) map[string]RepoTarget {
	candidates := map[string]RepoTarget{}
	for _, name := range binding.Repos {
		path := filepath.Join(binding.ContainerPath, name)
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			candidates[name] = RepoTarget{Name: name, Path: path}
		}
	}
	if len(candidates) > 0 {
		return candidates
	}
	children, err := os.ReadDir(binding.ContainerPath)
	if err != nil {
		return candidates
	}
	for _, child := range children {
		if !child.IsDir() || strings.HasPrefix(child.Name(), ".") {
			continue
		}
		path := filepath.Join(binding.ContainerPath, child.Name())
		if _, statErr := os.Stat(filepath.Join(path, ".git")); statErr == nil {
			candidates[child.Name()] = RepoTarget{Name: child.Name(), Path: path}
		}
	}
	return candidates
}
