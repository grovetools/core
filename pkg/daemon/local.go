package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/pkg/env"
	"github.com/grovetools/core/pkg/models"
	"github.com/grovetools/core/pkg/paths"
	"github.com/grovetools/core/pkg/repo"
	"github.com/grovetools/core/pkg/sessions"
	"github.com/grovetools/core/pkg/workspace"
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

// GetPlansRaw returns nil, nil when the daemon is unavailable. Callers
// are expected to fall back to a direct filesystem scan using their
// own plan-loading code (flow's orchestration.LoadPlan).
func (c *LocalClient) GetPlansRaw(ctx context.Context, planDir string) ([]byte, error) {
	return nil, nil
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

// StreamWorkspaceHUD returns an error for LocalClient since HUD streaming
// requires the daemon's aggregation + debouncing infrastructure.
func (c *LocalClient) StreamWorkspaceHUD(ctx context.Context, path string) (<-chan models.WorkspaceHUD, error) {
	return nil, errors.New("workspace HUD streaming requires the grove daemon; start groved for live HUD updates")
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

// EnsureRepo runs the clone+checkout in-process when the daemon is unavailable.
func (c *LocalClient) EnsureRepo(ctx context.Context, req models.RepoEnsureRequest) (*models.RepoEnsureResponse, error) {
	mgr, err := repo.NewManager()
	if err != nil {
		return nil, err
	}
	path, commit, err := mgr.EnsureVersion(ctx, req.URL, req.Version)
	if err != nil {
		return nil, err
	}
	return &models.RepoEnsureResponse{WorktreePath: path, Commit: commit}, nil
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
			SessionID:       confirmation.JobID,
			ClaudeSessionID: confirmation.NativeID,
			PID:             confirmation.PID,
			TranscriptPath:  confirmation.TranscriptPath,
			StartedAt:       time.Now(),
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
// In local mode, this persists the status to the filesystem registry's
// metadata.json so subsequent reads (RecoverSessions) return the right
// value instead of defaulting to "running".
func (c *LocalClient) UpdateSessionStatus(ctx context.Context, jobID, status string) error {
	registry, err := sessions.NewFileSystemRegistry()
	if err != nil {
		return err
	}
	// The session directory is keyed by the native (claude) session ID, not
	// the job ID. Find the registry entry for this jobID and update its dir.
	if meta, _ := registry.Find(jobID); meta != nil {
		dirName := meta.ClaudeSessionID
		if dirName == "" {
			dirName = meta.SessionID
		}
		return registry.UpdateStatus(dirName, status)
	}
	// Fall back to using jobID as directory name.
	return registry.UpdateStatus(jobID, status)
}

// EndSession marks a session as complete or interrupted.
// In local mode, this persists the terminal status to metadata.json so
// subsequent RecoverSessions scans report the correct outcome instead of
// defaulting to "running" because the (parent) PID in pid.lock is alive.
// The session directory and pid.lock are preserved for transcript
// archival; cleanup is handled separately.
func (c *LocalClient) EndSession(ctx context.Context, jobID, outcome string) error {
	registry, err := sessions.NewFileSystemRegistry()
	if err != nil {
		return err
	}
	dirName := jobID
	if meta, _ := registry.Find(jobID); meta != nil {
		if meta.ClaudeSessionID != "" {
			dirName = meta.ClaudeSessionID
		} else if meta.SessionID != "" {
			dirName = meta.SessionID
		}
	}
	return registry.UpdateStatus(dirName, outcome)
}

// KillSession returns an error in local mode — terminating a tracked agent
// session requires the daemon so it can clean up its in-memory store and
// background workers atomically. Callers may fall back to an in-process
// syscall path when the daemon is unreachable.
func (c *LocalClient) KillSession(ctx context.Context, sessionID string) error {
	return errors.New("kill requires the grove daemon")
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

// RegisterProxyRoute is a no-op without the daemon — the proxy lives in the
// global daemon process. Callers (scoped daemons) degrade gracefully to
// direct-port access when the global daemon is unreachable.
func (c *LocalClient) RegisterProxyRoute(ctx context.Context, worktree, route string, port int) error {
	return errors.New("proxy route registration requires the global grove daemon")
}

// UnregisterProxyRoutes is a no-op without the daemon.
func (c *LocalClient) UnregisterProxyRoutes(ctx context.Context, worktree string) error {
	return errors.New("proxy route registration requires the global grove daemon")
}

// --- Channel & Autonomous stubs (require daemon) ---

func (c *LocalClient) UpdateSessionChannels(ctx context.Context, jobID string, channels []string) error {
	return errors.New("channel management requires the grove daemon")
}

func (c *LocalClient) UpdateSessionAutonomous(ctx context.Context, jobID string, config *models.AutonomousConfig) error {
	return errors.New("autonomous management requires the grove daemon")
}

func (c *LocalClient) UpdateSessionTmuxTarget(ctx context.Context, jobID, target string) error {
	return errors.New("tmux target updates require the grove daemon")
}

func (c *LocalClient) SendChannelMessage(ctx context.Context, req models.ChannelSendRequest) (*models.ChannelSendResponse, error) {
	return nil, errors.New("channel messaging requires the grove daemon")
}

func (c *LocalClient) GetChannelStatus(ctx context.Context) (*models.ChannelStatusResponse, error) {
	return nil, errors.New("channel status requires the grove daemon")
}

func (c *LocalClient) CleanupChannels(ctx context.Context) (*models.ChannelCleanupResponse, error) {
	return nil, errors.New("channel cleanup requires the grove daemon")
}

// SendSessionInput returns an error since agent input requires the daemon for tmux target resolution.
func (c *LocalClient) SendSessionInput(ctx context.Context, sessionID, input string) error {
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

// SetNavLastAccessedGroup updates the last-accessed group in the sessions.yml file directly.
func (c *LocalClient) SetNavLastAccessedGroup(ctx context.Context, group string) error {
	file, err := c.GetNavBindings(ctx)
	if err != nil {
		return err
	}

	file.LastAccessedGroup = group
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
	return os.WriteFile(sessionsPath, data, 0o644) //nolint:gosec // nav session state is not sensitive
}

// GetNavConfig loads the static nav config from the grove config files
// (the same hierarchical merge that nav itself uses) and projects it onto
// the public NavConfig type. The result includes a "default" group entry
// derived from the top-level nav.prefix so callers can treat all groups
// uniformly.
func (c *LocalClient) GetNavConfig(ctx context.Context) (*models.NavConfig, error) {
	cfg, err := config.LoadDefault()
	if err != nil {
		return &models.NavConfig{Groups: map[string]models.NavGroupConfig{}}, nil
	}

	var navCfg struct {
		Prefix string `toml:"prefix" yaml:"prefix"`
		Groups map[string]struct {
			Prefix string `toml:"prefix" yaml:"prefix"`
		} `toml:"groups" yaml:"groups"`
	}
	// Errors here mean the nav extension is missing or malformed; we still
	// return a valid (possibly empty) config rather than failing the call.
	_ = cfg.UnmarshalExtension("nav", &navCfg)

	result := &models.NavConfig{Groups: make(map[string]models.NavGroupConfig)}

	defaultPrefix := navCfg.Prefix
	if defaultPrefix == "" {
		defaultPrefix = "<prefix>"
	}
	result.Groups["default"] = models.NavGroupConfig{Prefix: defaultPrefix}

	for name, g := range navCfg.Groups {
		result.Groups[name] = models.NavGroupConfig{Prefix: g.Prefix}
	}

	return result, nil
}

// --- Memory (require daemon) ---

// SearchMemory returns an error since memory search requires daemon-managed
// SQLite + Gemini embedder state.
func (c *LocalClient) SearchMemory(ctx context.Context, req models.MemorySearchRequest) ([]models.MemorySearchResult, error) {
	return nil, errors.New("memory operations require the grove daemon; start groved first")
}

// GetMemoryCoverage returns an error since coverage analysis requires the
// daemon's memory store.
func (c *LocalClient) GetMemoryCoverage(ctx context.Context, req models.MemoryCoverageRequest) (*models.MemoryCoverageReport, error) {
	return nil, errors.New("memory operations require the grove daemon; start groved first")
}

// GetMemoryStatus returns an error since status requires the daemon's memory store.
func (c *LocalClient) GetMemoryStatus(ctx context.Context) (*models.MemoryStatusResponse, error) {
	return nil, errors.New("memory operations require the grove daemon; start groved first")
}

func (c *LocalClient) ExecuteMemoryReindex(ctx context.Context, req models.MemoryReindexRequest) (*models.MemoryReindexResponse, error) {
	return nil, errors.New("memory reindex requires the grove daemon; start groved first")
}

// --- Memory Analysis (require daemon) ---

func (c *LocalClient) GetMemoryAnalysisGC(ctx context.Context) (*models.GCAnalysisResponse, error) {
	return nil, errors.New("memory analysis requires the grove daemon; start groved first")
}

func (c *LocalClient) ExecuteMemoryGC(ctx context.Context) (*models.GCAnalysisResponse, error) {
	return nil, errors.New("memory analysis requires the grove daemon; start groved first")
}

func (c *LocalClient) GetMemoryAnalysisWorkspaces(ctx context.Context) ([]*models.WorkspaceAnalysis, error) {
	return nil, errors.New("memory analysis requires the grove daemon; start groved first")
}

func (c *LocalClient) GetMemoryAnalysisEcosystems(ctx context.Context) ([]*models.EcosystemAnalysis, error) {
	return nil, errors.New("memory analysis requires the grove daemon; start groved first")
}

func (c *LocalClient) GetMemoryAnalysisCode(ctx context.Context) (*models.CodeAnalysis, error) {
	return nil, errors.New("memory analysis requires the grove daemon; start groved first")
}

func (c *LocalClient) GetMemoryAnalysisConcepts(ctx context.Context) (*models.ConceptAnalysis, error) {
	return nil, errors.New("memory analysis requires the grove daemon; start groved first")
}

func (c *LocalClient) GetMemoryAnalysisEmbeddings(ctx context.Context) (*models.EmbeddingAnalysis, error) {
	return nil, errors.New("memory analysis requires the grove daemon; start groved first")
}

func (c *LocalClient) GetMemoryAnalysisFreshness(ctx context.Context) (*models.FreshnessAnalysis, error) {
	return nil, errors.New("memory analysis requires the grove daemon; start groved first")
}

func (c *LocalClient) GetMemoryAnalysisDuplicates(ctx context.Context) (*models.DuplicateAnalysis, error) {
	return nil, errors.New("memory analysis requires the grove daemon; start groved first")
}

func (c *LocalClient) GetMemoryAnalysisNotebooks(ctx context.Context) ([]*models.NotebookAnalysis, error) {
	return nil, errors.New("memory analysis requires the grove daemon; start groved first")
}

func (c *LocalClient) GetMemoryAnalysisContext(ctx context.Context) (*models.ContextAnalysis, error) {
	return nil, errors.New("memory analysis requires the grove daemon; start groved first")
}

// IsTerminalConnected returns false since the local client has no daemon connection.
func (c *LocalClient) IsTerminalConnected(ctx context.Context) (bool, error) {
	return false, nil
}

// SpawnAgentPane returns an error since native agent panes require the daemon relay.
func (c *LocalClient) SpawnAgentPane(ctx context.Context, req SpawnAgentRequest) error {
	return errors.New("native agent panes require the grove daemon")
}

// SendAgentInput returns an error since native agent input requires the daemon relay.
func (c *LocalClient) SendAgentInput(ctx context.Context, jobID, input string) error {
	return errors.New("native agent input requires the grove daemon")
}

// CaptureAgentPane returns an error since native agent capture requires the daemon relay.
func (c *LocalClient) CaptureAgentPane(ctx context.Context, jobID string) (string, error) {
	return "", errors.New("native agent capture requires the grove daemon")
}

// SubmitAgentCaptureResponse returns an error since capture response requires the daemon relay.
func (c *LocalClient) SubmitAgentCaptureResponse(ctx context.Context, jobID, text string) error {
	return errors.New("native agent capture response requires the grove daemon")
}

// --- Daemon PTY Management (not available in local mode) ---

func (c *LocalClient) CreatePTY(ctx context.Context, req PTYCreateRequest) (*PTYSessionInfo, error) {
	return nil, errors.New("daemon PTY requires the grove daemon (groved)")
}

func (c *LocalClient) ListPTYs(ctx context.Context) ([]PTYSessionInfo, error) {
	return nil, errors.New("daemon PTY requires the grove daemon (groved)")
}

func (c *LocalClient) KillPTY(ctx context.Context, id string) error {
	return errors.New("daemon PTY requires the grove daemon (groved)")
}

func (c *LocalClient) GetPTYAttachURL(id string) string {
	return ""
}

func (c *LocalClient) ReportTask(ctx context.Context, workspace, verb string, exitCode int, commitHash string, durationMs int64, errorSummary string) error {
	return nil // No daemon to report to
}

func (c *LocalClient) ReportTestResults(ctx context.Context, workspace string, report *models.TestReport) error {
	return nil // No daemon to report to
}

// Ensure LocalClient implements Client interface.
var _ Client = (*LocalClient)(nil)
