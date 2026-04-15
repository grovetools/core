// Package daemon provides a client interface for interacting with the Grove Daemon (groved).
// It implements a transparent fallback pattern: if the daemon is running, use RPC;
// if not, fall back to direct library calls.
package daemon

import (
	"context"
	"time"

	"github.com/grovetools/core/pkg/env"
	"github.com/grovetools/core/pkg/models"
	"github.com/grovetools/core/pkg/workspace"
)

// SessionIntent represents the intent to start a session, registered before agent launch.
// This enables race-free session tracking by pre-registering before the agent process exists.
type SessionIntent struct {
	JobID       string `json:"job_id"`
	Provider    string `json:"provider"`      // "claude", "codex", "opencode"
	JobFilePath string `json:"job_file_path"` // Path to the job markdown file
	PlanName    string `json:"plan_name"`
	Title       string `json:"title"`
	WorkDir     string `json:"work_dir"`

	// Channel & Autonomous support (optional, set when job has claw features)
	Channels   []string                 `json:"channels,omitempty"`
	Autonomous *models.AutonomousConfig `json:"autonomous,omitempty"`
	TmuxTarget string                   `json:"tmux_target,omitempty"`
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

	// StreamWorkspaceHUD subscribes to per-workspace HUD updates for the
	// given path. The daemon aggregates git/plan/cx/hooks/notebook state
	// and emits a debounced snapshot whenever something relevant changes.
	// For LocalClient, this returns a friendly error (requires daemon).
	StreamWorkspaceHUD(ctx context.Context, path string) (<-chan models.WorkspaceHUD, error)

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

	// KillSession terminates a tracked agent session by sending SIGTERM to its
	// PID and removing the filesystem registry entry. The daemon owns the kill
	// path so it can clean up its in-memory store and any background workers
	// (autonomous pinger, channel manager, etc.) atomically.
	//
	// LocalClient returns an error since killing requires the daemon — callers
	// (e.g., the hooks browse TUI) may fall back to an in-process syscall path
	// when the daemon is unreachable.
	KillSession(ctx context.Context, sessionID string) error

	// --- Channel & Autonomous Management ---

	// UpdateSessionChannels updates the active channels for a session.
	UpdateSessionChannels(ctx context.Context, jobID string, channels []string) error

	// UpdateSessionAutonomous updates the autonomous config for a session.
	UpdateSessionAutonomous(ctx context.Context, jobID string, config *models.AutonomousConfig) error

	// UpdateSessionTmuxTarget updates the tmux target for a session (after detach/attach).
	UpdateSessionTmuxTarget(ctx context.Context, jobID string, target string) error

	// SendChannelMessage sends a message via an external channel (e.g., Signal).
	SendChannelMessage(ctx context.Context, req models.ChannelSendRequest) (*models.ChannelSendResponse, error)

	// GetChannelStatus returns the status of the channel system.
	GetChannelStatus(ctx context.Context) (*models.ChannelStatusResponse, error)

	// --- Environment Management ---

	// EnvUp requests the daemon to spin up an environment for a workspace.
	EnvUp(ctx context.Context, req env.EnvRequest) (*env.EnvResponse, error)

	// EnvDown requests the daemon to tear down an environment for a workspace.
	EnvDown(ctx context.Context, req env.EnvRequest) (*env.EnvResponse, error)

	// EnvStatus returns the current status of an environment for a worktree.
	EnvStatus(ctx context.Context, worktree string) (*env.EnvResponse, error)

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

	// --- Agent Input/Interrupt ---
	// These methods allow sending input to and interrupting interactive agent sessions
	// running in tmux panes. The daemon resolves the tmux target from its session store.

	// SendSessionInput sends input text to an interactive agent session.
	// The daemon handles vim-mode detection and tmux key sending.
	SendSessionInput(ctx context.Context, sessionID string, input string) error

	// SendSessionInterrupt sends Ctrl+C to interrupt an interactive agent session.
	SendSessionInterrupt(ctx context.Context, sessionID string) error

	// --- Nav Bindings Management ---
	// These methods enable reading and writing nav key bindings via the daemon.
	// The daemon is the source of truth; LocalClient falls back to direct file I/O.

	// GetNavBindings returns the current nav binding state.
	GetNavBindings(ctx context.Context) (*models.NavSessionsFile, error)

	// GetNavConfig returns the static nav configuration (group prefixes, etc.)
	// loaded from the grove config files. This is the source of truth for
	// group prefix transitions in non-nav clients.
	GetNavConfig(ctx context.Context) (*models.NavConfig, error)

	// UpdateNavGroup updates the session state for a single group.
	UpdateNavGroup(ctx context.Context, group string, state models.NavGroupState) error

	// UpdateNavLockedKeys updates the global locked keys list.
	UpdateNavLockedKeys(ctx context.Context, keys []string) error

	// SetNavLastAccessedGroup updates the last-accessed group field in the nav binding state.
	SetNavLastAccessedGroup(ctx context.Context, group string) error

	// --- Log Streaming ---
	// These methods enable streaming and fetching job logs via the daemon's LogStreamer.

	// StreamJobLogs subscribes to real-time log output for a specific job.
	// Returns a channel that receives log and status events. Closed when the job completes or context is cancelled.
	StreamJobLogs(ctx context.Context, jobID string) (<-chan models.JobStreamEvent, error)

	// GetJobLogs returns the historical log content for a completed or running job.
	GetJobLogs(ctx context.Context, jobID string) ([]models.LogLine, error)

	// --- Memory Search ---
	// These methods expose the memory SQLite store (hybrid BM25 + vector search)
	// over the daemon. The daemon owns the store connection and Gemini embedder;
	// LocalClient returns an error instructing callers to start groved.

	// SearchMemory runs a hybrid search against the daemon's memory store.
	SearchMemory(ctx context.Context, req models.MemorySearchRequest) ([]models.MemorySearchResult, error)

	// GetMemoryCoverage returns documentation coverage for a target path.
	GetMemoryCoverage(ctx context.Context, req models.MemoryCoverageRequest) (*models.MemoryCoverageReport, error)

	// GetMemoryStatus returns database stats (size, document/chunk counts, doctype distribution).
	GetMemoryStatus(ctx context.Context) (*models.MemoryStatusResponse, error)

	// --- Native Agent Pane Relay ---

	// IsTerminalConnected returns true if a groveterm instance is connected to the
	// daemon via SSE. Used by flow executors to decide between the groveterm provider
	// (native panes) and the legacy tmux provider.
	IsTerminalConnected(ctx context.Context) (bool, error)

	// SpawnAgentPane requests groveterm to spawn a native agent pane via the daemon relay.
	SpawnAgentPane(ctx context.Context, req SpawnAgentRequest) error

	// SendAgentInput relays input text to a native agent pane in groveterm.
	SendAgentInput(ctx context.Context, jobID string, input string) error

	// CaptureAgentPane requests a screen capture from a native agent pane.
	// Blocks until groveterm responds or the request times out.
	CaptureAgentPane(ctx context.Context, jobID string) (string, error)

	// SubmitAgentCaptureResponse sends the captured screen text back to
	// the daemon to unblock a pending CaptureAgentPane request.
	SubmitAgentCaptureResponse(ctx context.Context, jobID string, text string) error

	// --- Daemon PTY Management ---

	// CreatePTY requests the daemon to spawn a new PTY session.
	CreatePTY(ctx context.Context, req PTYCreateRequest) (*PTYSessionInfo, error)

	// ListPTYs returns metadata for all active daemon PTY sessions.
	ListPTYs(ctx context.Context) ([]PTYSessionInfo, error)

	// KillPTY terminates a daemon PTY session by ID.
	KillPTY(ctx context.Context, id string) error

	// GetPTYAttachURL returns the WebSocket URL for attaching to a daemon PTY.
	GetPTYAttachURL(id string) string
}

