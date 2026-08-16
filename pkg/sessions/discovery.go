package sessions

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/grovetools/core/logging"
	"github.com/grovetools/core/pkg/models"
	"github.com/grovetools/core/pkg/paths"
	"github.com/grovetools/core/pkg/process"
)

// RecoverSessions reads the crash-recovery filesystem registry to find sessions
// that were running before the daemon restarted.
// Dead sessions are cleaned up automatically.
func RecoverSessions() ([]*models.Session, error) {
	return recoverSessions(false, "")
}

// RecoverSessionsForScope behaves like RecoverSessions but returns only the
// sessions whose owning scope equals the given scope, and only cleans up dead
// records it owns. Empty scope == unscoped/global; legacy records without a
// scope field read as unscoped. The daemon uses this to seed its operational
// store so it only ever sees and reaps agents launched under its own scope.
func RecoverSessionsForScope(scope string) ([]*models.Session, error) {
	return recoverSessions(true, scope)
}

func recoverSessions(filterByScope bool, scope string) ([]*models.Session, error) {
	groveSessionsDir := filepath.Join(paths.StateDir(), "hooks", "sessions")

	if _, err := os.Stat(groveSessionsDir); os.IsNotExist(err) {
		return []*models.Session{}, nil
	}

	entries, err := os.ReadDir(groveSessionsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read sessions directory: %w", err)
	}

	var sessions []*models.Session
	registry, _ := NewFileSystemRegistry()
	ulog := logging.NewUnifiedLogger("core.sessions.recovery")

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirName := entry.Name()
		sessionDir := filepath.Join(groveSessionsDir, dirName)
		pidFile := filepath.Join(sessionDir, "pid.lock")
		metadataFile := filepath.Join(sessionDir, "metadata.json")

		// Read metadata before pid.lock so missing and malformed recovery claims
		// can still be classified, and so a scoped daemon never mutates a record
		// it does not own.
		metadataContent, err := os.ReadFile(metadataFile)
		if err != nil {
			continue
		}
		var metadata SessionMetadata
		if err := json.Unmarshal(metadataContent, &metadata); err != nil {
			continue
		}
		if filterByScope && metadata.Scope != scope {
			continue
		}

		pidContent, pidReadErr := os.ReadFile(pidFile)
		var pid int
		_, pidParseErr := fmt.Sscanf(string(pidContent), "%d", &pid)
		if pidReadErr != nil || pidParseErr != nil {
			// An active metadata claim without a readable pid.lock has no live
			// process claim to recover. Mark the exact record terminal so it is
			// retention-eligible rather than silently skipping it forever.
			from := metadata.Status
			changed := false
			if !isTerminalRecoveryStatus(metadata.Status) {
				metadata.Status = "interrupted"
				changed = writeMetadataFile(metadataFile, metadata) == nil
			}
			// A malformed lock must not remain an immortal GC veto. New-format
			// cleanup is exact-attempt; only legacy records sweep aliases.
			removed := removeRecoveryClaim(registry, metadata, dirName, filterByScope, scope)
			observed := "pid.lock missing"
			if pidReadErr == nil {
				observed = "pid.lock unreadable"
			}
			ulog.Info("Classified unrecoverable session registry record").
				Field("event", "session.registry_recovery").
				Field("job_id", metadata.JobID).
				Field("native_id", metadata.ClaudeSessionID).
				Field("observed", observed+"; metadata status="+from).
				Field("concluded", "no live recovery claim; terminal status=interrupted").
				Field("changed", changed || removed > 0).
				StructuredOnly().Log(context.Background())
			continue
		}

		if !process.IsProcessAlive(pid) {
			// New-format cleanup is exact-attempt; only legacy records sweep
			// aliases. metadata.json remains the durable job→transcript index.
			removed := removeRecoveryClaim(registry, metadata, dirName, filterByScope, scope)
			ulog.Info("Classified dead session registry process").
				Field("event", "session.registry_recovery").
				Field("job_id", metadata.JobID).
				Field("native_id", metadata.ClaudeSessionID).
				Field("observed", fmt.Sprintf("pid.lock pid=%d is dead", pid)).
				Field("concluded", "recovery claim must not resurrect").
				Field("changed", removed > 0).
				StructuredOnly().Log(context.Background())
			continue
		}

		sessionID := metadata.SessionID
		claudeSessionID := metadata.ClaudeSessionID
		if claudeSessionID == "" {
			claudeSessionID = dirName
		}

		// Use persisted status if available, default to "running" for alive processes
		status := metadata.Status
		if status == "" {
			status = "running"
			ulog.Info("Defaulted legacy session registry status").
				Field("event", "session.registry_legacy_default").
				Field("registry_dir", dirName).
				Field("job_id", metadata.JobID).
				Field("attempt_id", metadata.AttemptID).
				Field("observed", "metadata status is empty").
				Field("concluded", "legacy alive record defaults to running").
				StructuredOnly().Log(context.Background())
		}

		session := &models.Session{
			ID:               sessionID,
			AttemptID:        metadata.AttemptID,
			Type:             metadata.Type,
			ClaudeSessionID:  claudeSessionID,
			PID:              pid,
			Repo:             metadata.Repo,
			Branch:           metadata.Branch,
			WorkingDirectory: metadata.WorkingDirectory,
			User:             metadata.User,
			Status:           status,
			StartedAt:        metadata.StartedAt,
			LastActivity:     time.Now(),
			IsTest:           false,
			JobTitle:         metadata.JobTitle,
			ParentJobID:      metadata.ParentJobID,
			PlanName:         metadata.PlanName,
			JobFilePath:      metadata.JobFilePath,
			Provider:         metadata.Provider,
			PtyID:            metadata.PtyID,
			// The transcript path is persisted at session confirmation and must
			// survive a daemon restart: without it the daemon's session
			// collector falls back to job-ID transcript resolution, which cannot
			// succeed for agent-owned session dirs, so live token tracking dies
			// for the recovered session after a few wasted corpus scans.
			TranscriptPath: metadata.TranscriptPath,
		}

		sessions = append(sessions, session)
	}

	return sessions, nil
}

