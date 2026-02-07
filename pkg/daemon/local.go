package daemon

import (
	"context"
	"errors"

	"github.com/grovetools/core/pkg/enrichment"
	"github.com/grovetools/core/pkg/models"
	"github.com/grovetools/core/pkg/sessions"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/sirupsen/logrus"
)

// LocalClient implements Client by calling library functions directly.
// This is used when the daemon is not running, providing the same API
// but executing all operations in-process.
type LocalClient struct {
	logger *logrus.Logger
}

// NewLocalClient creates a new LocalClient.
func NewLocalClient() *LocalClient {
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)
	return &LocalClient{logger: logger}
}

// GetWorkspaces returns all discovered workspaces by calling the discovery service directly.
func (c *LocalClient) GetWorkspaces(ctx context.Context) ([]*workspace.WorkspaceNode, error) {
	return workspace.GetProjects(c.logger)
}

// GetEnrichedWorkspaces returns workspaces with enrichment data.
func (c *LocalClient) GetEnrichedWorkspaces(ctx context.Context, opts *enrichment.EnrichmentOptions) ([]*enrichment.EnrichedWorkspace, error) {
	nodes, err := c.GetWorkspaces(ctx)
	if err != nil {
		return nil, err
	}
	return enrichment.EnrichWorkspaces(ctx, nodes, opts), nil
}

// GetPlanStats returns aggregated plan statistics.
func (c *LocalClient) GetPlanStats(ctx context.Context) (map[string]*enrichment.PlanStats, error) {
	return enrichment.FetchPlanStatsMap()
}

// GetNoteCounts returns aggregated note counts.
func (c *LocalClient) GetNoteCounts(ctx context.Context) (map[string]*enrichment.NoteCounts, error) {
	return enrichment.FetchNoteCountsMap()
}

// GetSessions returns active sessions from filesystem discovery.
// Note: This is a simplified version that only returns live sessions from the registry.
// For full session discovery including flow jobs and database sessions,
// the hooks package provides more comprehensive discovery.
func (c *LocalClient) GetSessions(ctx context.Context) ([]*models.Session, error) {
	return sessions.DiscoverLiveSessions()
}

// StreamState returns an error for LocalClient since streaming is only available via daemon.
// Use the daemon for real-time updates.
func (c *LocalClient) StreamState(ctx context.Context) (<-chan StateUpdate, error) {
	return nil, errors.New("streaming not available in local mode; start the daemon for real-time updates")
}

// IsRunning returns false since this is the local fallback client.
func (c *LocalClient) IsRunning() bool {
	return false
}

// Close is a no-op for LocalClient.
func (c *LocalClient) Close() error {
	return nil
}

// Ensure LocalClient implements Client interface.
var _ Client = (*LocalClient)(nil)
