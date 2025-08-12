package models

import (
	"time"
)

// TranscriptEntry represents a single entry in the transcript
type TranscriptEntry struct {
	Index     int                    `json:"index"`
	Type      TranscriptEntryType    `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	Content   string                 `json:"content"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	SessionID string                 `json:"session_id,omitempty"`
}

// TranscriptEntryType categorizes transcript entries
type TranscriptEntryType string

const (
	TranscriptTypeUser      TranscriptEntryType = "user"
	TranscriptTypeAssistant TranscriptEntryType = "assistant"
	TranscriptTypeSystem    TranscriptEntryType = "system"
	TranscriptTypeTool      TranscriptEntryType = "tool"
	TranscriptTypeError     TranscriptEntryType = "error"
)

// Message represents a Claude conversation message
type Message struct {
	ID         string          `json:"id" db:"id"`
	SessionID  string          `json:"session_id" db:"session_id"`
	Role       MessageRole     `json:"role" db:"role"`
	Content    string          `json:"content" db:"content"`
	Timestamp  time.Time       `json:"timestamp" db:"timestamp"`
	TokenCount int             `json:"token_count,omitempty" db:"token_count"`
	Metadata   MessageMetadata `json:"metadata,omitempty" db:"metadata"`
}

// MessageRole represents the role of a message sender
type MessageRole string

const (
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
	MessageRoleSystem    MessageRole = "system"
)

// MessageMetadata contains additional message details
type MessageMetadata struct {
	Model           string   `json:"model,omitempty"`
	Temperature     float64  `json:"temperature,omitempty"`
	ToolCalls       []string `json:"tool_calls,omitempty"`
	ParentMessageID string   `json:"parent_message_id,omitempty"`
}

// TranscriptParserConfig configuration
type TranscriptParserConfig struct {
	BufferSize    int           `json:"buffer_size"`
	FlushInterval time.Duration `json:"flush_interval"`
	MaxBatchSize  int           `json:"max_batch_size"`
	ParseWorkers  int           `json:"parse_workers"`
}
