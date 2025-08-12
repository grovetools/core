package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// EventType represents the type of event
type EventType string

const (
	EventTypeNotification EventType = "notification"
	EventTypeToolUse      EventType = "tool_use"
	EventTypeMessage      EventType = "message"
	EventTypeError        EventType = "error"
	EventTypeStateChange  EventType = "state_change"
	EventTypeTranscript   EventType = "transcript"
	// Additional event types from events package
	EventStop         EventType = "stop"
	EventPreToolUse   EventType = "pretooluse"
	EventPostToolUse  EventType = "posttooluse"
	EventSubagentStop EventType = "subagentstop"
)

// Event represents any system event
type Event struct {
	ID              string          `json:"id" db:"id"`
	SessionID       string          `json:"session_id" db:"session_id"`
	Type            EventType       `json:"type" db:"type"`
	Timestamp       time.Time       `json:"timestamp" db:"timestamp"`
	Source          string          `json:"source" db:"source"`
	Data            json.RawMessage `json:"data" db:"data"`
	CorrelationID   string          `json:"correlation_id,omitempty" db:"correlation_id"`
	TranscriptIndex int             `json:"transcript_index,omitempty" db:"transcript_index"`
	CreatedAt       time.Time       `json:"created_at" db:"created_at"`
	// Additional fields from events package
	TranscriptPath string        `json:"transcript_path,omitempty" db:"-"`
	TranscriptUUID string        `json:"transcript_uuid,omitempty" db:"transcript_uuid"`
	ParentUUID     string        `json:"parent_uuid,omitempty" db:"parent_uuid"`
	Metadata       EventMetadata `json:"metadata" db:"-"`
}

// EventMetadata contains metadata about an event
type EventMetadata struct {
	Version       string            `json:"version"`
	Source        string            `json:"source"` // hook name
	Environment   map[string]string `json:"environment,omitempty"`
	CorrelationID string            `json:"correlation_id,omitempty"`
}

// Value implements driver.Valuer for EventMetadata
func (em EventMetadata) Value() (driver.Value, error) {
	b, err := json.Marshal(em)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal EventMetadata: %w", err)
	}
	return b, nil
}

// Scan implements sql.Scanner for EventMetadata
func (em *EventMetadata) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	var b []byte
	switch v := value.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return fmt.Errorf("cannot scan %T into EventMetadata", value)
	}

	if err := json.Unmarshal(b, em); err != nil {
		return fmt.Errorf("failed to unmarshal EventMetadata: %w", err)
	}
	return nil
}

// Notification represents a user-facing notification
type Notification struct {
	ID                     string                 `json:"id" db:"id"`
	SessionID              string                 `json:"session_id" db:"session_id"`
	Type                   string                 `json:"type" db:"type"`
	Title                  string                 `json:"title" db:"title"`
	Body                   string                 `json:"body" db:"body"`
	Level                  string                 `json:"level" db:"level"` // info|warning|error
	Timestamp              time.Time              `json:"timestamp" db:"timestamp"`
	SystemNotificationSent bool                   `json:"system_notification_sent" db:"system_notification_sent"`
	Metadata               map[string]interface{} `json:"metadata,omitempty" db:"metadata"`
	Actions                []NotificationAction   `json:"actions,omitempty" db:"-"`
}

// NotificationAction represents an action button in a notification
type NotificationAction struct {
	Label   string `json:"label"`
	Action  string `json:"action"`
	Primary bool   `json:"primary"`
}

// NotificationInput represents input for creating a notification
type NotificationInput struct {
	Type     string                 `json:"type"`
	Title    string                 `json:"title"`
	Body     string                 `json:"body"`
	Metadata map[string]interface{} `json:"metadata"`
}

// NotificationPreferences stores user notification settings
type NotificationPreferences map[EventType]bool

// ShouldNotify checks if a notification should be sent for an event type
func (np NotificationPreferences) ShouldNotify(eventType EventType) bool {
	if enabled, exists := np[eventType]; exists {
		return enabled
	}
	return true // Default to enabled
}

// GetDataAsMap returns the Data field as a map[string]any
func (e *Event) GetDataAsMap() (map[string]any, error) {
	if len(e.Data) == 0 {
		return make(map[string]any), nil
	}

	var data map[string]any
	if err := json.Unmarshal(e.Data, &data); err != nil {
		return nil, err
	}
	return data, nil
}

// SetDataFromMap sets the Data field from a map[string]any
func (e *Event) SetDataFromMap(data map[string]any) error {
	if data == nil {
		e.Data = json.RawMessage("{}")
		return nil
	}

	bytes, err := json.Marshal(data)
	if err != nil {
		return err
	}
	e.Data = bytes
	return nil
}
