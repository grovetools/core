package plan

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/grovetools/core/pkg/worktreeregistry"
	"github.com/grovetools/core/util/pathutil"
)

// PlanKey is the globally-qualified identity of a plan. PlanDir, sourced from
// the daemon plan index, is authoritative; plan slugs are display values only.
type PlanKey struct {
	PlanDir string
}

// NewPlanKey accepts only an absolute plan directory. Refusing relative input
// prevents this identity layer from becoming another CWD-based resolver.
func NewPlanKey(planDir string) PlanKey {
	if !filepath.IsAbs(planDir) {
		return PlanKey{}
	}
	return PlanKey{PlanDir: filepath.Clean(planDir)}
}

func (k PlanKey) String() string { return k.PlanDir }

// BindingHealth describes whether a qualified plan can safely target a live
// worktree container.
type BindingHealth string

const (
	BindingValid     BindingHealth = "valid"
	BindingUnbound   BindingHealth = "unbound"
	BindingMissing   BindingHealth = "missing container"
	BindingDuplicate BindingHealth = "duplicate bindings"
	BindingMismatch  BindingHealth = "binding mismatch"
	BindingArchived  BindingHealth = "archived"
)

// PlanBinding is the registry-backed association used by plan actions. It is
// resolved from a PlanKey; it never resolves a bare plan name or process CWD.
type PlanBinding struct {
	Key           PlanKey
	Health        BindingHealth
	RegistryID    string
	ContainerPath string
	WorkspaceRoot string
	PlanName      string
	Reason        string
}

func (b PlanBinding) Valid() bool { return b.Health == BindingValid }

type BindingRequest struct {
	PlanDir            string
	WorkspaceRoot      string // display provenance only; never used to select a registry entry
	ConfiguredWorktree string
	Archived           bool
}

// ResolvePlanBinding resolves one qualified identity. configuredWorktree is
// compatibility metadata only: it validates, but never selects, a container.
func ResolvePlanBinding(key PlanKey, configuredWorktree string, archived bool) PlanBinding {
	return ResolvePlanBindings([]BindingRequest{{PlanDir: key.PlanDir, ConfiguredWorktree: configuredWorktree, Archived: archived}})[key.String()]
}

// ResolvePlanBindings resolves a portfolio with one registry scan and one
// canonical workspace enrichment per registry entry.
func ResolvePlanBindings(requests []BindingRequest) map[string]PlanBinding {
	entries, err := worktreeregistry.ListAll()
	if err != nil {
		out := make(map[string]PlanBinding, len(requests))
		for _, req := range requests {
			key := NewPlanKey(req.PlanDir)
			out[key.String()] = PlanBinding{Key: key, Health: BindingMismatch, Reason: fmt.Sprintf("registry unavailable: %v", err)}
		}
		return out
	}
	return resolvePlanBindings(requests, entries, ResolveTarget)
}

func resolvePlanBindings(requests []BindingRequest, entries []*worktreeregistry.Entry, resolver func(string) (*ResolvedTarget, error)) map[string]PlanBinding {
	targets := make(map[*worktreeregistry.Entry]*ResolvedTarget, len(entries))
	for _, entry := range entries {
		if entry != nil {
			targets[entry], _ = resolver(entry.AbsPath)
		}
	}
	out := make(map[string]PlanBinding, len(requests))
	for _, req := range requests {
		key := NewPlanKey(req.PlanDir)
		binding := PlanBinding{Key: key, Health: BindingUnbound}
		if req.Archived {
			binding.Health, binding.Reason = BindingArchived, "plan is archived"
			out[key.String()] = binding
			continue
		}
		if key.String() == "" {
			binding.Health, binding.Reason = BindingMismatch, "plan has no absolute qualified directory"
			out[key.String()] = binding
			continue
		}
		binding.PlanName = filepath.Base(key.PlanDir)
		var qualified []*worktreeregistry.Entry
		var sameName int
		for _, entry := range entries {
			if entry == nil || entry.IsArchived() || entry.Plan != binding.PlanName {
				continue
			}
			sameName++
			if target := targets[entry]; target != nil && NewPlanKey(target.PlanDir) == key {
				qualified = append(qualified, entry)
			}
		}
		switch len(qualified) {
		case 0:
			if sameName > 0 {
				binding.Health, binding.Reason = BindingMismatch, "same-named registry entries belong to other workspaces"
			} else if req.ConfiguredWorktree != "" {
				binding.Reason = "configured worktree is not registered"
			} else {
				binding.Reason = "plan has no worktree"
			}
		case 1:
			entry := qualified[0]
			binding.RegistryID = pathutil.WorktreeID(entry.AbsPath)
			binding.ContainerPath = entry.AbsPath
			if target := targets[entry]; target != nil {
				binding.WorkspaceRoot = target.WorkspaceRoot
			}
			switch {
			case req.ConfiguredWorktree == "":
				binding.Health, binding.Reason = BindingMismatch, "registry has a container but plan config is unbound"
			case req.ConfiguredWorktree != filepath.Base(entry.AbsPath):
				binding.Health = BindingMismatch
				binding.Reason = fmt.Sprintf("configured worktree %q does not match container %q", req.ConfiguredWorktree, filepath.Base(entry.AbsPath))
			case func() bool { info, statErr := os.Stat(entry.AbsPath); return statErr != nil || !info.IsDir() }():
				binding.Health, binding.Reason = BindingMissing, "registered container is unavailable"
			default:
				binding.Health = BindingValid
			}
		default:
			binding.Health = BindingDuplicate
			paths := make([]string, 0, len(qualified))
			for _, entry := range qualified {
				paths = append(paths, entry.AbsPath)
			}
			sort.Strings(paths)
			binding.Reason = fmt.Sprintf("multiple live containers: %v", paths)
		}
		out[key.String()] = binding
	}
	return out
}