func removeRecoveryClaim(registry *FileSystemRegistry, metadata SessionMetadata, dirName string, scoped bool, scope string) int {
	if registry == nil {
		return 0
	}
	jobID := metadata.JobID
	if jobID == "" {
		jobID = metadata.SessionID
	}
	if metadata.AttemptID != "" {
		var err error
		if scoped {
			err = registry.RemoveRecoveryFilesForAttemptInScope(jobID, metadata.AttemptID, scope)
		} else {
			err = registry.RemoveRecoveryFilesForAttempt(jobID, metadata.AttemptID)
		}
		if err == nil {
			return 1
		}
		return 0
	}
	nativeID := metadata.ClaudeSessionID
	if nativeID == "" {
		nativeID = dirName
	}
	if scoped {
		removed, _ := registry.RemoveRecoveryFilesForJobInScope(jobID, nativeID, scope)
		return removed
	}
	removed, _ := registry.RemoveRecoveryFilesForJob(jobID, nativeID)
	return removed
}

func isTerminalRecoveryStatus(status string) bool {
	switch status {
	case "completed", "failed", "error", "interrupted", "stopped", "abandoned", "orphaned", "cancelled", "canceled":
		return true
	default:
		return false
	}
}

func writeMetadataFile(path string, metadata SessionMetadata) error {
	updated, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, updated, 0o644) //nolint:gosec // session metadata is not sensitive
}

// ResolveClaudeSessionDirs returns every directory under ~/.claude/projects/*/
// named after the given Claude session ID. Session artifacts can fragment
// across multiple project-slug directories when the shell cwd changes
// mid-session (e.g. a workflow's runs land under the worktree slug while its
// scripts land under a submodule slug), so callers must consider all matches
// rather than constructing a single path.
func ResolveClaudeSessionDirs(claudeSessionID string) ([]string, error) {
	if claudeSessionID == "" {
		return nil, fmt.Errorf("claude session ID is empty")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve home directory: %w", err)
	}

	matches, err := filepath.Glob(filepath.Join(home, ".claude", "projects", "*", claudeSessionID))
	if err != nil {
		return nil, fmt.Errorf("failed to glob claude session dirs: %w", err)
	}

	var dirs []string
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil || !info.IsDir() {
			continue
		}
		dirs = append(dirs, match)
	}
	sort.Strings(dirs)
	return dirs, nil
}

// DiscoverAll returns sessions recovered from the filesystem crash-recovery registry.
// This is used by LocalClient as a fallback when the daemon is not available.
// The daemon is the single source of truth for live session state; this only returns
// sessions with live PIDs found via crash-recovery scanning.
func DiscoverAll() ([]*models.Session, error) {
	sessions, err := RecoverSessions()
	if err != nil {
		return nil, err
	}

	// Sort by last activity (most recent first)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].LastActivity.After(sessions[j].LastActivity)
	})

	return sessions, nil
}
