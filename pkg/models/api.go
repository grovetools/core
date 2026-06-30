package models

import (
	"fmt"
	"time"
)

// SessionCreateRequest represents the request to create a new session
type SessionCreateRequest struct {
	SessionID        string `json:"session_id"`
	PID              int    `json:"pid"`
	Repo             string `json:"repo"`
	Branch           string `json:"branch"`
	TmuxKey          string `json:"tmux_key"`
	WorkingDirectory string `json:"working_directory"`
	User             string `json:"user"`
	StartedAt        string `json:"started_at"`
	Status           string `json:"status"`
}

// SessionUpdateRequest represents the request to update a session
type SessionUpdateRequest struct {
	Status          string          `json:"status,omitempty"`
	LastActivity    string          `json:"last_activity,omitempty"`
	ToolStats       *ToolStatistics `json:"tool_stats,omitempty"`
	CurrentActivity string          `json:"current_activity,omitempty"`
}

// ToolLogRequest represents the request to log tool usage
type ToolLogRequest struct {
	Timestamp     string         `json:"timestamp"`
	ToolName      string         `json:"tool_name"`
	Parameters    map[string]any `json:"parameters"`
	Approved      bool           `json:"approved"`
	BlockedReason string         `json:"blocked_reason,omitempty"`
}

// ToolUpdateRequest represents the request to update tool completion
type ToolUpdateRequest struct {
	CompletedAt   string             `json:"completed_at"`
	DurationMs    int64              `json:"duration_ms"`
	Success       bool               `json:"success"`
	ResultSummary *ToolResultSummary `json:"result_summary,omitempty"`
	Error         string             `json:"error,omitempty"`
}

// NotificationRequest represents the request to log a notification
type NotificationRequest struct {
	Timestamp              string `json:"timestamp"`
	Type                   string `json:"type"`
	Level                  string `json:"level"`
	Message                string `json:"message"`
	SystemNotificationSent bool   `json:"system_notification_sent"`
}

// SessionCompleteRequest represents the request to mark a session complete
type SessionCompleteRequest struct {
	EndedAt         string         `json:"ended_at"`
	DurationSeconds int            `json:"duration_seconds"`
	ExitStatus      string         `json:"exit_status"` // completed|terminated|error
	SessionSummary  map[string]any `json:"session_summary"`
	Recommendations []string       `json:"recommendations,omitempty"`
}

// SubagentRequest represents the request to track a subagent
type SubagentRequest struct {
	SubagentID      string         `json:"subagent_id"`
	ParentSessionID string         `json:"parent_session_id"`
	TaskDescription string         `json:"task_description"`
	TaskType        string         `json:"task_type"`
	CompletedAt     string         `json:"completed_at"`
	DurationSeconds int            `json:"duration_seconds"`
	Status          string         `json:"status"`
	Result          SubagentResult `json:"result"`
}

// SubagentResult contains subagent execution results
type SubagentResult struct {
	ToolCalls         int      `json:"tool_calls"`
	FilesRead         int      `json:"files_read"`
	FilesModified     int      `json:"files_modified"`
	SuccessIndicators []string `json:"success_indicators"`
	PerformanceScore  float64  `json:"performance_score"`
}

// SessionSummary contains overall session analytics (for internal use)
type SessionSummary struct {
	TotalTools         int                `json:"total_tools"`
	FilesModified      int                `json:"files_modified"`
	CommandsExecuted   int                `json:"commands_executed"`
	ErrorsCount        int                `json:"errors_count"`
	NotificationsSent  int                `json:"notifications_sent"`
	PerformanceMetrics PerformanceMetrics `json:"performance_metrics"`
	Recommendations    []string           `json:"recommendations"`
}

// PerformanceMetrics contains performance analytics
type PerformanceMetrics struct {
	AvgToolDurationMs      float64 `json:"avg_tool_duration_ms"`
	TotalFileReads         int     `json:"total_file_reads"`
	ModificationEfficiency float64 `json:"modification_efficiency"`
}

// Helper method to convert ClaudeSession (old) to Session (new)
func SessionFromClaudeSession(cs interface{}) *Session {
	// This will be implemented when we start migrating code
	// For now, it's a placeholder
	return nil
}

// --- Channel & Autonomous Configuration ---

