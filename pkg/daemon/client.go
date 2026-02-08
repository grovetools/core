// Package daemon provides a client interface for interacting with the Grove Daemon (groved).
// It implements a transparent fallback pattern: if the daemon is running, use RPC;
// if not, fall back to direct library calls.
package daemon

import (
	"context"
	"time"

	"github.com/grovetools/core/pkg/enrichment"
	"github.com/grovetools/core/pkg/models"
	"github.com/grovetools/core/pkg/workspace"
)

// RunningConfig holds the active configuration intervals being used by the daemon.
type RunningConfig struct {
	GitInterval       time.Duration `json:"git_interval"`
	SessionInterval   time.Duration `json:"session_interval"`
	WorkspaceInterval time.Duration `json:"workspace_interval"`
	PlanInterval      time.Duration `json:"plan_interval"`
	NoteInterval      time.Duration `json:"note_interval"`
	StartedAt         time.Time     `json:"started_at"`
}

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

	// GetConfig returns the running configuration of the daemon.
	// For LocalClient, this returns an error since config is only available via daemon.
	GetConfig(ctx context.Context) (*RunningConfig, error)

	// SetFocus tells the daemon which workspaces to prioritize for scanning.
	// For LocalClient, this is a no-op since there's no daemon to notify.
	SetFocus(ctx context.Context, paths []string) error

	// Refresh triggers a re-scan of workspaces and enrichment data.
	// For LocalClient, this is a no-op since there's no daemon cache to refresh.
	// For RemoteClient, this signals the daemon to immediately re-discover workspaces.
	Refresh(ctx context.Context) error

	// IsRunning returns true if the daemon is available and responding.
	IsRunning() bool

	// Close cleans up any resources used by the client.
	Close() error
}

// StateUpdate represents an update pushed from the daemon to subscribers.
type StateUpdate struct {
	Workspaces []*enrichment.EnrichedWorkspace `json:"workspaces,omitempty"`
	Sessions   []*models.Session               `json:"sessions,omitempty"`
	UpdateType string                          `json:"update_type"`         // "full", "workspace", "session", "enrichment"
	Source     string                          `json:"source,omitempty"`    // Which collector sent this update (e.g., "git", "workspace", "session", "plan", "note")
	Scanned    int                             `json:"scanned,omitempty"`   // Number of items actually scanned (for focused updates)
}
