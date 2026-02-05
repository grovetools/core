// Package store provides the in-memory state store for the grove daemon.
package store

import (
	"github.com/grovetools/core/pkg/enrichment"
	"github.com/grovetools/core/pkg/models"
)

// State represents the complete world view of the daemon.
type State struct {
	Workspaces map[string]*enrichment.EnrichedWorkspace `json:"workspaces"` // Keyed by path
	Sessions   map[string]*models.Session               `json:"sessions"`   // Keyed by ID
}

// UpdateType defines what kind of data changed.
type UpdateType string

const (
	UpdateWorkspaces UpdateType = "workspaces"
	UpdateSessions   UpdateType = "sessions"
)

// Update represents a change to the state.
type Update struct {
	Type    UpdateType
	Payload interface{}
}
