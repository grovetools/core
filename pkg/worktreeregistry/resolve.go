package worktreeregistry

import (
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

func reposContain(repos []string, name string) bool {
	for _, r := range repos {
		if r == name {
			return true
		}
	}
	return false
}
