package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/grovetools/core/internal/daemon/store"
	"github.com/grovetools/core/logging"
	"github.com/grovetools/core/pkg/models"
	"github.com/grovetools/core/pkg/paths"
	"github.com/grovetools/core/pkg/process"
	"github.com/grovetools/core/pkg/sessions"
	"github.com/sirupsen/logrus"
)

// SessionCollector monitors active sessions using fsnotify for instant detection
// and periodic PID verification for liveness checking.
//
// Phase 2 implementation: The daemon becomes the single source of truth for
// "what sessions are running?" This eliminates redundant scanning of
// ~/.grove/hooks/sessions by multiple tools.
type SessionCollector struct {
	interval    time.Duration
	sessionsDir string
	logger      *logrus.Entry

	// In-memory session registry
	mu       sync.RWMutex
	registry map[string]*models.Session
}

// NewSessionCollector creates a new SessionCollector with the specified interval.
// If interval is 0, defaults to 2 seconds for PID verification.
func NewSessionCollector(interval time.Duration) *SessionCollector {
	if interval == 0 {
		interval = 2 * time.Second
	}
	return &SessionCollector{
		interval:    interval,
		sessionsDir: filepath.Join(paths.StateDir(), "hooks", "sessions"),
		logger:      logging.NewLogger("daemon.collector.session"),
		registry:    make(map[string]*models.Session),
	}
}

// Name returns the collector's name.
func (c *SessionCollector) Name() string { return "session" }

// Run starts the session monitoring loop with fsnotify watching.
func (c *SessionCollector) Run(ctx context.Context, st *store.Store, updates chan<- store.Update) error {
	// Ensure sessions directory exists
	if err := os.MkdirAll(c.sessionsDir, 0755); err != nil {
		c.logger.WithError(err).Warn("Failed to ensure sessions directory exists")
		// Continue anyway - directory might be created later
	}

	// Setup fsnotify watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		c.logger.WithError(err).Error("Failed to create fsnotify watcher, falling back to polling only")
		return c.runPollingOnly(ctx, st, updates)
	}
	defer watcher.Close()

	// Watch the sessions directory
	if err := watcher.Add(c.sessionsDir); err != nil {
		c.logger.WithError(err).Warn("Failed to watch sessions directory, falling back to polling only")
		return c.runPollingOnly(ctx, st, updates)
	}

	// Also watch each existing session subdirectory for metadata changes
	c.watchExistingSessionDirs(watcher)

	// Initial scan to populate the registry
	c.fullScan()
	c.emitUpdate(updates)

	// PID verification ticker
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	c.logger.WithField("sessions_dir", c.sessionsDir).Info("Session collector started with fsnotify watching")

	for {
		select {
		case <-ctx.Done():
			return nil

		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			c.handleFsEvent(event, watcher, updates)

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			c.logger.WithError(err).Error("Watcher error")

		case <-ticker.C:
			// Periodic PID verification for active sessions
			if c.verifyPIDs() {
				c.emitUpdate(updates)
			}
		}
	}
}

// runPollingOnly is the fallback when fsnotify is unavailable
func (c *SessionCollector) runPollingOnly(ctx context.Context, st *store.Store, updates chan<- store.Update) error {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	scan := func() {
		liveSessions, err := sessions.DiscoverLiveSessions()
		if err != nil {
			return
		}

		updates <- store.Update{
			Type:    store.UpdateSessions,
			Source:  "session",
			Payload: liveSessions,
		}
	}

	scan()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			scan()
		}
	}
}

// watchExistingSessionDirs adds watchers for all existing session subdirectories
func (c *SessionCollector) watchExistingSessionDirs(watcher *fsnotify.Watcher) {
	entries, err := os.ReadDir(c.sessionsDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			sessionDir := filepath.Join(c.sessionsDir, entry.Name())
			if err := watcher.Add(sessionDir); err != nil {
				c.logger.WithError(err).WithField("dir", sessionDir).Debug("Failed to watch session directory")
			}
		}
	}
}

// handleFsEvent processes filesystem events and updates the registry
func (c *SessionCollector) handleFsEvent(event fsnotify.Event, watcher *fsnotify.Watcher, updates chan<- store.Update) {
	// Ignore events on the sessions directory itself
	if event.Name == c.sessionsDir {
		return
	}

	// Parse the path to get session info
	relPath, err := filepath.Rel(c.sessionsDir, event.Name)
	if err != nil {
		return
	}

	parts := strings.Split(relPath, string(filepath.Separator))
	if len(parts) == 0 {
		return
	}

	dirName := parts[0]

	// Handle directory creation (new session)
	if event.Has(fsnotify.Create) {
		sessionDir := filepath.Join(c.sessionsDir, dirName)
		info, err := os.Stat(sessionDir)
		if err == nil && info.IsDir() {
			// New session directory - watch it for file changes
			if err := watcher.Add(sessionDir); err != nil {
				c.logger.WithError(err).WithField("dir", sessionDir).Debug("Failed to watch new session directory")
			}
			// Try to load the session (it may not have all files yet)
			c.loadSession(dirName)
			c.emitUpdate(updates)
		}
	}

	// Handle file creation/modification within session directory
	if (event.Has(fsnotify.Create) || event.Has(fsnotify.Write)) && len(parts) > 1 {
		filename := parts[1]
		// Reload on metadata.json or pid.lock changes
		if filename == "metadata.json" || filename == "pid.lock" {
			c.loadSession(dirName)
			c.emitUpdate(updates)
		}
	}

	// Handle directory removal
	if event.Has(fsnotify.Remove) && len(parts) == 1 {
		c.mu.Lock()
		delete(c.registry, dirName)
		c.mu.Unlock()
		c.emitUpdate(updates)
	}
}

