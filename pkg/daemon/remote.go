package daemon

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/grovetools/core/pkg/env"
	"github.com/grovetools/core/pkg/models"
	"github.com/grovetools/core/pkg/workspace"
)

// RemoteClient implements Client by calling the daemon's HTTP API over a Unix socket.
// This provides fast, cached access to workspace and session data when the daemon is running.
type RemoteClient struct {
	httpClient    *http.Client
	envHttpClient *http.Client // longer timeout for env up/down operations
	socketPath    string
	// fallback is used when the daemon responds with 404 on endpoints it doesn't
	// know about — typically because the running groved binary is older than the
	// client and predates a newly-added endpoint. Rather than silently returning
	// empty data (the original footgun), methods for endpoints that have a viable
	// in-process equivalent delegate to this LocalClient.
	fallback *LocalClient
}

// errEndpointNotFound is returned internally when the daemon responds with 404
// on a known endpoint. It signals "stale daemon binary" so callers can either
// fall back to LocalClient or surface an informative error.
var errEndpointNotFound = errors.New("daemon endpoint not found (stale groved binary?)")

// errMemoryEndpointMissing is surfaced by the memory RemoteClient methods
// when the daemon socket is reachable but the /api/memory/* routes 404.
// That means groved is running but predates the memory HTTP API — the
// user-facing fix is rebuild + restart, NOT "start groved" (which the
// LocalClient fallback message used to imply).
var errMemoryEndpointMissing = errors.New("groved is running but lacks memory endpoints; rebuild and restart groved")

// NewRemoteClient creates a new RemoteClient connected to the daemon socket.
func NewRemoteClient(socketPath string) (*RemoteClient, error) {
	// Create HTTP client that dials Unix socket
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
		DisableKeepAlives: false,
		MaxIdleConns:      10,
		IdleConnTimeout:   90 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	// TODO(streaming-progress): the daemon currently holds the HTTP
	// connection open for the full duration of an env up/down. Long
	// terraform applies (~5-30 min) caused EOFs at the previous 10m
	// cap. Real fix is streaming heartbeats from the daemon; until
	// that lands we just bump the client-side deadline to 30m so
	// realistic applies complete without tripping the timeout.
	envClient := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Minute,
	}

	return &RemoteClient{
		httpClient:    client,
		envHttpClient: envClient,
		socketPath:    socketPath,
		fallback:      NewLocalClient(),
	}, nil
}

// baseURL is the dummy host used for Unix socket HTTP requests.
// The actual connection goes through the Unix socket, not this URL.
const baseURL = "http://unix"

// GetWorkspaces returns all discovered workspaces by extracting WorkspaceNode from enriched workspaces.
func (c *RemoteClient) GetWorkspaces(ctx context.Context) ([]*workspace.WorkspaceNode, error) {
	enriched, err := c.GetEnrichedWorkspaces(ctx, nil)
	if err != nil {
		return nil, err
	}

	nodes := make([]*workspace.WorkspaceNode, len(enriched))
	for i, ew := range enriched {
		nodes[i] = ew.WorkspaceNode
	}
	return nodes, nil
}

// GetEnrichedWorkspaces returns workspaces with enrichment data from the daemon.
func (c *RemoteClient) GetEnrichedWorkspaces(ctx context.Context, opts *models.EnrichmentOptions) ([]*models.EnrichedWorkspace, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/workspaces", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspaces from daemon: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}

	var workspaces []*models.EnrichedWorkspace
	if err := json.NewDecoder(resp.Body).Decode(&workspaces); err != nil {
		return nil, fmt.Errorf("failed to decode workspaces: %w", err)
	}
	return workspaces, nil
}

// GetPlanStats returns aggregated plan statistics indexed by workspace path.
// Extracts PlanStats from enriched workspaces.
func (c *RemoteClient) GetPlanStats(ctx context.Context) (map[string]*models.PlanStats, error) {
	enriched, err := c.GetEnrichedWorkspaces(ctx, nil)
	if err != nil {
		return nil, err
	}

	stats := make(map[string]*models.PlanStats)
	for _, ew := range enriched {
		if ew.PlanStats != nil {
			stats[ew.Path] = ew.PlanStats
		}
	}
	return stats, nil
}

