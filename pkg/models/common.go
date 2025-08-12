package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Constants for validation limits
const (
	MaxFilterStatusCount   = 10
	MaxFilterTagsCount     = 20
	MaxFilterMetadataCount = 50
	MaxFilterLimit         = 1000
)

// Timestamps provides common time tracking fields
type Timestamps struct {
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
}

// Identifiable provides common ID fields
type Identifiable struct {
	ID   string `json:"id" db:"id"`
	UUID string `json:"uuid,omitempty" db:"uuid"`
}

// JSONField is a generic type for JSON database fields
type JSONField[T any] struct {
	Data T
}

func (j JSONField[T]) Value() (driver.Value, error) {
	b, err := json.Marshal(j.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return b, nil
}

func (j *JSONField[T]) Scan(value interface{}) error {
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
		return fmt.Errorf("cannot scan %T into JSONField", value)
	}

	if err := json.Unmarshal(b, &j.Data); err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %w", err)
	}
	return nil
}

// Result represents a generic operation result
type Result[T any] struct {
	Data  T      `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
	Ok    bool   `json:"ok"`
}

// Page represents paginated results
type Page[T any] struct {
	Items    []T  `json:"items"`
	Total    int  `json:"total"`
	Page     int  `json:"page"`
	PageSize int  `json:"page_size"`
	HasNext  bool `json:"has_next"`
	HasPrev  bool `json:"has_prev"`
}

// Filter represents common query filters
type Filter struct {
	StartTime *time.Time        `json:"start_time,omitempty"`
	EndTime   *time.Time        `json:"end_time,omitempty"`
	Status    []string          `json:"status,omitempty"`
	Tags      []string          `json:"tags,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	OrderBy   string            `json:"order_by,omitempty"`
	Limit     int               `json:"limit,omitempty"`
	Offset    int               `json:"offset,omitempty"`
}

// Validate validates the filter constraints
func (f *Filter) Validate() error {
	if len(f.Status) > MaxFilterStatusCount {
		return fmt.Errorf("too many status filters: %d, maximum allowed: %d", len(f.Status), MaxFilterStatusCount)
	}

	if len(f.Tags) > MaxFilterTagsCount {
		return fmt.Errorf("too many tag filters: %d, maximum allowed: %d", len(f.Tags), MaxFilterTagsCount)
	}

	if len(f.Metadata) > MaxFilterMetadataCount {
		return fmt.Errorf("too many metadata filters: %d, maximum allowed: %d", len(f.Metadata), MaxFilterMetadataCount)
	}

	if f.Limit > MaxFilterLimit {
		return fmt.Errorf("limit too large: %d, maximum allowed: %d", f.Limit, MaxFilterLimit)
	}

	if f.Limit < 0 {
		return errors.New("limit cannot be negative")
	}

	if f.Offset < 0 {
		return errors.New("offset cannot be negative")
	}

	if f.StartTime != nil && f.EndTime != nil && f.StartTime.After(*f.EndTime) {
		return errors.New("start time cannot be after end time")
	}

	return nil
}
