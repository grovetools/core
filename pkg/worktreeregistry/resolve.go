package worktreeregistry

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/grovetools/core/pkg/paths"
	"github.com/grovetools/core/util/pathutil"
)

// Resolve loads the registry entry for absPath (using pathutil.WorktreeID as
// the key) and overlays liveRepos onto the returned entry so structural facts
// from the filesystem always take precedence over cached values.
//
// Anchor logic: AnchorOverride is used when it is non-empty AND it matches a
// member of liveRepos; otherwise the anchor defaults to entry.Owner.
//
// Returns (nil, nil) when no registry entry exists for absPath — callers
// should treat this the same as an empty structural default.
func Resolve(absPath string, liveRepos []string) (*Entry, error) {
	id := pathutil.WorktreeID(absPath)
	entry, err := Load(id)
	if err != nil {
		return nil, nil //nolint:nilerr // not-found is a valid empty state
	}

	// Overlay live structural facts.
	if len(liveRepos) > 0 {
		entry.Repos = liveRepos
	}

	// Heal anchor: if AnchorOverride no longer names a live repo, clear it.
	if entry.AnchorOverride != "" && !reposContain(entry.Repos, entry.AnchorOverride) {
		entry.AnchorOverride = ""
	}

	return entry, nil
}

// PlanForPath returns the grove-flow plan recorded for the worktree whose
// absolute path is absPath (the worktree container root). ok is false when no
// registry entry exists or it records no plan.
func PlanForPath(absPath string) (plan string, ok bool) {
	entry, err := Load(pathutil.WorktreeID(absPath))
	if err != nil || entry == nil {
		return "", false
	}
	return entry.Plan, entry.Plan != ""
}

// FindByRef resolves a user-supplied reference to a registry Entry. ref may be:
//
//  1. An absolute container path → Load(WorktreeID(ref)).
//  2. A "<container-id>/<name>" pair → joined under paths.WorktreesDir() and
//     loaded as an absolute container path.
//  3. A bare plan name → ListAll() filtered by Entry.Plan == ref. A unique
//     match wins; multiple matches produce a clear error mirroring the flow
//     "multiple plans found named '%s'" phrasing.
//
// Returns a non-nil error when nothing matches or when the reference is
// ambiguous. This is the reverse of PlanForPath: name|id → entry.
func FindByRef(ref string) (*Entry, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("empty target reference")
	}

	// 1. Absolute container path.
	if filepath.IsAbs(ref) {
		entry, err := Load(pathutil.WorktreeID(ref))
		if err != nil {
			return nil, fmt.Errorf("no worktree registered at %q: %w", ref, err)
		}
		return entry, nil
	}

	// 2. "<container-id>/<name>" relative reference, joined under the XDG
	//    worktrees base. Only attempt this when the joined path exists on disk
	//    so a slash-bearing plan name (e.g. a branch-style plan) still falls
	//    through to the plan-name scan below.
	if strings.Contains(ref, string(filepath.Separator)) {
		if base := paths.WorktreesDir(); base != "" {
			candidate := filepath.Join(base, ref)
			if _, statErr := os.Stat(candidate); statErr == nil {
				if entry, err := Load(pathutil.WorktreeID(candidate)); err == nil {
					return entry, nil
				}
			}
		}
	}

	// 3. Bare plan name → scan ListAll() for Entry.Plan == ref.
	all, err := ListAll()
	if err != nil {
		return nil, fmt.Errorf("list registry: %w", err)
	}
	var matches []*Entry
	for _, e := range all {
		if e != nil && e.Plan == ref {
			matches = append(matches, e)
		}
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no worktree found for target %q", ref)
	case 1:
		return matches[0], nil
	default:
		absPaths := make([]string, 0, len(matches))
		for _, m := range matches {
			absPaths = append(absPaths, m.AbsPath)
		}
		return nil, fmt.Errorf("multiple worktrees found for plan %q: %s", ref, strings.Join(absPaths, ", "))
	}
}

func reposContain(repos []string, name string) bool {
	for _, r := range repos {
		if r == name {
			return true
		}
	}
	return false
}