// SpawnAgentRequest contains the parameters for spawning a native agent pane.
type SpawnAgentRequest struct {
	JobID     string            `json:"job_id"`
	PlanName  string            `json:"plan_name"`
	JobTitle  string            `json:"job_title"`
	Command   string            `json:"command"`
	Args      []string          `json:"args"`
	WorkDir   string            `json:"work_dir"`
	Env       map[string]string `json:"env,omitempty"`
	AutoSplit bool              `json:"auto_split"`
}

// PTYCreateRequest holds the parameters for creating a daemon PTY session.
type PTYCreateRequest struct {
	CWD       string            `json:"cwd"`
	Env       []string          `json:"env,omitempty"`
	Workspace string            `json:"workspace,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
	Rows      uint16            `json:"rows,omitempty"`
	Cols      uint16            `json:"cols,omitempty"`
	Origin    string            `json:"origin,omitempty"`
	PanelID   string            `json:"panel_id,omitempty"`
	Label     string            `json:"label,omitempty"`
	SessionID string            `json:"session_id,omitempty"`
	CreatedBy string            `json:"created_by,omitempty"`
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
}

// PTYSessionInfo is the client-side representation of a daemon PTY session.
type PTYSessionInfo struct {
	ID                string            `json:"id"`
	Workspace         string            `json:"workspace"`
	CWD               string            `json:"cwd"`
	Labels            map[string]string `json:"labels,omitempty"`
	PID               int               `json:"pid"`
	StartedAt         string            `json:"started_at"`
	AttachedClients   int               `json:"attached_clients"`
	Origin            string            `json:"origin,omitempty"`
	PanelID           string            `json:"panel_id,omitempty"`
	Label             string            `json:"label,omitempty"`
	SessionID         string            `json:"session_id,omitempty"`
	CreatedBy         string            `json:"created_by,omitempty"`
	ForegroundProcess string            `json:"foreground_process,omitempty"`
}

// StateUpdate represents an update pushed from the daemon to subscribers.
type StateUpdate struct {
	Workspaces      []*models.EnrichedWorkspace `json:"workspaces,omitempty"`
	WorkspaceDeltas []*models.WorkspaceDelta    `json:"workspace_deltas,omitempty"`
	Sessions        []*models.Session           `json:"sessions,omitempty"`
	UpdateType      string                      `json:"update_type"`           // "full", "workspace", "workspaces_delta", "session", "enrichment", "config_reload", "skill_sync"
	Source          string                      `json:"source,omitempty"`      // Which collector sent this update (e.g., "git", "workspace", "session", "plan", "note", "config", "skills")
	Scanned         int                         `json:"scanned,omitempty"`     // Number of items actually scanned (for focused updates)
	ConfigFile      string                      `json:"config_file,omitempty"` // The config file that changed (for "config_reload" events)
	Payload         interface{}                 `json:"payload,omitempty"`     // Generic payload for events like skill_sync
}
