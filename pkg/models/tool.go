package models

import (
	"encoding/json"
	"time"
)

// Tool represents a tool execution
type Tool struct {
	ID            string          `json:"id" db:"id"`
	SessionID     string          `json:"session_id" db:"session_id"`
	Name          string          `json:"name" db:"name"`
	Status        ToolStatus      `json:"status" db:"status"`
	StartTime     time.Time       `json:"start_time" db:"start_time"`
	EndTime       *time.Time      `json:"end_time,omitempty" db:"end_time"`
	Duration      *time.Duration  `json:"duration,omitempty" db:"duration"`
	Input         json.RawMessage `json:"input,omitempty" db:"input"`
	Output        json.RawMessage `json:"output,omitempty" db:"output"`
	Error         string          `json:"error,omitempty" db:"error"`
	Metadata      ToolMetadata    `json:"metadata,omitempty" db:"metadata"`
	Approved      bool            `json:"approved" db:"approved"`
	BlockedReason string          `json:"blocked_reason,omitempty" db:"blocked_reason"`
	CreatedAt     time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at" db:"updated_at"`
}

// ToolStatus represents the execution status
type ToolStatus string

const (
	ToolStatusPending   ToolStatus = "pending"
	ToolStatusRunning   ToolStatus = "running"
	ToolStatusSuccess   ToolStatus = "success"
	ToolStatusFailed    ToolStatus = "failed"
	ToolStatusCancelled ToolStatus = "cancelled"
	ToolStatusBlocked   ToolStatus = "blocked"
)

// ToolMetadata contains additional tool execution details
type ToolMetadata struct {
	RetryCount    int                    `json:"retry_count,omitempty"`
	ParentToolID  string                 `json:"parent_tool_id,omitempty"`
	Tags          []string               `json:"tags,omitempty"`
	ResourcesUsed map[string]interface{} `json:"resources_used,omitempty"`
	ResultSummary *ToolResultSummary     `json:"result_summary,omitempty"`
}

// ToolResultSummary contains summary of tool execution results
type ToolResultSummary struct {
	ModifiedFiles   []string `json:"modified_files,omitempty"`
	CommandExitCode *int     `json:"command_exit_code,omitempty"`
	OutputSizeBytes int64    `json:"output_size_bytes,omitempty"`
	FilesRead       []string `json:"files_read,omitempty"`
	SearchMatches   int      `json:"search_matches,omitempty"`
	AISummary       string   `json:"ai_summary,omitempty"`
}

// ToolValidation represents pre-execution validation
type ToolValidation struct {
	ToolName    string                 `json:"tool_name"`
	IsValid     bool                   `json:"is_valid"`
	Errors      []string               `json:"errors,omitempty"`
	Warnings    []string               `json:"warnings,omitempty"`
	Suggestions []string               `json:"suggestions,omitempty"`
	Context     map[string]interface{} `json:"context,omitempty"`
}

// ToolHookData represents data passed to tool hooks
type ToolHookData struct {
	SessionID string          `json:"session_id"`
	ToolName  string          `json:"tool_name"`
	HookType  string          `json:"hook_type"` // "pre" or "post"
	Input     json.RawMessage `json:"input,omitempty"`
	Output    json.RawMessage `json:"output,omitempty"`
	Error     string          `json:"error,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
}

// Subagent tracks subagent completions
type Subagent struct {
	ID              string         `json:"id" db:"id"`
	ParentSessionID string         `json:"parent_session_id" db:"parent_session_id"`
	TaskDescription string         `json:"task_description" db:"task_description"`
	TaskType        string         `json:"task_type" db:"task_type"` // search|implementation|debugging|analysis
	StartedAt       time.Time      `json:"started_at" db:"started_at"`
	CompletedAt     time.Time      `json:"completed_at" db:"completed_at"`
	DurationMs      int64          `json:"duration_ms" db:"duration_ms"`
	DurationSeconds int            `json:"duration_seconds" db:"duration_seconds"` // For backward compatibility
	ToolCallCount   int            `json:"tool_call_count" db:"tool_call_count"`
	Success         bool           `json:"success" db:"success"`
	Status          string         `json:"status" db:"status"` // completed|failed|timeout
	Error           string         `json:"error,omitempty" db:"error"`
	Result          map[string]any `json:"result,omitempty" db:"result"`
	ResultSummary   map[string]any `json:"result_summary,omitempty" db:"result_summary"`
}

// ToolExecution represents a single tool execution (simplified version for state package)
type ToolExecution struct {
	ID            string             `json:"id" db:"id"`
	SessionID     string             `json:"session_id" db:"session_id"`
	StartedAt     time.Time          `json:"started_at" db:"started_at"`
	CompletedAt   *time.Time         `json:"completed_at,omitempty" db:"completed_at"`
	ToolName      string             `json:"tool_name" db:"tool_name"`
	Parameters    map[string]any     `json:"parameters" db:"parameters"`
	Approved      bool               `json:"approved" db:"approved"`
	BlockedReason string             `json:"blocked_reason,omitempty" db:"blocked_reason"`
	Success       *bool              `json:"success,omitempty" db:"success"`
	DurationMs    *int64             `json:"duration_ms,omitempty" db:"duration_ms"`
	ResultSummary *ToolResultSummary `json:"result_summary,omitempty" db:"result_summary"`
	Error         string             `json:"error,omitempty" db:"error"`
}

// ToolStatistics tracks tool usage metrics
type ToolStatistics struct {
	TotalCalls          int     `json:"total_calls" db:"total_calls"`
	BashCommands        int     `json:"bash_commands" db:"bash_commands"`
	FileModifications   int     `json:"file_modifications" db:"file_modifications"`
	FileReads           int     `json:"file_reads" db:"file_reads"`
	SearchOperations    int     `json:"search_operations" db:"search_operations"`
	AverageToolDuration float64 `json:"avg_tool_duration_ms" db:"avg_tool_duration_ms"`
}

// SubagentExecution is an alias for Subagent to maintain compatibility
type SubagentExecution = Subagent