// GetNoteCounts returns aggregated note counts indexed by workspace name.
// Extracts NoteCounts from enriched workspaces.
func (c *RemoteClient) GetNoteCounts(ctx context.Context) (map[string]*models.NoteCounts, error) {
	enriched, err := c.GetEnrichedWorkspaces(ctx, nil)
	if err != nil {
		return nil, err
	}

	counts := make(map[string]*models.NoteCounts)
	for _, ew := range enriched {
		if ew.NoteCounts != nil {
			counts[ew.Name] = ew.NoteCounts
		}
	}
	return counts, nil
}

// GetPlansRaw fetches the cached plan list for the given plansDir from
// the daemon and returns the raw JSON body. The caller decodes into its
// own plan type (typically flow's orchestration.Plan) — see the Client
// interface comment for why the payload type doesn't live here.
func (c *RemoteClient) GetPlansRaw(ctx context.Context, planDir string) ([]byte, error) {
	reqURL := baseURL + "/api/plans?dir=" + url.QueryEscape(planDir)
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get plans from daemon: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// Stale groved binary without the /api/plans endpoint. Signal
		// "no daemon data" so callers fall back to a local scan.
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read plans response: %w", err)
	}
	return body, nil
}

// GetSessions returns active sessions from the daemon.
func (c *RemoteClient) GetSessions(ctx context.Context) ([]*models.Session, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/sessions", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get sessions from daemon: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}

	var sessions []*models.Session
	if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
		return nil, fmt.Errorf("failed to decode sessions: %w", err)
	}
	return sessions, nil
}

// GetConfig returns the running configuration of the daemon.
func (c *RemoteClient) GetConfig(ctx context.Context) (*RunningConfig, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/config", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get config from daemon: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}

	var cfg RunningConfig
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to decode config: %w", err)
	}
	return &cfg, nil
}

