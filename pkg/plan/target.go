package plan

import (
	"fmt"

	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/core/pkg/worktreeregistry"
)

// ResolvedTarget is the enriched bundle describing a flow target — the
// worktree container plus the plan it hosts and the derived workspace/plan
// directories. It is produced by ResolveTarget from a single user-supplied
// reference (`flow --at <ref>`) so every flow subcommand resolves the same
// way regardless of cwd.
//
// The split is deliberate: the bare registry lookup (name|id|path → Entry)
// lives in core/pkg/worktreeregistry (which must NOT import workspace), while
// the plan-dir/workspace-root enrichment lives here in core/pkg/plan (which
// already depends on workspace and worktreeregistry).
type ResolvedTarget struct {
	// ContainerPath is the absolute worktree container directory.
	ContainerPath string

	// PlanName is the grove-flow plan recorded for the worktree.
	PlanName string

	// PlanDir is the absolute path to the plan's directory
	// (<plansDir>/<planName>), or empty when it cannot be derived.
	PlanDir string

	// WorkspaceRoot is the worktree root containing ContainerPath, or
	// ContainerPath itself when it is already a worktree root.
	WorkspaceRoot string

	// Owner is the absolute path to the git root that owns the worktree.
	Owner string

	// Repos is the list of workspace/repo names present in the worktree.
	Repos []string
}

// ResolveTarget resolves a user-supplied reference into an enriched
// ResolvedTarget. ref accepts everything FindByRef accepts: an absolute
// container path, a "<container-id>/<name>" pair, or a bare plan name.
//
// Enrichment derives WorkspaceRoot via workspace.WorktreeRootForPath and
// PlanDir via ResolvePlanDir rooted at the container path, so callers receive
// a directory they can hand straight to the existing plan-loading code.
func ResolveTarget(ref string) (*ResolvedTarget, error) {
	entry, err := worktreeregistry.FindByRef(ref)
	if err != nil {
		return nil, err
	}
	if entry == nil || entry.AbsPath == "" {
		return nil, fmt.Errorf("resolved target %q has no container path", ref)
	}

	target := &ResolvedTarget{
		ContainerPath: entry.AbsPath,
		PlanName:      entry.Plan,
		Owner:         entry.Owner,
		Repos:         entry.Repos,
	}

	if root, ok := workspace.WorktreeRootForPath(entry.AbsPath); ok {
		target.WorkspaceRoot = root
	} else {
		target.WorkspaceRoot = entry.AbsPath
	}

	if entry.Plan != "" {
		target.PlanDir = ResolvePlanDir(target.WorkspaceRoot, entry.Plan)
	}

	return target, nil
}
