package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// ClaudeNotification represents a notification from Claude
type ClaudeNotification struct {
	ID                     string    `json:"id" db:"id"`
	SessionID              string    `json:"session_id" db:"session_id"`
	Timestamp              time.Time `json:"timestamp" db:"timestamp"`
	Type                   string    `json:"type" db:"type"`
	Level                  string    `json:"level" db:"level"` // info|warning|error
	Message                string    `json:"message" db:"message"`
	SystemNotificationSent bool      `json:"system_notification_sent" db:"system_notification_sent"`
}

// SimpleNotification represents a basic notification for external systems
type SimpleNotification struct {
	Title    string   `json:"title"`
	Message  string   `json:"message"`
	Priority string   `json:"priority"`
	Tags     []string `json:"tags,omitempty"`
}

// NotificationSettings represents user notification preferences
type NotificationSettings struct {
	BrowserEnabled bool   `json:"browser_enabled" db:"browser_enabled"`
	NtfyEnabled    bool   `json:"ntfy_enabled" db:"ntfy_enabled"`
	NtfyURL        string `json:"ntfy_url" db:"ntfy_url"`
	NtfyTopic      string `json:"ntfy_topic" db:"ntfy_topic"`
}

// Value implements driver.Valuer for NotificationSettings
func (ns NotificationSettings) Value() (driver.Value, error) {
	return json.Marshal(ns)
}

// Scan implements sql.Scanner for NotificationSettings
func (ns *NotificationSettings) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("cannot scan %T into NotificationSettings", value)
	}
	return json.Unmarshal(b, ns)
}
