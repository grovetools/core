package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/grovetools/core/pkg/env"
	"github.com/grovetools/core/pkg/models"
	"github.com/grovetools/core/pkg/paths"
	"github.com/grovetools/core/pkg/sessions"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
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

// GetSession returns a specific session by ID.
// In local mode, this scans all sessions and returns the matching one.
func (c *LocalClient) GetSession(ctx context.Context, sessionID string) (*models.Session, error) {
	allSessions, err := c.GetSessions(ctx)
	if err != nil {
		return nil, err
	}
	for _, s := range allSessions {
		if s.ID == sessionID {
			return s, nil
		}
	}
	return nil, nil // Not found
}

// RegisterSessionIntent pre-registers a session before the agent is launched.
// In local mode, this writes to the filesystem registry with PID=0 (pending).
func (c *LocalClient) RegisterSessionIntent(ctx context.Context, intent SessionIntent) error {
	registry, err := sessions.NewFileSystemRegistry()
	if err != nil {
		return err
	}

	metadata := sessions.SessionMetadata{
		SessionID:        intent.JobID,
		Provider:         intent.Provider,
		PID:              0, // Not yet known
		WorkingDirectory: intent.WorkDir,
		StartedAt:        time.Now(),
		Type:             "interactive_agent",
		JobTitle:         intent.Title,
		PlanName:         intent.PlanName,
		JobFilePath:      intent.JobFilePath,
	}

	return registry.Register(metadata)
}

// ConfirmSession links a pre-registered intent with the actual running agent.
// In local mode, this updates the filesystem registry with the actual PID and native session ID.
func (c *LocalClient) ConfirmSession(ctx context.Context, confirmation SessionConfirmation) error {
	registry, err := sessions.NewFileSystemRegistry()
	if err != nil {
		return err
	}

	// Find the existing intent by job ID
	existing, err := registry.Find(confirmation.JobID)
	if err != nil {
		// If not found, create a new entry
		metadata := sessions.SessionMetadata{
			SessionID:        confirmation.JobID,
			ClaudeSessionID:  confirmation.NativeID,
			PID:              confirmation.PID,
			TranscriptPath:   confirmation.TranscriptPath,
			StartedAt:        time.Now(),
		}
		return registry.Register(metadata)
	}

	// Update the existing entry with confirmation data
	existing.ClaudeSessionID = confirmation.NativeID
	existing.PID = confirmation.PID
	existing.TranscriptPath = confirmation.TranscriptPath

	return registry.Register(*existing)
}

// UpdateSessionStatus updates the status of an active session.
// In local mode, this is a no-op since the filesystem registry doesn't store status.
// The daemon's in-memory store tracks status; in local mode we rely on PID liveness.
func (c *LocalClient) UpdateSessionStatus(ctx context.Context, jobID string, status string) error {
	// In local mode, status is derived from PID liveness, so this is a no-op.
	// The filesystem registry doesn't have a status field.
	return nil
}

// EndSession marks a session as complete or interrupted.
// In local mode, this removes the session from the filesystem registry.
func (c *LocalClient) EndSession(ctx context.Context, jobID string, outcome string) error {
	// For local mode, we could clean up the session directory.
	// However, this is handled by the daemon's session collector in normal operation.
	// For now, this is a no-op in local mode.
	return nil
}

// SubmitJob returns an error since job execution requires the daemon.
func (c *LocalClient) SubmitJob(ctx context.Context, req models.JobSubmitRequest) (*models.JobInfo, error) {
	return nil, errors.New("job execution requires the grove daemon; use daemon.NewWithAutoStart()")
}

// CancelJob returns an error since job execution requires the daemon.
func (c *LocalClient) CancelJob(ctx context.Context, jobID string) error {
	return errors.New("job execution requires the grove daemon; use daemon.NewWithAutoStart()")
}

// GetJob returns an error since job queries require the daemon.
func (c *LocalClient) GetJob(ctx context.Context, jobID string) (*models.JobInfo, error) {
	return nil, errors.New("job execution requires the grove daemon")
}

// ListJobs returns an error since job queries require the daemon.
func (c *LocalClient) ListJobs(ctx context.Context, filter models.JobFilter) ([]*models.JobInfo, error) {
	return nil, errors.New("job execution requires the grove daemon")
}

// StreamJobLogs returns an error since log streaming requires the daemon.
func (c *LocalClient) StreamJobLogs(ctx context.Context, jobID string) (<-chan models.JobStreamEvent, error) {
	return nil, errors.New("log streaming requires the grove daemon; use daemon.NewWithAutoStart()")
}

// GetJobLogs returns an error since log fetching requires the daemon.
func (c *LocalClient) GetJobLogs(ctx context.Context, jobID string) ([]models.LogLine, error) {
	return nil, errors.New("log fetching requires the grove daemon; use daemon.NewWithAutoStart()")
}

