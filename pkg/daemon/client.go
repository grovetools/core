// Package daemon provides a client interface for interacting with the Grove Daemon (groved).
// It implements a transparent fallback pattern: if the daemon is running, use RPC;
// if not, fall back to direct library calls.
package daemon

import (
	"context"

	"github.com/grovetools/core/pkg/enrichment"
	"github.com/grovetools/core/pkg/models"
	"github.com/grovetools/core/pkg/workspace"
)

// Client defines the interface for interacting with the Grove Daemon.
// Both DaemonClient (RPC) and LocalClient (direct calls) implement this interface.
type Client interface {
	// GetWorkspaces returns all discovered workspaces.
	GetWorkspaces(ctx context.Context) ([]*workspace.WorkspaceNode, error)

	// GetEnrichedWorkspaces returns workspaces with enrichment data.
	GetEnrichedWorkspaces(ctx context.Context, opts *enrichment.EnrichmentOptions) ([]*enrichment.EnrichedWorkspace, error)

	// GetPlanStats returns aggregated plan statistics indexed by workspace path.
	GetPlanStats(ctx context.Context) (map[string]*enrichment.PlanStats, error)

	// GetNoteCounts returns aggregated note counts indexed by workspace name.
	GetNoteCounts(ctx context.Context) (map[string]*enrichment.NoteCounts, error)

	// GetSessions returns active sessions from all sources.
	GetSessions(ctx context.Context) ([]*models.Session, error)

	// StreamState subscribes to real-time state updates from the daemon.
	// Returns a channel that receives updates and a function to stop the stream.
	// For LocalClient, this returns an error since streaming is only available via daemon.
	StreamState(ctx context.Context) (<-chan StateUpdate, error)

	// IsRunning returns true if the daemon is available and responding.
	IsRunning() bool

	// Close cleans up any resources used by the client.
	Close() error
}

// StateUpdate represents an update pushed from the daemon to subscribers.
type StateUpdate struct {
	Workspaces []*enrichment.EnrichedWorkspace `json:"workspaces,omitempty"`
	Sessions   []*models.Session               `json:"sessions,omitempty"`
	UpdateType string                          `json:"update_type"` // "full", "workspace", "session", "enrichment"
}
