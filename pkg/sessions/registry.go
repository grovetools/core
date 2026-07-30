package sessions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/grovetools/core/pkg/paths"
	"github.com/grovetools/core/pkg/process"
)

// Registry defines the interface for managing live session tracking.
type Registry interface {
	Register(metadata SessionMetadata) error
	IsAlive(sessionID string) (bool, error)
	Find(jobID string) (*SessionMetadata, error)
}

// FileSystemRegistry implements Registry using the filesystem at ~/.grove/hooks/sessions/
type FileSystemRegistry struct {
	baseDir string
}

func NewFileSystemRegistry() (*FileSystemRegistry, error) {
	return NewFileSystemRegistryAt(paths.StateDir())
}

// NewFileSystemRegistryAt is NewFileSystemRegistry rooted at an explicit
// state directory instead of paths.StateDir(). Used by tests and by
// callers (health.Cleaner) that carry a state-dir override so a sweep
// can be exercised without touching the real registry.
func NewFileSystemRegistryAt(stateDir string) (*FileSystemRegistry, error) {
	baseDir := filepath.Join(stateDir, "hooks", "sessions")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
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

	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		return fmt.Errorf("failed to create session directory: %w", err)
	}

	// Write pid.lock
	pidFile := filepath.Join(sessionDir, "pid.lock")
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", metadata.PID)), 0o644); err != nil { //nolint:gosec // pid file is not sensitive
		return fmt.Errorf("failed to write pid.lock: %w", err)
	}

	// Write metadata.json
	metadataFile := filepath.Join(sessionDir, "metadata.json")
	metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}
	if err := os.WriteFile(metadataFile, metadataJSON, 0o644); err != nil { //nolint:gosec // session metadata is not sensitive
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
	return process.IsProcessAlive(pid), nil
}

// UpdateStatus updates the status field in the session's metadata.json file.
// This ensures crash recovery can restore the correct status (e.g., "idle").
func (r *FileSystemRegistry) UpdateStatus(sessionID, status string) error {
	if sessionID == "" {
		return nil
	}
	sessionDir := filepath.Join(r.baseDir, sessionID)
	metadataFile := filepath.Join(sessionDir, "metadata.json")

	// Read existing metadata
	content, err := os.ReadFile(metadataFile)
	if err != nil {
		return nil // Best-effort: file may not exist yet
	}

	var metadata SessionMetadata
	if err := json.Unmarshal(content, &metadata); err != nil {
		return nil // Best-effort
	}

	metadata.Status = status

	updated, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return nil
	}

	return os.WriteFile(metadataFile, updated, 0o644) //nolint:gosec // session metadata is not sensitive
}

// UpdateFields applies a partial update to a session's metadata.json.
// The updater function receives the current metadata and mutates it in place.
func (r *FileSystemRegistry) UpdateFields(sessionID string, updater func(*SessionMetadata)) error {
	if sessionID == "" {
		return nil
	}
	sessionDir := filepath.Join(r.baseDir, sessionID)
	metadataFile := filepath.Join(sessionDir, "metadata.json")

	content, err := os.ReadFile(metadataFile)
	if err != nil {
		return nil
	}

	var metadata SessionMetadata
	if err := json.Unmarshal(content, &metadata); err != nil {
		return nil
	}

	updater(&metadata)

	updated, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return nil
	}

	return os.WriteFile(metadataFile, updated, 0o644) //nolint:gosec // session metadata
}

// RemovePIDLock removes the pid.lock for a session directory while
// preserving the rest of the dir (transcripts, metadata). After removal,
// RecoverSessions will skip the session as no longer live, so a previously
// persisted terminal status in metadata.json wins over the default
// "running" assumption for alive PIDs.
func (r *FileSystemRegistry) RemovePIDLock(sessionID string) error {
	if sessionID == "" {
		return nil
	}
	pidFile := filepath.Join(r.baseDir, sessionID, "pid.lock")
	if err := os.Remove(pidFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove pid.lock: %w", err)
	}
	return nil
}

// RemoveRecoveryFiles drops a session's crash-recovery state — today the
// pid.lock — while preserving metadata.json. This is what a liveness sweep must
// call when it decides a session's process is gone.
//
// metadata.json is the only index binding a Flow job to its native session and
// transcript, and it is consumed long after the process exits (completion
// evidence, transcript resolution, archival). Deleting it on a dead-PID reading
// makes an irreversible decision from an admittedly unreliable signal: the
// pid.lock PID can be stale through process forking, and an interactive agent
// outlives the launcher whose PID was recorded. The index must therefore
// survive both the process and a wrong guess about the process.
//
// Once pid.lock is gone RecoverSessions skips the session as no longer live, so
// dropping it is sufficient to stop the record from resurrecting as running.
func (r *FileSystemRegistry) RemoveRecoveryFiles(sessionID string) error {
	return r.RemovePIDLock(sessionID)
}

// Purge deletes a session's entire registry directory, metadata.json included.
// It destroys the job→transcript index, so it belongs only to a deliberate GC
// with a retention policy (see PurgeStaleSessions) or to an explicit operator
// action — never to a liveness sweep.
func (r *FileSystemRegistry) Purge(sessionID string) error {
	if sessionID == "" {
		return nil
	}
	sessionDir := filepath.Join(r.baseDir, sessionID)

	// Remove the directory and its contents
	if err := os.RemoveAll(sessionDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove session directory: %w", err)
	}
	return nil
}

// Find searches for a session by Grove job ID in the SessionMetadata.
func (r *FileSystemRegistry) Find(jobID string) (*SessionMetadata, error) {
	// List all session directories
	entries, err := os.ReadDir(r.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no sessions found")
		}
		return nil, fmt.Errorf("failed to read sessions directory: %w", err)
	}

	// Search through all session metadata files
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		metadataFile := filepath.Join(r.baseDir, entry.Name(), "metadata.json")
		metadataBytes, err := os.ReadFile(metadataFile)
		if err != nil {
			continue // Skip sessions without metadata
		}

		var metadata SessionMetadata
		if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
			continue // Skip invalid metadata
		}

		// Match by session ID, job ID, Claude session ID, or directory name
		if metadata.SessionID == jobID || metadata.JobID == jobID || metadata.ClaudeSessionID == jobID || entry.Name() == jobID {
			return &metadata, nil
		}
	}

	return nil, fmt.Errorf("no session found for job ID: %s", jobID)
}
