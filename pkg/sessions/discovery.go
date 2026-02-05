package sessions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/grovetools/core/pkg/models"
	"github.com/grovetools/core/pkg/paths"
	"github.com/grovetools/core/pkg/process"
	"github.com/grovetools/core/util/frontmatter"
)

// DiscoverLiveSessions scans ~/.grove/hooks/sessions/ directory and returns live sessions.
// A session is considered live if its PID is still alive.
// This is a core-only implementation without storage dependencies.
func DiscoverLiveSessions() ([]*models.Session, error) {
	groveSessionsDir := filepath.Join(paths.StateDir(), "hooks", "sessions")

	if _, err := os.Stat(groveSessionsDir); os.IsNotExist(err) {
		return []*models.Session{}, nil
	}

	entries, err := os.ReadDir(groveSessionsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read sessions directory: %w", err)
	}

	var sessions []*models.Session

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

		status := "running"
		var endedAt *time.Time
		lastActivity := metadata.StartedAt

		if !isAlive {
			status = "interrupted"
			now := time.Now()
			endedAt = &now
			lastActivity = now
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
			LastActivity:     lastActivity,
			EndedAt:          endedAt,
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

// DiscoverJobsFromFiles scans directories for job files and returns sessions.
// This uses the lightweight frontmatter parser to avoid importing flow packages.
func DiscoverJobsFromFiles(dirs []string) ([]*models.Session, error) {
	var sessions []*models.Session
	seen := make(map[string]bool)

	for _, dir := range dirs {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}

			// Skip archive directories
			if info.IsDir() {
				name := info.Name()
				if name == "archive" || name == ".archive" ||
					strings.HasPrefix(name, "archive-") || strings.HasPrefix(name, ".archive-") {
					return filepath.SkipDir
				}
				return nil
			}

			if !strings.HasSuffix(info.Name(), ".md") {
				return nil
			}
			if info.Name() == "spec.md" || info.Name() == "README.md" {
				return nil
			}

			if seen[path] {
				return nil
			}

			session := parseJobFileToSession(path)
			if session != nil {
				seen[path] = true
				sessions = append(sessions, session)
			}

			return nil
		})
		if err != nil {
			continue
		}
	}

	return sessions, nil
}

// parseJobFileToSession parses a job markdown file and returns a Session.
func parseJobFileToSession(path string) *models.Session {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	meta, err := frontmatter.Parse(file)
	if err != nil {
		return nil
	}

	// Only process valid job types or active statuses
	isActiveStatus := meta.Status == "running" || meta.Status == "pending_user" || meta.Status == "idle"
	isValidJobType := meta.Type == "chat" || meta.Type == "oneshot" || meta.Type == "agent" ||
		meta.Type == "interactive_agent" || meta.Type == "headless_agent" || meta.Type == "shell"

	if !isValidJobType && !isActiveStatus {
		return nil
	}

	sessionType := meta.Type
	if sessionType == "" && isActiveStatus {
		sessionType = "note"
	}

	// Determine repo and branch from path
	pathParts := strings.Split(path, string(filepath.Separator))
	var repo, branch string
	for i, part := range pathParts {
		if part == "repos" && i+2 < len(pathParts) {
			repo = pathParts[i+1]
			branch = pathParts[i+2]
			break
		} else if part == ".grove-worktrees" && i > 0 && i+1 < len(pathParts) {
			repo = pathParts[i-1]
			branch = pathParts[i+1]
			break
		}
	}

	planName := filepath.Base(filepath.Dir(path))

	// Determine effective status based on lock file and PID
	status := meta.Status
	if status == "running" || status == "pending_user" {
		// Check lock file for non-chat/non-interactive_agent jobs
		if meta.Type != "chat" && meta.Type != "interactive_agent" {
			lockFile := path + ".lock"
			pidContent, err := os.ReadFile(lockFile)
			if err != nil {
				status = "interrupted"
			} else {
				var pid int
				if _, err := fmt.Sscanf(string(pidContent), "%d", &pid); err == nil {
					if !process.IsProcessAlive(pid) {
						status = "interrupted"
					}
				} else {
					status = "interrupted"
				}
			}
		}
	}

	session := &models.Session{
		ID:           meta.ID,
		Type:         sessionType,
		Status:       status,
		Repo:         repo,
		Branch:       branch,
		StartedAt:    meta.StartedAt,
		LastActivity: meta.UpdatedAt,
		PlanName:     planName,
		JobTitle:     meta.Title,
		JobFilePath:  path,
	}

	return session
}

// GetJobStatus reads the current status of a job file.
func GetJobStatus(jobFilePath string) (string, error) {
	file, err := os.Open(jobFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to open job file: %w", err)
	}
	defer file.Close()

	meta, err := frontmatter.Parse(file)
	if err != nil {
		return "", fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	// Terminal states are authoritative
	terminalStates := map[string]bool{
		"completed": true, "failed": true, "interrupted": true,
		"error": true, "abandoned": true, "hold": true, "todo": true, "pending": true,
	}
	if terminalStates[meta.Status] {
		return meta.Status, nil
	}

	// For running/pending_user states, verify with lock file
	if meta.Status == "running" || meta.Status == "pending_user" {
		if meta.Type == "chat" || meta.Type == "interactive_agent" {
			return meta.Status, nil
		}

		lockFile := jobFilePath + ".lock"
		pidContent, err := os.ReadFile(lockFile)
		if err != nil {
			return "interrupted", nil
		}

		var pid int
		if _, err := fmt.Sscanf(string(pidContent), "%d", &pid); err != nil {
			return "interrupted", nil
		}

		if !process.IsProcessAlive(pid) {
			return "interrupted", nil
		}

		return "running", nil
	}

	return meta.Status, nil
}