// SetFocus tells the daemon which workspaces to prioritize for scanning.
func (c *RemoteClient) SetFocus(ctx context.Context, paths []string) error {
	body, err := json.Marshal(map[string][]string{"paths": paths})
	if err != nil {
		return fmt.Errorf("failed to marshal focus request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/api/focus", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to set focus: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}
	return nil
}

// IsRunning returns true if the daemon is available and responding.
func (c *RemoteClient) IsRunning() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/health", nil)
	if err != nil {
		return false
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// Refresh triggers a re-scan of workspaces and enrichment data.
func (c *RemoteClient) Refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/api/refresh", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to trigger refresh: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}
	return nil
}

// StreamState subscribes to real-time state updates via Server-Sent Events (SSE).
// Returns a channel that receives updates. The channel is closed when the context is cancelled
// or the connection is lost.
func (c *RemoteClient) StreamState(ctx context.Context) (<-chan StateUpdate, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/stream", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create stream request: %w", err)
	}

	// Use a separate client with no timeout for streaming
	streamTransport := &http.Transport{
		DialContext: func(dialCtx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(dialCtx, "unix", c.socketPath)
		},
	}
	streamClient := &http.Client{
		Transport: streamTransport,
		Timeout:   0, // No timeout for streaming
	}

	resp, err := streamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to stream: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("stream returned status %d", resp.StatusCode)
	}

	ch := make(chan StateUpdate, 10)

	go func() {
		defer resp.Body.Close()
		defer close(ch)
		defer streamTransport.CloseIdleConnections()

		scanner := bufio.NewScanner(resp.Body)
		// Increase buffer size to handle large workspace updates (default is 64KB)
		buf := make([]byte, 0, 4*1024*1024) // 4MB initial capacity
		scanner.Buffer(buf, 10*1024*1024)   // 10MB max
		for scanner.Scan() {
			line := scanner.Text()

			// Skip comments and empty lines
			if strings.HasPrefix(line, ":") || line == "" {
				continue
			}

			// Parse SSE data lines
			if strings.HasPrefix(line, "data: ") {
				jsonStr := strings.TrimPrefix(line, "data: ")
				var update StateUpdate
				if err := json.Unmarshal([]byte(jsonStr), &update); err != nil {
					continue // Skip malformed data
				}

				select {
				case ch <- update:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return ch, nil
}

// StreamWorkspaceHUD subscribes to per-workspace HUD updates via SSE.
// Returns a channel that receives HUD snapshots. The channel is closed when
// the context is cancelled or the connection is lost.
func (c *RemoteClient) StreamWorkspaceHUD(ctx context.Context, path string) (<-chan models.WorkspaceHUD, error) {
	if path == "" {
		return nil, fmt.Errorf("workspace path is required")
	}

	streamURL := baseURL + "/api/workspace/hud/stream?path=" + url.QueryEscape(path)
	req, err := http.NewRequestWithContext(ctx, "GET", streamURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HUD stream request: %w", err)
	}

	// Use a separate client with no timeout for streaming.
	streamTransport := &http.Transport{
		DialContext: func(dialCtx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(dialCtx, "unix", c.socketPath)
		},
	}
	streamClient := &http.Client{
		Transport: streamTransport,
		Timeout:   0,
	}

	resp, err := streamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to HUD stream: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, fmt.Errorf("workspace HUD stream not available; rebuild and restart groved")
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("HUD stream returned status %d", resp.StatusCode)
	}

	ch := make(chan models.WorkspaceHUD, 4)

	go func() {
		defer resp.Body.Close()
		defer close(ch)
		defer streamTransport.CloseIdleConnections()

		scanner := bufio.NewScanner(resp.Body)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1*1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, ":") || line == "" {
				continue
			}
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			jsonStr := strings.TrimPrefix(line, "data: ")
			var hud models.WorkspaceHUD
			if err := json.Unmarshal([]byte(jsonStr), &hud); err != nil {
				continue
			}
			select {
			case ch <- hud:
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}

// Close cleans up any resources used by the client.
func (c *RemoteClient) Close() error {
	c.httpClient.CloseIdleConnections()
	return nil
}

// GetSession returns a specific session by ID from the daemon.
func (c *RemoteClient) GetSession(ctx context.Context, sessionID string) (*models.Session, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/sessions/"+sessionID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get session from daemon: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // Not found
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}

	var session models.Session
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return nil, fmt.Errorf("failed to decode session: %w", err)
	}
	return &session, nil
}

// RegisterSessionIntent pre-registers a session before the agent is launched.
func (c *RemoteClient) RegisterSessionIntent(ctx context.Context, intent SessionIntent) error {
	body, err := json.Marshal(intent)
	if err != nil {
		return fmt.Errorf("failed to marshal session intent: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/api/sessions/intent", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to register session intent: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}
	return nil
}

// ConfirmSession links a pre-registered intent with the actual running agent.
func (c *RemoteClient) ConfirmSession(ctx context.Context, confirmation SessionConfirmation) error {
	body, err := json.Marshal(confirmation)
	if err != nil {
		return fmt.Errorf("failed to marshal session confirmation: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/api/sessions/confirm", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to confirm session: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}
	return nil
}

// UpdateSessionStatus updates the status of an active session.
func (c *RemoteClient) UpdateSessionStatus(ctx context.Context, jobID string, status string) error {
	body, err := json.Marshal(map[string]string{"status": status})
	if err != nil {
		return fmt.Errorf("failed to marshal status update: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PATCH", baseURL+"/api/sessions/"+jobID+"/status", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to update session status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}
	return nil
}

// EndSession marks a session as complete or interrupted.
func (c *RemoteClient) EndSession(ctx context.Context, jobID string, outcome string) error {
	body, err := json.Marshal(map[string]string{"outcome": outcome})
	if err != nil {
		return fmt.Errorf("failed to marshal end session request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/api/sessions/"+jobID+"/end", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to end session: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}
	return nil
}

// KillSession terminates a tracked agent session via the daemon. The daemon
// looks up the session by ID, sends SIGTERM to the tracked PID, and removes
// the filesystem registry entry. A 404 response from an older daemon that
// does not implement the kill endpoint is surfaced as a sentinel error so
// callers can fall back to the in-process syscall path.
func (c *RemoteClient) KillSession(ctx context.Context, sessionID string) error {
	req, err := http.NewRequestWithContext(ctx, "DELETE", baseURL+"/api/sessions/"+sessionID, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to kill session: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// 404 may mean either "session not found" (modern daemon) or
		// "endpoint not implemented" (older daemon). Either way, the
		// caller should treat this as "daemon couldn't kill it" and
		// decide whether to fall back. Returning a wrapped error keeps
		// the failure observable in the TUI status line.
		return fmt.Errorf("daemon returned 404 for kill session %s", sessionID)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}
	return nil
}

// SubmitJob submits a job to the daemon for execution.
func (c *RemoteClient) SubmitJob(ctx context.Context, req models.JobSubmitRequest) (*models.JobInfo, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal job request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/api/jobs", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to submit job: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}

	var info models.JobInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to decode job info: %w", err)
	}
	return &info, nil
}

// CancelJob cancels a running or queued job.
func (c *RemoteClient) CancelJob(ctx context.Context, jobID string) error {
	req, err := http.NewRequestWithContext(ctx, "DELETE", baseURL+"/api/jobs/"+jobID, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to cancel job: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}
	return nil
}

// GetJob returns the current state of a specific job.
func (c *RemoteClient) GetJob(ctx context.Context, jobID string) (*models.JobInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/jobs/"+jobID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get job from daemon: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}

	var info models.JobInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to decode job info: %w", err)
	}
	return &info, nil
}

// ListJobs returns jobs matching the given filter.
func (c *RemoteClient) ListJobs(ctx context.Context, filter models.JobFilter) ([]*models.JobInfo, error) {
	url := baseURL + "/api/jobs"
	if filter.Status != "" {
		url += "?status=" + filter.Status
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list jobs from daemon: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}

	var jobs []*models.JobInfo
	if err := json.NewDecoder(resp.Body).Decode(&jobs); err != nil {
		return nil, fmt.Errorf("failed to decode jobs: %w", err)
	}
	return jobs, nil
}

// GetJobLogs returns historical log content for a job.
func (c *RemoteClient) GetJobLogs(ctx context.Context, jobID string) ([]models.LogLine, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/jobs/"+jobID+"/logs", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get job logs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("job %s not found", jobID)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}

	var lines []models.LogLine
	if err := json.NewDecoder(resp.Body).Decode(&lines); err != nil {
		return nil, fmt.Errorf("failed to decode log lines: %w", err)
	}
	return lines, nil
}

// StreamJobLogs subscribes to real-time log output for a specific job via SSE.
func (c *RemoteClient) StreamJobLogs(ctx context.Context, jobID string) (<-chan models.JobStreamEvent, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/jobs/"+jobID+"/logs/stream", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create stream request: %w", err)
	}

	// Use a separate client with no timeout for streaming
	streamTransport := &http.Transport{
		DialContext: func(dialCtx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(dialCtx, "unix", c.socketPath)
		},
	}
	streamClient := &http.Client{
		Transport: streamTransport,
		Timeout:   0, // No timeout for streaming
	}

	resp, err := streamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to log stream: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("log stream returned status %d", resp.StatusCode)
	}

	ch := make(chan models.JobStreamEvent, 100)

	go func() {
		defer resp.Body.Close()
		defer close(ch)
		defer streamTransport.CloseIdleConnections()

		scanner := bufio.NewScanner(resp.Body)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1*1024*1024)

		var currentEvent string
		for scanner.Scan() {
			line := scanner.Text()

			// Skip comments and empty lines
			if strings.HasPrefix(line, ":") || line == "" {
				currentEvent = "" // Reset on empty line (end of SSE message)
				continue
			}

			if strings.HasPrefix(line, "event: ") {
				currentEvent = strings.TrimPrefix(line, "event: ")
				continue
			}

			if strings.HasPrefix(line, "data: ") {
				jsonStr := strings.TrimPrefix(line, "data: ")

				var event models.JobStreamEvent
				if currentEvent == "log" {
					var logLine models.LogLine
					if err := json.Unmarshal([]byte(jsonStr), &logLine); err == nil {
						event = models.JobStreamEvent{
							Event: "log",
							Line:  &logLine,
						}
					} else {
						continue
					}
				} else if currentEvent == "status" {
					if err := json.Unmarshal([]byte(jsonStr), &event); err != nil {
						continue
					}
					event.Event = "status"
				} else {
					// Generic fallback
					if err := json.Unmarshal([]byte(jsonStr), &event); err != nil {
						continue
					}
				}

				select {
				case ch <- event:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return ch, nil
}

// GetNoteIndex returns the daemon's cached note index, optionally filtered by workspace.
func (c *RemoteClient) GetNoteIndex(ctx context.Context, workspace string) ([]*models.NoteIndexEntry, error) {
	url := baseURL + "/api/notes/index"
	if workspace != "" {
		url += "?workspace=" + workspace
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get note index from daemon: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}

	var entries []*models.NoteIndexEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("failed to decode note index: %w", err)
	}
	return entries, nil
}

// NotifyNoteEvent sends a note mutation event to the daemon.
func (c *RemoteClient) NotifyNoteEvent(ctx context.Context, event models.NoteEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal note event: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/api/notes/event", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send note event: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}
	return nil
}

// EnvUp requests the daemon to spin up an environment.
// Uses a 10-minute timeout to accommodate slow operations like docker builds.
func (c *RemoteClient) EnvUp(ctx context.Context, req env.EnvRequest) (*env.EnvResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal env request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/api/env/up", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.envHttpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call daemon env up: %w", err)
	}
	defer resp.Body.Close()

	var result env.EnvResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode env response: %w", err)
	}
	return &result, nil
}

