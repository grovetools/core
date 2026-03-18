// Package daemon provides a client interface for interacting with the Grove Daemon (groved).
// It implements a transparent fallback pattern: if the daemon is running, use RPC;
// if not, fall back to direct library calls.
package daemon

import (
	"context"
	"time"

	"github.com/grovetools/core/pkg/models"
	"github.com/grovetools/core/pkg/workspace"
)

// SessionIntent represents the intent to start a session, registered before agent launch.
// This enables race-free session tracking by pre-registering before the agent process exists.
type SessionIntent struct {
	JobID       string `json:"job_id"`
	Provider    string `json:"provider"`     // "claude", "codex", "opencode"
	JobFilePath string `json:"job_file_path"` // Path to the job markdown file
	PlanName    string `json:"plan_name"`
	Title       string `json:"title"`
	WorkDir     string `json:"work_dir"`
}

// SessionConfirmation contains the data needed to confirm a session after agent startup.
// This links the pre-registered intent with the actual running process.
type SessionConfirmation struct {
	JobID          string `json:"job_id"`           // Matches the intent's JobID
	NativeID       string `json:"native_id"`        // Agent's native session ID (e.g., Claude's UUID)
	PID            int    `json:"pid"`              // Process ID of the running agent
	TranscriptPath string `json:"transcript_path"`  // Path to the agent's transcript file
}

// RunningConfig holds the active configuration intervals being used by the daemon.
type RunningConfig struct {
	GitInterval       time.Duration `json:"git_interval"`
	SessionInterval   time.Duration `json:"session_interval"`
	WorkspaceInterval time.Duration `json:"workspace_interval"`
	PlanInterval      time.Duration `json:"plan_interval"`
	NoteInterval      time.Duration `json:"note_interval"`
	StartedAt         time.Time     `json:"started_at"`
}

