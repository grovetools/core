package sessions

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/grovetools/core/pkg/paths"
	"github.com/grovetools/core/pkg/process"
)

// ErrAttemptNotFound identifies an exact point-lookup miss. Callers may create
// that attempt, but must not treat decode or identity mismatches as misses.
var ErrAttemptNotFound = errors.New("session attempt not found")

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

// Register creates or upgrades the tracking files for one attempt. New-format
// records are keyed by AttemptID, so intent, confirmation, and hook enrichment
// all replace the same metadata.json. Empty AttemptID retains the legacy
// native-ID-first key rule for migration reads and flow-less sessions.
func (r *FileSystemRegistry) Register(metadata SessionMetadata) error {
	sessionDirName := metadata.AttemptID
	if sessionDirName == "" {
		sessionDirName = metadata.ClaudeSessionID
		if sessionDirName == "" {
			sessionDirName = metadata.SessionID
		}
	}
	if !validRegistryKey(sessionDirName) {
		return fmt.Errorf("invalid empty or non-local session key %q", sessionDirName)
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

func validRegistryKey(key string) bool {
	return key != "" && key != "." && key != ".." && !filepath.IsAbs(key) && filepath.Base(key) == key
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

// FindAttempt performs a point lookup for a new-format attempt. It never
// falls back to broad aliases: a miss or metadata mismatch must not bind a
// stale prior execution of the same reusable job ID.
func (r *FileSystemRegistry) FindAttempt(attemptID string) (*SessionMetadata, error) {
	if !validRegistryKey(attemptID) {
		return nil, fmt.Errorf("invalid attempt ID %q", attemptID)
	}
	content, err := os.ReadFile(filepath.Join(r.baseDir, attemptID, "metadata.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrAttemptNotFound, attemptID)
		}
		return nil, fmt.Errorf("failed to read attempt metadata: %w", err)
	}
	var metadata SessionMetadata
	if err := json.Unmarshal(content, &metadata); err != nil {
		return nil, fmt.Errorf("failed to decode attempt metadata: %w", err)
	}
	if metadata.AttemptID != attemptID {
		return nil, fmt.Errorf("attempt metadata mismatch: path %q contains %q", attemptID, metadata.AttemptID)
	}
	return &metadata, nil
}

// UpdateStatusForAttempt updates exactly (jobID, attemptID). A mismatch is an
// error rather than a broad alias fallback, preventing late lifecycle events
// from mutating a newer execution of a reused job ID.
func (r *FileSystemRegistry) UpdateStatusForAttempt(jobID, attemptID, status string) error {
	metadata, err := r.FindAttempt(attemptID)
	if err != nil {
		return err
	}
	if metadata.JobID != jobID && metadata.SessionID != jobID {
		return fmt.Errorf("attempt %q belongs to job %q, not %q", attemptID, metadata.JobID, jobID)
	}
	return r.UpdateStatus(attemptID, status)
}

// UpdateStatus updates the status field in the session's metadata.json file.
// This ensures crash recovery can restore the correct status (e.g., "idle").
// The key is a directory key (AttemptID for new records, legacy key otherwise).
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
	_, err := r.removeRecoveryFiles(sessionID)
	return err
}

func (r *FileSystemRegistry) removeRecoveryFiles(sessionID string) (bool, error) {
	if sessionID == "" {
		return false, nil
	}
	pidFile := filepath.Join(r.baseDir, sessionID, "pid.lock")
	if err := os.Remove(pidFile); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to remove pid.lock: %w", err)
	}
	return true, nil
}

// RemoveRecoveryFilesForAttempt clears recovery state only when the exact
// attempt record belongs to jobID. New-format cleanup must use this rather
// than sweeping every attempt of a reusable job ID.
func (r *FileSystemRegistry) RemoveRecoveryFilesForAttempt(jobID, attemptID string) error {
	_, err := r.removeRecoveryFilesForAttempt(jobID, attemptID, nil)
	return err
}

// RemoveRecoveryFilesForAttemptInScope is the scoped-daemon variant.
func (r *FileSystemRegistry) RemoveRecoveryFilesForAttemptInScope(jobID, attemptID, scope string) error {
	_, err := r.removeRecoveryFilesForAttempt(jobID, attemptID, &scope)
	return err
}

// removeRecoveryFilesForAttempt reports whether recovery state was actually
// removed. The public methods retain their historical error-only API, while
// recovery classification uses the transition bit to avoid re-emitting on a
// record whose pid.lock was already cleared by an earlier sweep.
func (r *FileSystemRegistry) removeRecoveryFilesForAttempt(jobID, attemptID string, scope *string) (bool, error) {
	metadata, err := r.FindAttempt(attemptID)
	if err != nil {
		return false, err
	}
	if scope != nil && metadata.Scope != *scope {
		return false, fmt.Errorf("attempt %q belongs to scope %q, not %q", attemptID, metadata.Scope, *scope)
	}
	if metadata.JobID != jobID && metadata.SessionID != jobID {
		return false, fmt.Errorf("attempt %q belongs to job %q, not %q", attemptID, metadata.JobID, jobID)
	}
	return r.removeRecoveryFiles(attemptID)
}

// RemoveRecoveryFilesForJob clears crash-recovery state from every registry
// directory belonging to the same Flow job/native-session aliases. Legacy
// daemonless launches can leave both a job-ID intent directory and a
// native-ID confirmation directory; clearing only one lets the other record
// resurrect the same attempt after restart.
//
// The returned count is the number of matching directories whose recovery
// state is now absent. Metadata is always retained for transcript/history
// lookups. Directory-name matching is retained for legacy or damaged records
// whose metadata cannot be decoded.
func (r *FileSystemRegistry) RemoveRecoveryFilesForJob(jobID, nativeID string) (int, error) {
	return r.removeRecoveryFilesForJob(jobID, nativeID, nil)
}

// RemoveRecoveryFilesForJobInScope is the scoped-daemon variant. Alias keys
// such as short Flow job IDs can repeat in different ecosystem worktrees, so a
// scoped cleanup must not mutate a matching record owned by another daemon.
func (r *FileSystemRegistry) RemoveRecoveryFilesForJobInScope(jobID, nativeID, scope string) (int, error) {
	return r.removeRecoveryFilesForJob(jobID, nativeID, &scope)
}

func (r *FileSystemRegistry) removeRecoveryFilesForJob(jobID, nativeID string, scope *string) (int, error) {
	keys := make(map[string]struct{}, 2)
	if jobID != "" {
		keys[jobID] = struct{}{}
	}
	if nativeID != "" {
		keys[nativeID] = struct{}{}
	}
	if len(keys) == 0 {
		return 0, nil
	}

	entries, err := os.ReadDir(r.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to read sessions directory: %w", err)
	}

	removed := 0
	var errs []error
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		_, matches := keys[entry.Name()]
		metadataPath := filepath.Join(r.baseDir, entry.Name(), "metadata.json")
		metadataDecoded := false
		var metadata SessionMetadata
		if content, readErr := os.ReadFile(metadataPath); readErr == nil && json.Unmarshal(content, &metadata) == nil {
			metadataDecoded = true
			for _, alias := range []string{metadata.AttemptID, metadata.SessionID, metadata.JobID, metadata.ClaudeSessionID} {
				if _, ok := keys[alias]; ok && alias != "" {
					matches = true
					break
				}
			}
		}
		if scope != nil && (!metadataDecoded || metadata.Scope != *scope) {
			continue
		}
		if !matches {
			continue
		}
		changed, err := r.removeRecoveryFiles(entry.Name())
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", entry.Name(), err))
			continue
		}
		if changed {
			removed++
		}
	}
	return removed, errors.Join(errs...)
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

		// Match by attempt ID, session ID, job ID, native session ID, or
		// directory name. This broad scan is retained only for legacy callers;
		// new-format callers with AttemptID must use FindAttempt.
		if metadata.AttemptID == jobID || metadata.SessionID == jobID || metadata.JobID == jobID || metadata.ClaudeSessionID == jobID || entry.Name() == jobID {
			return &metadata, nil
		}
	}

	return nil, fmt.Errorf("no session found for job ID: %s", jobID)
}
