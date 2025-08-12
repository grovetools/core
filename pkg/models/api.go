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