// Client defines the interface for interacting with the Grove Daemon.
// Both DaemonClient (RPC) and LocalClient (direct calls) implement this interface.
type Client interface {
	// GetWorkspaces returns all discovered workspaces.
	GetWorkspaces(ctx context.Context) ([]*workspace.WorkspaceNode, error)

	// GetEnrichedWorkspaces returns workspaces with enrichment data.
	GetEnrichedWorkspaces(ctx context.Context, opts *models.EnrichmentOptions) ([]*models.EnrichedWorkspace, error)

	// GetPlanStats returns aggregated plan statistics indexed by workspace path.
	GetPlanStats(ctx context.Context) (map[string]*models.PlanStats, error)

	// GetNoteCounts returns aggregated note counts indexed by workspace name.
	GetNoteCounts(ctx context.Context) (map[string]*models.NoteCounts, error)

	// GetSessions returns active sessions from all sources.
	GetSessions(ctx context.Context) ([]*models.Session, error)

	// GetSession returns a specific session by ID.
	GetSession(ctx context.Context, sessionID string) (*models.Session, error)

	// StreamState subscribes to real-time state updates from the daemon.
	// Returns a channel that receives updates and a function to stop the stream.
	// For LocalClient, this returns an error since streaming is only available via daemon.
	StreamState(ctx context.Context) (<-chan StateUpdate, error)

	// GetConfig returns the running configuration of the daemon.
	// For LocalClient, this returns an error since config is only available via daemon.
	GetConfig(ctx context.Context) (*RunningConfig, error)

	// SetFocus tells the daemon which workspaces to prioritize for scanning.
	// For LocalClient, this is a no-op since there's no daemon to notify.
	SetFocus(ctx context.Context, paths []string) error

	// Refresh triggers a re-scan of workspaces and enrichment data.
	// For LocalClient, this is a no-op since there's no daemon cache to refresh.
	// For RemoteClient, this signals the daemon to immediately re-discover workspaces.
	Refresh(ctx context.Context) error

	// IsRunning returns true if the daemon is available and responding.
	IsRunning() bool

	// Close cleans up any resources used by the client.
	Close() error

	// GetNoteIndex returns the daemon's cached note index, optionally filtered by workspace.
	// Returns nil, nil when the daemon is unavailable (graceful degradation for TUI fallback).
	GetNoteIndex(ctx context.Context, workspace string) ([]*models.NoteIndexEntry, error)

	// NotifyNoteEvent sends a note mutation event to the daemon for incremental count updates.
	// This is fire-and-forget from the caller's perspective.
	NotifyNoteEvent(ctx context.Context, event models.NoteEvent) error

	// --- Session Lifecycle Management ---
	// These methods enable race-free session tracking by allowing:
	// 1. Pre-registration of intent before agent launch (flow)
	// 2. Confirmation with actual PID after agent starts (hooks)
	// 3. Status updates during agent execution (hooks)
	// 4. Session end notification (hooks/flow)

	// RegisterSessionIntent pre-registers a session before the agent is launched.
	// This eliminates PID race conditions by establishing the session record first.
	// For LocalClient, this writes to the filesystem registry as a fallback.
	RegisterSessionIntent(ctx context.Context, intent SessionIntent) error

	// ConfirmSession links a pre-registered intent with the actual running agent.
	// Called by hooks after the agent process has started and its PID is known.
	// For LocalClient, this updates the filesystem registry.
	ConfirmSession(ctx context.Context, confirmation SessionConfirmation) error

	// UpdateSessionStatus updates the status of an active session.
	// Valid statuses: "running", "idle", "pending_user"
	// For LocalClient, this updates the filesystem registry.
	UpdateSessionStatus(ctx context.Context, jobID string, status string) error

	// EndSession marks a session as complete or interrupted.
	// Valid outcomes: "completed", "interrupted", "failed"
	// For LocalClient, this updates the filesystem registry and may trigger cleanup.
	EndSession(ctx context.Context, jobID string, outcome string) error

	// --- Job Management ---
	// These methods enable submitting and managing jobs via the daemon's JobRunner.

	// SubmitJob submits a job to the daemon for execution.
	// Returns the created JobInfo with assigned ID and status.
	SubmitJob(ctx context.Context, req models.JobSubmitRequest) (*models.JobInfo, error)

	// CancelJob cancels a running or queued job.
	CancelJob(ctx context.Context, jobID string) error

	// GetJob returns the current state of a specific job.
	GetJob(ctx context.Context, jobID string) (*models.JobInfo, error)

	// ListJobs returns jobs matching the given filter.
	ListJobs(ctx context.Context, filter models.JobFilter) ([]*models.JobInfo, error)

	// --- Log Streaming ---
	// These methods enable streaming and fetching job logs via the daemon's LogStreamer.

	// StreamJobLogs subscribes to real-time log output for a specific job.
	// Returns a channel that receives log and status events. Closed when the job completes or context is cancelled.
	StreamJobLogs(ctx context.Context, jobID string) (<-chan models.JobStreamEvent, error)

	// GetJobLogs returns the historical log content for a completed or running job.
	GetJobLogs(ctx context.Context, jobID string) ([]models.LogLine, error)
}

// StateUpdate represents an update pushed from the daemon to subscribers.
type StateUpdate struct {
	Workspaces []*models.EnrichedWorkspace `json:"workspaces,omitempty"`
	Sessions   []*models.Session           `json:"sessions,omitempty"`
	UpdateType string                      `json:"update_type"`           // "full", "workspace", "session", "enrichment", "config_reload", "skill_sync"
	Source     string                      `json:"source,omitempty"`      // Which collector sent this update (e.g., "git", "workspace", "session", "plan", "note", "config", "skills")
	Scanned    int                         `json:"scanned,omitempty"`     // Number of items actually scanned (for focused updates)
	ConfigFile string                      `json:"config_file,omitempty"` // The config file that changed (for "config_reload" events)
	Payload    interface{}                 `json:"payload,omitempty"`     // Generic payload for events like skill_sync
}
