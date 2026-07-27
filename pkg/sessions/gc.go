package sessions

import (
	"os"
	"path/filepath"
	"time"

	"github.com/grovetools/core/pkg/paths"
)

// DefaultSessionRetention is how long a reaped session's metadata.json is kept
// before the GC may drop it. The record is small and is the only index binding
// a Flow job to its native session and transcript, so retention is generous:
// consumers (completion evidence, transcript resolution, archival, `aglogs`)
// read it well after the process exits.
const DefaultSessionRetention = 30 * 24 * time.Hour

// PurgeStaleSessions deletes registry records that are both not live and older
// than the retention window, and returns how many directories it removed.
//
// This is the ONLY caller of Purge in normal operation. Liveness sweeps call
// RemoveRecoveryFiles instead: "this PID is not alive" is a statement about a
// process, and must never be promoted into "this session never happened".
// A directory is eligible only when it has no pid.lock (nothing claims it is
// live) and nothing in it has been modified within the retention window.
func PurgeStaleSessions(retention time.Duration) (int, error) {
	if retention <= 0 {
		retention = DefaultSessionRetention
	}

	baseDir := filepath.Join(paths.StateDir(), "hooks", "sessions")
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	registry, regErr := NewFileSystemRegistry()
	if regErr != nil {
		return 0, regErr
	}

	cutoff := time.Now().Add(-retention)
	purged := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sessionDir := filepath.Join(baseDir, entry.Name())

		// A pid.lock means some writer still considers this session live.
		// Never purge it here, however stale the PID looks — that judgement
		// belongs to the liveness sweep, which only removes the lock.
		if _, err := os.Stat(filepath.Join(sessionDir, "pid.lock")); err == nil {
			continue
		}

		if newestModTime(sessionDir).After(cutoff) {
			continue
		}

		if err := registry.Purge(entry.Name()); err == nil {
			purged++
		}
	}

	return purged, nil
}

// newestModTime returns the most recent modification time across a session
// directory and its files. A zero time means the directory could not be read,
// which the caller treats as "too old to keep" only if the directory itself is
// also old.
func newestModTime(dir string) time.Time {
	newest := time.Time{}
	if info, err := os.Stat(dir); err == nil {
		newest = info.ModTime()
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return newest
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	return newest
}
