package daemon

import (
	"context"
	"errors"

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

// GetEnrichedWorkspaces returns workspaces without enrichment data.
// The daemon provides enrichment - in local mode, only basic workspace info is returned.
func (c *LocalClient) GetEnrichedWorkspaces(ctx context.Context, opts *models.EnrichmentOptions) ([]*models.EnrichedWorkspace, error) {
	nodes, err := c.GetWorkspaces(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]*models.EnrichedWorkspace, len(nodes))
	for i, n := range nodes {
		result[i] = &models.EnrichedWorkspace{WorkspaceNode: n}
	}
	return result, nil
}

// GetPlanStats returns an empty map in local mode.
// The daemon provides enrichment data - this is a graceful degradation.
func (c *LocalClient) GetPlanStats(ctx context.Context) (map[string]*models.PlanStats, error) {
	return make(map[string]*models.PlanStats), nil
}

// GetNoteCounts returns an empty map in local mode.
// The daemon provides enrichment data - this is a graceful degradation.
func (c *LocalClient) GetNoteCounts(ctx context.Context) (map[string]*models.NoteCounts, error) {
	return make(map[string]*models.NoteCounts), nil
}

// GetSessions returns active sessions from all sources.
// This uses the comprehensive DiscoverAll function which aggregates:
// - Interactive sessions (from ~/.grove/hooks/sessions)
// - Flow jobs (from workspace plan/chat/note directories)
// - OpenCode sessions (from ~/.local/share/opencode/storage)
//
// This provides full parity with the daemon's session registry when running in local mode.
func (c *LocalClient) GetSessions(ctx context.Context) ([]*models.Session, error) {
	return sessions.DiscoverAll()
}

// StreamState returns an error for LocalClient since streaming is only available via daemon.
// Use the daemon for real-time updates.
func (c *LocalClient) StreamState(ctx context.Context) (<-chan StateUpdate, error) {
	return nil, errors.New("streaming not available in local mode; start the daemon for real-time updates")
}

// GetConfig returns an error for LocalClient since config is only available via daemon.
func (c *LocalClient) GetConfig(ctx context.Context) (*RunningConfig, error) {
	return nil, errors.New("config not available in local mode; start the daemon to view running config")
}

// SetFocus is a no-op for LocalClient since there's no daemon to notify.
func (c *LocalClient) SetFocus(ctx context.Context, paths []string) error {
	return nil // No-op in local mode
}

// Refresh is a no-op for LocalClient since there's no daemon cache to refresh.
func (c *LocalClient) Refresh(ctx context.Context) error {
	return nil // No-op in local mode
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