// AutonomousConfig holds settings for autonomous idle pinging on a session.
type AutonomousConfig struct {
	Enabled     bool   `json:"enabled" yaml:"enabled"`
	IdleMinutes int    `json:"idle_minutes" yaml:"idle_minutes"`
	Prompt      string `json:"prompt,omitempty" yaml:"prompt,omitempty"`
}

// ChannelSendRequest represents a request to send a message via a channel.
type ChannelSendRequest struct {
	Channel   string `json:"channel,omitempty"` // Target channel: "signal" (default), "ha"
	JobID     string `json:"job_id"`
	JobTitle  string `json:"job_title,omitempty"` // Explicit title for cross-daemon tagging
	Recipient string `json:"recipient,omitempty"` // Empty = use LastSender or broadcast
	GroupID   string `json:"group_id,omitempty"`  // Signal group ID (base64); when set, sends to group
	Message   string `json:"message"`
}

// ChannelSendResponse represents the result of sending a channel message.
type ChannelSendResponse struct {
	Timestamp int64  `json:"timestamp"`
	Status    string `json:"status"`
}

// InboundRecord captures a single inbound routing decision for observability.
type InboundRecord struct {
	Timestamp time.Time `json:"timestamp"`
	Sender    string    `json:"sender"`
	Strategy  string    `json:"strategy"` // quote, tag, single_active, dropped
	TargetJob string    `json:"target_job,omitempty"`
	Delivered bool      `json:"delivered"`
	Error     string    `json:"error,omitempty"`
}

// ChannelStatusResponse represents the status of the channel system.
type ChannelStatusResponse struct {
	SignalCLIRunning     bool            `json:"signal_cli_running"`
	ActiveRoutes         int             `json:"active_routes"`
	RefCount             int             `json:"ref_count"`
	SignalRestartCount   int             `json:"signal_restart_count"`
	SignalLastRestart    *time.Time      `json:"signal_last_restart,omitempty"`
	SignalIsAlive        bool            `json:"signal_is_alive"`
	LastInboundTimestamp *time.Time      `json:"last_inbound_timestamp,omitempty"`
	RecentInbound        []InboundRecord `json:"recent_inbound,omitempty"`
}

// ChannelCleanupResponse represents the result of a channel cleanup operation.
type ChannelCleanupResponse struct {
	Purged int `json:"purged"`
}

// SessionChannelsRequest represents a request to update session channels.
type SessionChannelsRequest struct {
	Channels     []string `json:"channels"`
	SignalTarget string   `json:"signal_target,omitempty"`
}

// SessionAutonomousRequest represents a request to update session autonomous config.
type SessionAutonomousRequest struct {
	Enabled     bool   `json:"enabled"`
	IdleMinutes int    `json:"idle_minutes,omitempty"`
	Prompt      string `json:"prompt,omitempty"`
}

// SessionPatchRequest represents a partial update to session metadata.
type SessionPatchRequest struct {
	TmuxTarget      string `json:"tmux_target,omitempty"`
	LastSender      string `json:"last_sender,omitempty"`
	LastSenderGroup string `json:"last_sender_group,omitempty"`
}

// --- Job Runner API Types ---

// JobSubmitRequest represents a request to submit a job to the daemon.
type JobSubmitRequest struct {
	PlanDir     string            `json:"plan_dir"`
	JobFile     string            `json:"job_file"`
	Priority    int               `json:"priority,omitempty"`
	Timeout     string            `json:"timeout,omitempty"` // e.g., "30m"
	Env         map[string]string `json:"env,omitempty"`
	AgentTarget string            `json:"agent_target,omitempty"` // "native" or "tmux" — resolved by caller
}

// JobSubmitResponse represents the response to a job submission.
// It includes the JobInfo plus any warnings about unknown/unsupported fields.
type JobSubmitResponse struct {
	*JobInfo `json:",inline"`
	Warnings []string `json:"warnings,omitempty"` // Warnings about ignored fields or unsupported features
}

