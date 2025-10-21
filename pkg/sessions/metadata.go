package sessions

import "time"

// SessionMetadata is the data stored on disk to track a live session.
type SessionMetadata struct {
	SessionID        string    `json:"session_id"`
	ClaudeSessionID  string    `json:"claude_session_id,omitempty"` // For Claude provider (or native agent ID)
	Provider         string    `json:"provider"`                    // "claude" or "codex"
	PID              int       `json:"pid"`
	Repo             string    `json:"repo,omitempty"`
	Branch           string    `json:"branch,omitempty"`
	WorkingDirectory string    `json:"working_directory"`
	User             string    `json:"user"`
	StartedAt        time.Time `json:"started_at"`
	TranscriptPath   string    `json:"transcript_path,omitempty"`
	JobTitle         string    `json:"job_title,omitempty"`
	PlanName         string    `json:"plan_name,omitempty"`
	JobFilePath      string    `json:"job_file_path,omitempty"`
}
