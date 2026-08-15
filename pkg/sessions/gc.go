package sessions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/grovetools/core/pkg/paths"
	"github.com/grovetools/core/pkg/process"
)

// DefaultSessionRetention is how long a reaped session's metadata.json is kept
// before the GC may drop it. The record is small and is the only index binding
// a Flow job to its native session and transcript, so retention is generous:
// consumers (completion evidence, transcript resolution, archival, `aglogs`)
// read it well after the process exits.
const DefaultSessionRetention = 30 * 24 * time.Hour

// RecoveryCorroborator is the daemon-owned half of dead-pid.lock cleanup. It
// returns true only when the daemon roster says the matching session is absent
// or terminal. GC supplies both the legacy directory key and decoded aliases
// so callers can match either registry shape.
type RecoveryCorroborator func(sessionDir string, metadata SessionMetadata) bool

// PurgeStaleSessions keeps the historical compatibility behavior: any
// pid.lock, including one containing a dead PID, vetoes collection. Daemon
// callers that can corroborate registry state against their roster should use
// PurgeStaleSessionsWithCorroboration.
func PurgeStaleSessions(retention time.Duration) (int, error) {
	return purgeStaleSessions(retention, nil)
}

// PurgeStaleSessionsWithCorroboration removes a dead pid.lock only when the
// daemon roster independently says the corresponding row is absent or
// terminal, then applies the normal metadata retention policy. A live PID is
// always retained, and an unreadable lock is left for startup recovery to
// classify; GC never judges either case from filesystem evidence alone.
func PurgeStaleSessionsWithCorroboration(retention time.Duration, corroborate RecoveryCorroborator) (int, error) {
	return purgeStaleSessions(retention, corroborate)
}

func purgeStaleSessions(retention time.Duration, corroborate RecoveryCorroborator) (int, error) {
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
		oldEnough := !newestModTime(sessionDir).After(cutoff)

		pidPath := filepath.Join(sessionDir, "pid.lock")
		pidContent, lockErr := os.ReadFile(pidPath)
		switch {
		case lockErr == nil:
			var pid int
			if _, err := fmt.Sscanf(string(pidContent), "%d", &pid); err != nil {
				continue // Startup recovery owns unreadable-lock classification.
			}
			if process.IsProcessAlive(pid) || corroborate == nil {
				continue
			}
			metadataContent, err := os.ReadFile(filepath.Join(sessionDir, "metadata.json"))
			if err != nil {
				continue
			}
			var metadata SessionMetadata
			if err := json.Unmarshal(metadataContent, &metadata); err != nil || !corroborate(entry.Name(), metadata) {
				continue
			}
			if err := registry.RemoveRecoveryFiles(entry.Name()); err != nil {
				continue
			}
		case os.IsNotExist(lockErr):
			// Already reaped; normal retention applies.
		default:
			continue
		}

		// Compute age before removing pid.lock: unlinking it refreshes the
		// directory mtime but should not restart metadata retention.
		if !oldEnough {
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