// JobFilter represents query parameters for listing jobs.
type JobFilter struct {
	Status string `json:"status,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

// JobType represents the type of a job (e.g., chat, oneshot, interactive_agent).
type JobType string

// JobInfo represents the current state of a job in the daemon.
type JobInfo struct {
	ID          string            `json:"id"`
	Title       string            `json:"title,omitempty"`
	Type        JobType           `json:"type,omitempty"`
	PlanDir     string            `json:"plan_dir"`
	PlanName    string            `json:"plan_name,omitempty"`
	JobFile     string            `json:"job_file"`
	WorkDir     string            `json:"work_dir,omitempty"`
	Repo        string            `json:"repo,omitempty"`
	Branch      string            `json:"branch,omitempty"`
	Status      string            `json:"status"` // queued, running, completed, failed, cancelled, idle, pending_user
	Priority    int               `json:"priority,omitempty"`
	TimeoutStr  string            `json:"timeout,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	AgentTarget string            `json:"agent_target,omitempty"` // "native" or "tmux"
	Channels    []string          `json:"channels,omitempty"`
	SubmittedAt time.Time         `json:"submitted_at"`
	StartedAt   *time.Time        `json:"started_at,omitempty"`
	CompletedAt *time.Time        `json:"completed_at,omitempty"`
	Error       string            `json:"error,omitempty"`
	LogFilePath string            `json:"log_file_path,omitempty"`
	PID         int               `json:"pid,omitempty"` // PHASE 2: Process ID for adoption on daemon restart
}

// PlanRunOptions represents options for running an entire plan.
type PlanRunOptions struct {
	Mode     string   `json:"mode"` // "next", "all", "specific"
	JobFiles []string `json:"job_files,omitempty"`
	Parallel int      `json:"parallel,omitempty"`
	AutoRun  bool     `json:"autorun,omitempty"`
}

// LogStreamOptions configures the daemon's aggregated workspace log stream.
type LogStreamOptions struct {
	Scope     string `json:"scope"`     // "workspace", "ecosystem", "all", "system"
	Workspace string `json:"workspace"` // Path of the active workspace context
	Level     string `json:"level"`     // "debug", "info", "warn", "error"
	System    bool   `json:"system"`    // Whether to interleave system logs
	Replay    int    `json:"replay"`    // Number of historical lines to replay
}

// LogStreamLine represents a single workspace log entry in the aggregated stream.
type LogStreamLine struct {
	Workspace     string `json:"workspace"`
	WorkspacePath string `json:"workspace_path"`
	Line          string `json:"line"`
}

// LogLine represents a single streamed log entry.
type LogLine struct {
	Line      string    `json:"line"`
	Timestamp time.Time `json:"timestamp"`
}

// JobStreamEvent encapsulates events sent over the job log SSE stream.
// Event types: "log" for log lines, "status" for job status changes.
type JobStreamEvent struct {
	Event  string   `json:"event"`            // "log" or "status"
	Line   *LogLine `json:"line,omitempty"`   // Present when Event == "log"
	Status string   `json:"status,omitempty"` // Present when Event == "status"
	Error  string   `json:"error,omitempty"`  // Present when Event == "status" and job failed
}

// SystemInfo represents the daemon's system information (version, commit, build date).
type SystemInfo struct {
	Version   string `json:"version"`    // e.g., "main-abc123def" or "v1.2.3"
	Commit    string `json:"commit"`     // Git commit hash (short SHA)
	BuildDate string `json:"build_date"` // ISO 8601 timestamp (e.g., "2026-06-11T15:20:30Z")
	// Scope is the daemon's owning scope (ecosystem-boundary path). Empty ==
	// unscoped/global. Clients (HUD/inspector) render a short label or "global"
	// to attribute which daemon they're paired to.
	Scope string `json:"scope,omitempty"`
	// UpgradeAvailable is true when the daemon's on-disk binary has changed
	// since this process started — i.e. a rebuild is waiting and `groved
	// upgrade` would swap to it. Clients (e.g. the treemux HUD) surface this
	// as a staleness marker. It compares the running daemon against its own
	// executable on disk, NOT against the client's commit (daemon and client
	// are separate repos with unrelated commit hashes, so a commit comparison
	// is always "stale").
	UpgradeAvailable bool `json:"upgrade_available"`
}

// Helper method to parse time strings from API requests
func ParseTimeString(timeStr string) (time.Time, error) {
	// Try common time formats
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, timeStr); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse time string: %s", timeStr)
}