// GetNoteIndex returns nil in local mode — TUI falls back to filesystem.
func (c *LocalClient) GetNoteIndex(ctx context.Context, workspace string) ([]*models.NoteIndexEntry, error) {
	return nil, nil
}

// NotifyNoteEvent is a no-op for LocalClient since there's no daemon to notify.
func (c *LocalClient) NotifyNoteEvent(ctx context.Context, event models.NoteEvent) error {
	return nil
}

// EnvUp returns an error since built-in environment providers require the daemon.
func (c *LocalClient) EnvUp(ctx context.Context, req env.EnvRequest) (*env.EnvResponse, error) {
	return nil, errors.New("built-in environment providers require the grove daemon; start groved first")
}

// EnvDown returns an error since built-in environment providers require the daemon.
func (c *LocalClient) EnvDown(ctx context.Context, req env.EnvRequest) (*env.EnvResponse, error) {
	return nil, errors.New("built-in environment providers require the grove daemon; start groved first")
}

// EnvStatus returns an error since built-in environment providers require the daemon.
func (c *LocalClient) EnvStatus(ctx context.Context, worktree string) (*env.EnvResponse, error) {
	return nil, errors.New("built-in environment providers require the grove daemon; start groved first")
}

// --- Channel & Autonomous stubs (require daemon) ---

func (c *LocalClient) UpdateSessionChannels(ctx context.Context, jobID string, channels []string) error {
	return errors.New("channel management requires the grove daemon")
}

func (c *LocalClient) UpdateSessionAutonomous(ctx context.Context, jobID string, config *models.AutonomousConfig) error {
	return errors.New("autonomous management requires the grove daemon")
}

func (c *LocalClient) UpdateSessionTmuxTarget(ctx context.Context, jobID string, target string) error {
	return errors.New("tmux target updates require the grove daemon")
}

func (c *LocalClient) SendChannelMessage(ctx context.Context, req models.ChannelSendRequest) (*models.ChannelSendResponse, error) {
	return nil, errors.New("channel messaging requires the grove daemon")
}

func (c *LocalClient) GetChannelStatus(ctx context.Context) (*models.ChannelStatusResponse, error) {
	return nil, errors.New("channel status requires the grove daemon")
}

// SendSessionInput returns an error since agent input requires the daemon for tmux target resolution.
func (c *LocalClient) SendSessionInput(ctx context.Context, sessionID string, input string) error {
	return errors.New("sending input to agent sessions requires the grove daemon")
}

// SendSessionInterrupt returns an error since agent interrupt requires the daemon for tmux target resolution.
func (c *LocalClient) SendSessionInterrupt(ctx context.Context, sessionID string) error {
	return errors.New("interrupting agent sessions requires the grove daemon")
}

// GetNavBindings reads the nav binding state directly from the sessions.yml file.
func (c *LocalClient) GetNavBindings(ctx context.Context) (*models.NavSessionsFile, error) {
	sessionsPath := filepath.Join(paths.StateDir(), "nav", "sessions.yml")
	data, err := os.ReadFile(sessionsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &models.NavSessionsFile{
				Sessions: make(map[string]models.NavSessionConfig),
			}, nil
		}
		return nil, fmt.Errorf("failed to read sessions file: %w", err)
	}

	var file models.NavSessionsFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("failed to parse sessions file: %w", err)
	}
	if file.Sessions == nil {
		file.Sessions = make(map[string]models.NavSessionConfig)
	}
	return &file, nil
}

// UpdateNavGroup updates a single group in the sessions.yml file directly.
func (c *LocalClient) UpdateNavGroup(ctx context.Context, group string, state models.NavGroupState) error {
	file, err := c.GetNavBindings(ctx)
	if err != nil {
		return err
	}

	if group == "default" || group == "" {
		file.Sessions = state.Sessions
	} else {
		if file.Groups == nil {
			file.Groups = make(map[string]models.NavGroupState)
		}
		file.Groups[group] = state
	}

	return c.writeNavBindings(file)
}

// UpdateNavLockedKeys updates the locked keys in the sessions.yml file directly.
func (c *LocalClient) UpdateNavLockedKeys(ctx context.Context, keys []string) error {
	file, err := c.GetNavBindings(ctx)
	if err != nil {
		return err
	}

	file.LockedKeys = keys
	return c.writeNavBindings(file)
}

func (c *LocalClient) writeNavBindings(file *models.NavSessionsFile) error {
	sessionsPath := filepath.Join(paths.StateDir(), "nav", "sessions.yml")
	data, err := yaml.Marshal(file)
	if err != nil {
		return fmt.Errorf("failed to marshal sessions: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(sessionsPath), 0o755); err != nil {
		return fmt.Errorf("failed to create nav state directory: %w", err)
	}
	return os.WriteFile(sessionsPath, data, 0o644)
}

// Ensure LocalClient implements Client interface.
var _ Client = (*LocalClient)(nil)
