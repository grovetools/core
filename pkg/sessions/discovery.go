package sessions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

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

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirName := entry.Name()
		sessionDir := filepath.Join(groveSessionsDir, dirName)
		pidFile := filepath.Join(sessionDir, "pid.lock")
		metadataFile := filepath.Join(sessionDir, "metadata.json")

		// Read PID
		pidContent, err := os.ReadFile(pidFile)
		if err != nil {
			continue
		}

		var pid int
		if _, err := fmt.Sscanf(string(pidContent), "%d", &pid); err != nil {
			continue
		}

		// Check if process is alive
		isAlive := process.IsProcessAlive(pid)

		if !isAlive {
			// Clean up dead session recovery files. When filtering by scope, a
			// daemon must only reap records it owns: read the metadata to learn
			// the owning scope and leave records belonging to other scopes (or
			// whose ownership can't be determined) untouched.
			if filterByScope {
				metadataContent, merr := os.ReadFile(metadataFile)
				if merr != nil {
					continue
				}
				var deadMeta SessionMetadata
				if err := json.Unmarshal(metadataContent, &deadMeta); err != nil {
					continue
				}
				if deadMeta.Scope != scope {
					continue
				}
			}
			if registry != nil {
				_ = registry.Unregister(dirName)
			}
			continue
		}

		// Read metadata
		metadataContent, err := os.ReadFile(metadataFile)
		if err != nil {
			continue
		}

		var metadata SessionMetadata
		if err := json.Unmarshal(metadataContent, &metadata); err != nil {
			continue
		}

		// Scope filter: a scoped daemon only seeds sessions it owns.
		if filterByScope && metadata.Scope != scope {
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
		}

		session := &models.Session{
			ID:               sessionID,
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
			PlanName:         metadata.PlanName,
			JobFilePath:      metadata.JobFilePath,
			Provider:         metadata.Provider,
			PtyID:            metadata.PtyID,
		}

		sessions = append(sessions, session)
	}

	return sessions, nil
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
