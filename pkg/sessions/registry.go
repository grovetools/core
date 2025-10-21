package sessions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mattsolo1/grove-core/util/pathutil"
)

// Registry defines the interface for managing live session tracking.
type Registry interface {
	Register(metadata SessionMetadata) error
	IsAlive(sessionID string) (bool, error)
}

// FileSystemRegistry implements Registry using the filesystem at ~/.grove/hooks/sessions/
type FileSystemRegistry struct {
	baseDir string
}

func NewFileSystemRegistry() (*FileSystemRegistry, error) {
	baseDir, err := pathutil.Expand("~/.grove/hooks/sessions")
	if err != nil {
		return nil, fmt.Errorf("could not expand session directory path: %w", err)
	}
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create sessions directory: %w", err)
	}
	return &FileSystemRegistry{baseDir: baseDir}, nil
}

// Register creates the tracking files for a live session.
func (r *FileSystemRegistry) Register(metadata SessionMetadata) error {
	// The directory is named after the agent's native session ID (e.g., Claude's UUID, Codex's UUID).
	sessionDirName := metadata.ClaudeSessionID
	if sessionDirName == "" {
		sessionDirName = metadata.SessionID
	}
	sessionDir := filepath.Join(r.baseDir, sessionDirName)

	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return fmt.Errorf("failed to create session directory: %w", err)
	}

	// Write pid.lock
	pidFile := filepath.Join(sessionDir, "pid.lock")
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", metadata.PID)), 0644); err != nil {
		return fmt.Errorf("failed to write pid.lock: %w", err)
	}

	// Write metadata.json
	metadataFile := filepath.Join(sessionDir, "metadata.json")
	metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}
	if err := os.WriteFile(metadataFile, metadataJSON, 0644); err != nil {
		return fmt.Errorf("failed to write metadata.json: %w", err)
	}

	return nil
}

// IsAlive checks if a session with the given ID is still running.
func (r *FileSystemRegistry) IsAlive(sessionID string) (bool, error) {
	sessionDir := filepath.Join(r.baseDir, sessionID)
	pidFile := filepath.Join(sessionDir, "pid.lock")

	// Check if the pid.lock file exists
	pidBytes, err := os.ReadFile(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to read pid.lock: %w", err)
	}

	var pid int
	if _, err := fmt.Sscanf(string(pidBytes), "%d", &pid); err != nil {
		return false, fmt.Errorf("failed to parse PID: %w", err)
	}

	// Check if the process is running
	process, err := os.FindProcess(pid)
	if err != nil {
		return false, nil
	}

	// On Unix systems, sending signal 0 checks if the process exists
	err = process.Signal(os.Signal(nil))
	return err == nil, nil
}
