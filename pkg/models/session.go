package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// Session represents a complete Claude session or a grove-flow job
type Session struct {
	// Core fields
	ID               string     `json:"id" db:"id"`
	Type             string     `json:"type" db:"type"` // "claude_session" or "oneshot_job"
	PID              int        `json:"pid" db:"pid"`
	Repo             string     `json:"repo" db:"repo"`
	Branch           string     `json:"branch" db:"branch"`
	TmuxKey          string     `json:"tmux_key" db:"tmux_key"`
	WorkingDirectory string     `json:"working_directory" db:"working_directory"`
	User             string     `json:"user" db:"user"`
	Status           string     `json:"status" db:"status"` // running|stopped|completed|failed|idle|error
	StartedAt        time.Time  `json:"started_at" db:"started_at"`
	EndedAt          *time.Time `json:"ended_at,omitempty" db:"ended_at"`
	LastActivity     time.Time  `json:"last_activity" db:"last_activity"`

	// Grove Flow Job specific fields
	PlanName      string `json:"plan_name,omitempty" db:"plan_name"`
	PlanDirectory string `json:"plan_directory,omitempty" db:"plan_directory"`
	JobTitle      string `json:"job_title,omitempty" db:"job_title"`
	JobFilePath   string `json:"job_file_path,omitempty" db:"job_file_path"`

	// ClaudeSessionID stores the original UUID of a claude_code session when it's
	// managed by a grove-flow interactive_agent job.
	ClaudeSessionID string `json:"claude_session_id,omitempty" db:"claude_session_id"`
	Provider        string `json:"provider,omitempty" db:"provider"`

	// Test mode
	IsTest    bool `json:"is_test" db:"is_test"`
	IsDeleted bool `json:"-" db:"is_deleted"` // Keep as internal field

	// JSON-marshaled fields
	ToolStats      *ToolStatistics `json:"tool_stats" db:"tool_stats"`
	SessionSummary *Summary        `json:"session_summary" db:"session_summary"` // Use the structured Summary type

	// Related data (populated on demand, not in main table)
	Tools         []ToolExecution      `json:"tools" db:"-"`
	Notifications []ClaudeNotification `json:"notifications" db:"-"`
	Subagents     []SubagentExecution  `json:"subagents" db:"-"`
}

// Summary represents the overall session summary including AI analysis
type Summary struct {
	// Summary statistics
	TotalTools         int                    `json:"total_tools"`
	FilesModified      int                    `json:"files_modified"`
	CommandsExecuted   int                    `json:"commands_executed"`
	ErrorsCount        int                    `json:"errors_count"`
	NotificationsSent  int                    `json:"notifications_sent"`
	PerformanceMetrics map[string]interface{} `json:"performance_metrics,omitempty"`
	Recommendations    []string               `json:"recommendations,omitempty"`

	// AI-generated summary
	AISummary    *AISummary    `json:"ai_summary,omitempty"`
	MessageStats *MessageStats `json:"message_stats,omitempty"`
}

// AISummary represents AI-generated session summary
type AISummary struct {
	// Content fields
	CurrentActivity string      `json:"current_activity,omitempty"`
	History         []Milestone `json:"history,omitempty"` // Renamed from Milestones

	// Metadata
	LastUpdated         time.Time `json:"last_updated"`
	UpdateCount         int       `json:"update_count"`
	NextUpdateAtMessage int       `json:"next_update_at_message"`

	// Legacy fields for backward compatibility
	Title            string    `json:"title,omitempty"`
	Description      string    `json:"description,omitempty"`
	KeyPoints        []string  `json:"key_points,omitempty"`
	TechnicalDetails []string  `json:"technical_details,omitempty"`
	Outcomes         []string  `json:"outcomes,omitempty"`
	GeneratedAt      time.Time `json:"generated_at,omitempty"`
	GeneratedBy      string    `json:"generated_by,omitempty"`
	Error            string    `json:"error,omitempty"`
}

// MessageStats tracks message extraction statistics
type MessageStats struct {
	TotalMessages     int       `json:"total_messages"`
	UserMessages      int       `json:"user_messages"`
	AssistantMessages int       `json:"assistant_messages"`
	LastExtraction    time.Time `json:"last_extraction,omitempty"`
}

// Milestone represents a key accomplishment
type Milestone struct {
	Timestamp time.Time `json:"timestamp"`
	Summary   string    `json:"summary"`        // Changed from Description to Summary to match frontend
	Type      string    `json:"type,omitempty"` // feature|fix|refactor|test|docs
}

// Metadata holds arbitrary session metadata
type Metadata map[string]interface{}

// Value implements driver.Valuer for database storage
func (m Metadata) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}
	return json.Marshal(m)
}

// Scan implements sql.Scanner for database retrieval
func (m *Metadata) Scan(value interface{}) error {
	if value == nil {
		*m = make(Metadata)
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("cannot scan %T into Metadata", value)
	}
	return json.Unmarshal(b, m)
}

// Value implements driver.Valuer for ToolStatistics
func (ts *ToolStatistics) Value() (driver.Value, error) {
	if ts == nil {
		return nil, nil
	}
	return json.Marshal(ts)
}

// Scan implements sql.Scanner for ToolStatistics
func (ts *ToolStatistics) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("cannot scan %T into ToolStatistics", value)
	}
	return json.Unmarshal(b, ts)
}

// ToolStats aggregates tool usage statistics
type ToolStats struct {
	TotalExecutions     int                  `json:"total_executions"`
	SuccessfulCount     int                  `json:"successful_count"`
	FailedCount         int                  `json:"failed_count"`
	TotalDuration       time.Duration        `json:"total_duration"`
	ByTool              map[string]*ToolStat `json:"by_tool"`
	MostUsed            string               `json:"most_used"`
	LastExecution       time.Time            `json:"last_execution"`
	BashCommands        int                  `json:"bash_commands"`
	FileModifications   int                  `json:"file_modifications"`
	FileReads           int                  `json:"file_reads"`
	SearchOperations    int                  `json:"search_operations"`
	AverageToolDuration float64              `json:"avg_tool_duration_ms"`
}

// ToolStat represents statistics for a specific tool
type ToolStat struct {
	Count        int           `json:"count"`
	SuccessCount int           `json:"success_count"`
	TotalTime    time.Duration `json:"total_time"`
	AvgTime      time.Duration `json:"avg_time"`
	LastUsed     time.Time     `json:"last_used"`
}

// Value implements driver.Valuer for ToolStats
func (ts *ToolStats) Value() (driver.Value, error) {
	if ts == nil {
		return nil, nil
	}
	return json.Marshal(ts)
}

// Scan implements sql.Scanner for ToolStats
func (ts *ToolStats) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("cannot scan %T into ToolStats", value)
	}
	return json.Unmarshal(b, ts)
}

// Summary's Value/Scan methods for database storage
func (s *Summary) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	return json.Marshal(s)
}

func (s *Summary) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("cannot scan %T into Summary", value)
	}
	return json.Unmarshal(b, s)
}
