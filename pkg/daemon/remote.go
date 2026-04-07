package daemon

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
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

	envClient := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Minute,
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

// --- Channel & Autonomous Management ---

// UpdateSessionChannels updates the active channels for a session.
func (c *RemoteClient) UpdateSessionChannels(ctx context.Context, jobID string, channels []string) error {
	payload := models.SessionChannelsRequest{Channels: channels}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal channels request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("http://daemon/api/sessions/%s/channels", jobID), bytes.NewReader(body))
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

	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("http://daemon/api/sessions/%s/autonomous", jobID), bytes.NewReader(body))
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

	req, err := http.NewRequestWithContext(ctx, "PATCH", fmt.Sprintf("http://daemon/api/sessions/%s", jobID), bytes.NewReader(body))
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

	httpReq, err := http.NewRequestWithContext(ctx, "POST", "http://daemon/api/channels/send", bytes.NewReader(body))
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
	req, err := http.NewRequestWithContext(ctx, "GET", "http://daemon/api/channels/status", nil)
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

// Ensure RemoteClient implements Client interface.
var _ Client = (*RemoteClient)(nil)
