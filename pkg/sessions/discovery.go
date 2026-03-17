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
			// Clean up dead session recovery files
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
		}

		sessions = append(sessions, session)
	}

	return sessions, nil
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

