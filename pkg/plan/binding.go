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
	// Repos is the registry-recorded member list of the bound container. It
	// lets action-target expansion enumerate checkouts from the qualified
	// registry identity when canonical discovery cannot see the container.
	Repos []string
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
		var absent []*worktreeregistry.Entry
		var sameName int
		for _, entry := range entries {
			if entry == nil || entry.IsArchived() || entry.Plan != binding.PlanName {
				continue
			}
			sameName++
			target := targets[entry]
			if containerAbsent(entry.AbsPath) {
				// Container-absent entries are classified BEFORE the qualification
				// and config-agreement comparisons: without the container on disk
				// the qualified plan dir may be underivable and the configured
				// worktree may already have been cleaned up, so those checks would
				// collapse a deleted container into unbound/mismatch. Attribute the
				// entry to this plan unless its derived identity positively names
				// another workspace's plan.
				if target == nil || target.PlanDir == "" || samePlanDir(target.PlanDir, key.PlanDir) {
					absent = append(absent, entry)
				}
				continue
			}
			if target != nil && samePlanDir(target.PlanDir, key.PlanDir) {
				qualified = append(qualified, entry)
			}
		}
		switch len(qualified) {
		case 0:
			if len(absent) > 0 {
				sort.Slice(absent, func(i, j int) bool { return absent[i].AbsPath < absent[j].AbsPath })
				entry := absent[0]
				binding.RegistryID = pathutil.WorktreeID(entry.AbsPath)
				binding.ContainerPath = entry.AbsPath
				if target := targets[entry]; target != nil {
					binding.WorkspaceRoot = target.WorkspaceRoot
				}
				binding.Health, binding.Reason = BindingMissing, "registered container is unavailable"
			} else if sameName > 0 {
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
			binding.Repos = append([]string(nil), entry.Repos...)
			if target := targets[entry]; target != nil {
				binding.WorkspaceRoot = target.WorkspaceRoot
			}
			switch {
			case req.ConfiguredWorktree == "":
				binding.Health, binding.Reason = BindingMismatch, "registry has a container but plan config is unbound"
			case req.ConfiguredWorktree != filepath.Base(entry.AbsPath):
				binding.Health = BindingMismatch
				binding.Reason = fmt.Sprintf("configured worktree %q does not match container %q", req.ConfiguredWorktree, filepath.Base(entry.AbsPath))
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

// containerAbsent reports whether a registered container path is gone from
// disk (or is not a directory).
func containerAbsent(path string) bool {
	info, err := os.Stat(path)
	return err != nil || !info.IsDir()
}

// samePlanDir compares plan identities through the canonical path normalizer.
// Registry enrichment commonly returns a symlink-resolved spelling (for
// example /private/tmp on macOS) while the daemon index retains the configured
// plans-directory spelling. Those are one qualified plan, not a mismatch.
func samePlanDir(left, right string) bool {
	if left == "" || right == "" {
		return false
	}
	equal, err := pathutil.ComparePaths(left, right)
	return err == nil && equal
}