// EnvDown requests the daemon to tear down an environment.
// Uses a 10-minute timeout to accommodate slow teardown operations.
func (c *RemoteClient) EnvDown(ctx context.Context, req env.EnvRequest) (*env.EnvResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal env request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/api/env/down", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.envHttpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call daemon env down: %w", err)
	}
	defer resp.Body.Close()

	var result env.EnvResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode env response: %w", err)
	}
	return &result, nil
}

// EnsureRepo asks the daemon to clone+checkout a repository at the given version.
func (c *RemoteClient) EnsureRepo(ctx context.Context, req models.RepoEnsureRequest) (*models.RepoEnsureResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal repo ensure request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/api/repos/ensure", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	// envHttpClient (30m) — clones can be slow; the regular httpClient's 10s would trip on large repos.
	resp, err := c.envHttpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call daemon ensure repo: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound && c.fallback != nil {
		return c.fallback.EnsureRepo(ctx, req)
	}
	if resp.StatusCode != http.StatusOK {
		var msg struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&msg)
		if msg.Error != "" {
			return nil, fmt.Errorf("daemon ensure repo: %s", msg.Error)
		}
		return nil, fmt.Errorf("daemon ensure repo: status %d", resp.StatusCode)
	}
	var out models.RepoEnsureResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode repo ensure response: %w", err)
	}
	return &out, nil
}