// fullScan reads all sessions from the filesystem
func (c *SessionCollector) fullScan() {
	entries, err := os.ReadDir(c.sessionsDir)
	if err != nil {
		c.logger.WithError(err).Debug("Failed to read sessions directory")
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			c.loadSession(entry.Name())
		}
	}
}

// loadSession reads a session from disk and updates the registry
func (c *SessionCollector) loadSession(dirName string) {
	sessionDir := filepath.Join(c.sessionsDir, dirName)
	pidFile := filepath.Join(sessionDir, "pid.lock")
	metadataFile := filepath.Join(sessionDir, "metadata.json")

	// Read PID
	pidContent, err := os.ReadFile(pidFile)
	if err != nil {
		return // Session not ready yet
	}

	var pid int
	if _, err := parseIntFromBytes(pidContent, &pid); err != nil {
		return
	}

	// Read metadata
	metadataContent, err := os.ReadFile(metadataFile)
	if err != nil {
		return // Session not ready yet
	}

	var metadata sessions.SessionMetadata
	if err := parseMetadata(metadataContent, &metadata); err != nil {
		return
	}

	// Determine effective IDs
	sessionID := metadata.SessionID
	claudeSessionID := metadata.ClaudeSessionID
	if claudeSessionID == "" {
		claudeSessionID = dirName
	}
	if sessionID == "" {
		sessionID = dirName
	}

	// Check liveness
	isAlive := process.IsProcessAlive(pid)
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
		JobTitle:         metadata.JobTitle,
		PlanName:         metadata.PlanName,
		JobFilePath:      metadata.JobFilePath,
		Provider:         metadata.Provider,
	}

	c.mu.Lock()
	// Use dirName as key since that's how we track in the filesystem
	c.registry[dirName] = session
	c.mu.Unlock()
}

// verifyPIDs checks if active sessions are still running
// Returns true if any session status changed
func (c *SessionCollector) verifyPIDs() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	changed := false
	for dirName, session := range c.registry {
		// Only check sessions we think are active
		if session.Status == "running" || session.Status == "idle" || session.Status == "pending_user" {
			if !process.IsProcessAlive(session.PID) {
				c.logger.WithFields(logrus.Fields{
					"session_id": session.ID,
					"pid":        session.PID,
				}).Debug("Session process died")

				session.Status = "interrupted"
				now := time.Now()
				session.EndedAt = &now
				session.LastActivity = now
				changed = true

				// Optionally clean up the session directory for dead processes
				// In Phase 5 (Lifecycle), this will trigger completion logic
				go c.cleanupDeadSession(dirName, session)
			}
		}
	}
	return changed
}

// cleanupDeadSession handles cleanup for sessions with dead processes
func (c *SessionCollector) cleanupDeadSession(dirName string, session *models.Session) {
	sessionDir := filepath.Join(c.sessionsDir, dirName)

	// For non-flow sessions (claude_session without job file), clean up immediately
	if session.JobFilePath == "" && session.Type != "interactive_agent" && session.Type != "agent" {
		// Wait a moment for any final file writes
		time.Sleep(2 * time.Second)
		os.RemoveAll(sessionDir)
		return
	}

	// For flow jobs, the cleanup is handled by the discovery/completion logic
	// Just log for now - Phase 5 will add auto-completion
}

// emitUpdate sends the current registry state to the update channel
func (c *SessionCollector) emitUpdate(updates chan<- store.Update) {
	c.mu.RLock()
	sessions := make([]*models.Session, 0, len(c.registry))
	for _, s := range c.registry {
		// Return a copy to prevent external mutation
		sCopy := *s
		sessions = append(sessions, &sCopy)
	}
	c.mu.RUnlock()

	updates <- store.Update{
		Type:    store.UpdateSessions,
		Source:  "session",
		Scanned: len(sessions),
		Payload: sessions,
	}
}

// GetSessions returns a copy of the current session list (for direct access)
func (c *SessionCollector) GetSessions() []*models.Session {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]*models.Session, 0, len(c.registry))
	for _, s := range c.registry {
		sCopy := *s
		result = append(result, &sCopy)
	}
	return result
}

// GetSession returns a specific session by ID
func (c *SessionCollector) GetSession(id string) *models.Session {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, s := range c.registry {
		if s.ID == id {
			sCopy := *s
			return &sCopy
		}
	}
	return nil
}

// parseIntFromBytes parses a PID from byte content using fmt.Sscanf
func parseIntFromBytes(content []byte, result *int) (int, error) {
	s := strings.TrimSpace(string(content))
	_, err := fmt.Sscanf(s, "%d", result)
	return *result, err
}

// parseMetadata unmarshals JSON metadata
func parseMetadata(content []byte, result *sessions.SessionMetadata) error {
	return json.Unmarshal(content, result)
}
