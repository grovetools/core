package worktreeregistry

import (
	"fmt"
	"time"

	"github.com/grovetools/core/util/pathutil"
)

// Archive re-keys the registry entry for a worktree that has been moved from
// oldAbsPath (its live location) to newAbsPath (under
// paths.WorktreeArchiveDir()). It deletes the old ID file, then saves the
// entry with AbsPath=newAbsPath, OriginalPath=oldAbsPath and ArchivedAt=now —
// Save re-derives the registry ID from the new AbsPath, so the entry lands
// under the archive-location key. When no entry exists for oldAbsPath (or it
// is corrupt), a minimal one is synthesized so the archive is still tracked.
//
// Archive only updates the registry; moving the directory itself is the
// caller's job and must happen before (or atomically with) this call so
// Reconcile's stat-prune does not race the re-keyed entry.
func Archive(oldAbsPath, newAbsPath string) error {
	if oldAbsPath == "" || newAbsPath == "" {
		return fmt.Errorf("archive: old and new paths must be non-empty")
	}

	registryMu.Lock()
	defer registryMu.Unlock()

	oldID := pathutil.WorktreeID(oldAbsPath)
	entry, err := Load(oldID)
	if err != nil {
		// Missing or corrupt old entry: synthesize a minimal one so the
		// archived worktree is still represented in the registry.
		entry = &Entry{AbsPath: oldAbsPath}
	}
	if err := Delete(oldID); err != nil {
		return fmt.Errorf("archive: delete old entry %s: %w", oldID, err)
	}

	entry.AbsPath = newAbsPath
	entry.OriginalPath = oldAbsPath
	entry.ArchivedAt = time.Now().UTC()
	return Save(entry)
}