// EnvStatus requests environment status from the daemon.
func (c *RemoteClient) EnvStatus(ctx context.Context, worktree string) (*env.EnvResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/env/status?worktree="+worktree, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call daemon env status: %w", err)
	}
	defer resp.Body.Close()

	var result env.EnvResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode env response: %w", err)
	}
	return &result, nil
}

// RegisterProxyRoute asks the daemon (expected to be the global/unscoped
// one) to add a host-based route for the given worktree/route -> port.
func (c *RemoteClient) RegisterProxyRoute(ctx context.Context, worktree, route string, port int) error {
	body, err := json.Marshal(env.ProxyRouteRequest{Worktree: worktree, Route: route, Port: port})
	if err != nil {
		return fmt.Errorf("marshal proxy register request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/api/proxy/register", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("call daemon proxy register: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("daemon returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// UnregisterProxyRoutes asks the daemon to drop every route keyed by worktree.
func (c *RemoteClient) UnregisterProxyRoutes(ctx context.Context, worktree string) error {
	body, err := json.Marshal(env.ProxyUnregisterRequest{Worktree: worktree})
	if err != nil {
		return fmt.Errorf("marshal proxy unregister request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/api/proxy/unregister", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("call daemon proxy unregister: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("daemon returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// --- Channel & Autonomous Management ---

// UpdateSessionChannels updates the active channels for a session.
func (c *RemoteClient) UpdateSessionChannels(ctx context.Context, jobID string, channels []string) error {
	payload := models.SessionChannelsRequest{Channels: channels}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal channels request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/api/sessions/%s/channels", baseURL, jobID), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}
	return nil
}

// UpdateSessionAutonomous updates the autonomous config for a session.
func (c *RemoteClient) UpdateSessionAutonomous(ctx context.Context, jobID string, config *models.AutonomousConfig) error {
	body, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal autonomous config: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/api/sessions/%s/autonomous", baseURL, jobID), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}
	return nil
}

// UpdateSessionTmuxTarget updates the tmux target for a session.
func (c *RemoteClient) UpdateSessionTmuxTarget(ctx context.Context, jobID string, target string) error {
	payload := models.SessionPatchRequest{TmuxTarget: target}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal patch request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PATCH", fmt.Sprintf("%s/api/sessions/%s", baseURL, jobID), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}
	return nil
}

// SendChannelMessage sends a message via an external channel.
func (c *RemoteClient) SendChannelMessage(ctx context.Context, req models.ChannelSendRequest) (*models.ChannelSendResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal send request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/api/channels/send", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}

	var result models.ChannelSendResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}

// GetChannelStatus returns the status of the channel system.
func (c *RemoteClient) GetChannelStatus(ctx context.Context) (*models.ChannelStatusResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/channels/status", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}

	var result models.ChannelStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}

// SendSessionInput sends input to an interactive agent session via the daemon.
func (c *RemoteClient) SendSessionInput(ctx context.Context, sessionID string, input string) error {
	body, err := json.Marshal(map[string]string{"input": input})
	if err != nil {
		return fmt.Errorf("failed to marshal input request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/api/sessions/"+sessionID+"/input", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send session input: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// LocalClient can't send input to a tmux pane without the daemon's
		// session store, so surface a clear error pointing at the stale binary
		// rather than silently delegating to a LocalClient stub.
		return fmt.Errorf("send input to session %s: %w — rebuild/restart groved", sessionID, errEndpointNotFound)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}
	return nil
}

// SendSessionInterrupt sends Ctrl+C to interrupt an interactive agent session via the daemon.
func (c *RemoteClient) SendSessionInterrupt(ctx context.Context, sessionID string) error {
	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/api/sessions/"+sessionID+"/interrupt", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send session interrupt: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("interrupt session %s: %w — rebuild/restart groved", sessionID, errEndpointNotFound)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}
	return nil
}

// GetNavBindings returns the current nav binding state from the daemon.
// If the daemon is stale and returns 404, transparently falls back to LocalClient,
// which reads sessions.yml directly.
func (c *RemoteClient) GetNavBindings(ctx context.Context) (*models.NavSessionsFile, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/nav/bindings", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get nav bindings from daemon: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return c.fallback.GetNavBindings(ctx)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}

	var file models.NavSessionsFile
	if err := json.NewDecoder(resp.Body).Decode(&file); err != nil {
		return nil, fmt.Errorf("failed to decode nav bindings: %w", err)
	}
	return &file, nil
}

// GetNavConfig returns the static nav configuration from the daemon.
// Falls back to LocalClient on 404 (stale daemon binary that doesn't expose
// /api/nav/config yet) so older daemons keep working.
func (c *RemoteClient) GetNavConfig(ctx context.Context) (*models.NavConfig, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/nav/config", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get nav config from daemon: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return c.fallback.GetNavConfig(ctx)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}

	var cfg models.NavConfig
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to decode nav config: %w", err)
	}
	return &cfg, nil
}

// UpdateNavGroup updates the session state for a single group via the daemon.
// Falls back to LocalClient on 404 (stale daemon binary).
func (c *RemoteClient) UpdateNavGroup(ctx context.Context, group string, state models.NavGroupState) error {
	body, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal nav group state: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", baseURL+"/api/nav/groups/"+group, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to update nav group: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return c.fallback.UpdateNavGroup(ctx, group, state)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}
	return nil
}

// UpdateNavLockedKeys updates the global locked keys list via the daemon.
// Falls back to LocalClient on 404 (stale daemon binary).
func (c *RemoteClient) UpdateNavLockedKeys(ctx context.Context, keys []string) error {
	body, err := json.Marshal(keys)
	if err != nil {
		return fmt.Errorf("failed to marshal locked keys: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", baseURL+"/api/nav/locked-keys", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to update nav locked keys: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return c.fallback.UpdateNavLockedKeys(ctx, keys)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}
	return nil
}

// SetNavLastAccessedGroup updates the last-accessed group via the daemon.
// Falls back to LocalClient on 404 (stale daemon binary).
func (c *RemoteClient) SetNavLastAccessedGroup(ctx context.Context, group string) error {
	body, err := json.Marshal(map[string]string{"group": group})
	if err != nil {
		return fmt.Errorf("failed to marshal last-accessed group: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", baseURL+"/api/nav/last-accessed", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to update nav last-accessed group: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return c.fallback.SetNavLastAccessedGroup(ctx, group)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}
	return nil
}

// --- Memory Search ---

// SearchMemory runs a hybrid memory search via the daemon.
func (c *RemoteClient) SearchMemory(ctx context.Context, req models.MemorySearchRequest) ([]models.MemorySearchResult, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal memory search request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/api/memory/search", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to search memory: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, errMemoryEndpointMissing
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}

	var results []models.MemorySearchResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, fmt.Errorf("failed to decode memory search results: %w", err)
	}
	return results, nil
}

// GetMemoryCoverage requests a coverage report from the daemon's memory store.
func (c *RemoteClient) GetMemoryCoverage(ctx context.Context, req models.MemoryCoverageRequest) (*models.MemoryCoverageReport, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal coverage request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/api/memory/coverage", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to get coverage: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, errMemoryEndpointMissing
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}

	var report models.MemoryCoverageReport
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		return nil, fmt.Errorf("failed to decode coverage report: %w", err)
	}
	return &report, nil
}

// GetMemoryStatus returns stats about the daemon's memory store.
func (c *RemoteClient) GetMemoryStatus(ctx context.Context) (*models.MemoryStatusResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/memory/status", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get memory status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, errMemoryEndpointMissing
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}

	var status models.MemoryStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("failed to decode memory status: %w", err)
	}
	return &status, nil
}

// IsTerminalConnected checks if a groveterm instance is connected to the daemon via SSE.
func (c *RemoteClient) IsTerminalConnected(ctx context.Context) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/system/terminal-status", nil)
	if err != nil {
		return false, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("terminal status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// Stale daemon binary that doesn't have this endpoint yet
		return false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}

	var result struct {
		Connected bool `json:"connected"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("decode response: %w", err)
	}
	return result.Connected, nil
}

// SpawnAgentPane requests groveterm to spawn a native agent pane via the daemon relay.
func (c *RemoteClient) SpawnAgentPane(ctx context.Context, req SpawnAgentRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal spawn request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/api/agents/spawn", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("spawn agent pane: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}
	return nil
}

// SendAgentInput relays input text to a native agent pane in groveterm.
func (c *RemoteClient) SendAgentInput(ctx context.Context, jobID string, input string) error {
	body, err := json.Marshal(map[string]string{"input": input})
	if err != nil {
		return fmt.Errorf("marshal input: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/api/agents/"+jobID+"/input", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send agent input: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}
	return nil
}

// CaptureAgentPane requests a screen capture from a native agent pane.
func (c *RemoteClient) CaptureAgentPane(ctx context.Context, jobID string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/agents/"+jobID+"/capture", nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("capture agent pane: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusGatewayTimeout {
		return "", fmt.Errorf("capture timeout: groveterm did not respond")
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return "", fmt.Errorf("read capture response: %w", err)
	}
	return buf.String(), nil
}

// SubmitAgentCaptureResponse sends the captured screen text back to the daemon.
func (c *RemoteClient) SubmitAgentCaptureResponse(ctx context.Context, jobID string, text string) error {
	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/api/agents/"+jobID+"/capture_response", strings.NewReader(text))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("submit capture response: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}
	return nil
}

// --- Daemon PTY Management ---

func (c *RemoteClient) CreatePTY(ctx context.Context, req PTYCreateRequest) (*PTYSessionInfo, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal pty create request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/api/pty/create", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("create pty: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, errEndpointNotFound
	}
	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}

	var info PTYSessionInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decode pty session: %w", err)
	}
	return &info, nil
}

func (c *RemoteClient) ListPTYs(ctx context.Context) ([]PTYSessionInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/pty/list", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list ptys: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, errEndpointNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}

	var list []PTYSessionInfo
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("decode pty list: %w", err)
	}
	return list, nil
}

func (c *RemoteClient) KillPTY(ctx context.Context, id string) error {
	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/api/pty/kill/"+id, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("kill pty: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("pty session %s not found", id)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}
	return nil
}

func (c *RemoteClient) GetPTYAttachURL(id string) string {
	// The WebSocket dialer will connect over the Unix socket; the host is
	// irrelevant. We use "ws://unix" to match the baseURL convention.
	return "ws://unix/api/pty/attach/" + id
}

// SocketPath returns the Unix socket path used by this client.
// Used by the terminal to configure WebSocket dialers for PTY attach.
func (c *RemoteClient) SocketPath() string {
	return c.socketPath
}

// Ensure RemoteClient implements Client interface.
var _ Client = (*RemoteClient)(nil)
