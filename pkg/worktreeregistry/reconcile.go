package worktreeregistry

import (
	"os"
	"path/filepath"
	"strings"
)

// Reconcile performs a set-diff between the registry and the live filesystem:
//
//  1. Any registered entry whose AbsPath no longer exists on disk is deleted.
//  2. Any directory found under xdgBase at two levels deep
//     (<xdgBase>/<identifier>/<name>) that lacks a registry entry is adopted
//     with a structural-default Entry (AbsPath only).
//  3. Any registered entry with a stale AnchorOverride (the override no longer
//     names a live Repos member) has the override cleared.
//
// xdgBase is typically paths.WorktreesDir(). Pass an empty string to skip
// the adopt-live-dirs step (prune-only mode).
//
// Callers hold workspace knowledge (WorktreeBases, legacy roots) and pass
// xdgBase explicitly — worktreeregistry must not import workspace.
func Reconcile(xdgBase string) error {
	dir := registryDir()
	files, _ := os.ReadDir(dir) // nil slice when dir doesn't exist yet — that's fine

	// Step 1 & 3: walk existing entries — prune missing paths, heal stale anchors.
	registered := make(map[string]struct{}) // AbsPath → present
	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".json" {
			continue
		}
		if strings.HasSuffix(f.Name(), ".json.tmp") {
			continue
		}
		id := strings.TrimSuffix(f.Name(), ".json")
		entry, err := Load(id)
		if err != nil {
			continue
		}

		// Archived entries pass the stat-prune above because Archive re-keys
		// AbsPath to the archive location (which exists on disk); they never
		// live under xdgBase, so the adopt step below ignores them too.
		if _, statErr := os.Stat(entry.AbsPath); os.IsNotExist(statErr) {
			_ = Delete(id)
			continue
		}
		registered[entry.AbsPath] = struct{}{}

		// Skip anchor-heal for archived entries: they are frozen history and
		// their Repos/AnchorOverride must not be rewritten by reconciliation.
		if entry.IsArchived() {
			continue
		}

		if entry.AnchorOverride != "" && !reposContain(entry.Repos, entry.AnchorOverride) {
			entry.AnchorOverride = ""
			_ = Save(entry) // best-effort
		}
	}

	// Step 2: adopt unregistered live dirs under xdgBase (two levels deep).
	if xdgBase == "" {
		return nil
	}
	identifiers, err := os.ReadDir(xdgBase)
	if err != nil {
		return nil // xdgBase not yet created — nothing to adopt
	}
	for _, idDir := range identifiers {
		if !idDir.IsDir() {
			continue
		}
		identifierPath := filepath.Join(xdgBase, idDir.Name())
		worktrees, err := os.ReadDir(identifierPath)
		if err != nil {
			continue
		}
		for _, wt := range worktrees {
			if !wt.IsDir() {
				continue
			}
			wtPath := filepath.Join(identifierPath, wt.Name())
			if _, ok := registered[wtPath]; ok {
				continue
			}
			// Adopt with a minimal structural-default entry.
			entry := &Entry{AbsPath: wtPath}
			_ = Save(entry) // best-effort
		}
	}
	return nil
}
