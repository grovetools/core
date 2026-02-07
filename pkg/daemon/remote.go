package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/grovetools/core/pkg/enrichment"
	"github.com/grovetools/core/pkg/models"
	"github.com/grovetools/core/pkg/workspace"
)

// RemoteClient implements Client by calling the daemon's HTTP API over a Unix socket.
// This provides fast, cached access to workspace and session data when the daemon is running.
type RemoteClient struct {
	httpClient *http.Client
	socketPath string
}

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

	return &RemoteClient{
		httpClient: client,
		socketPath: socketPath,
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
func (c *RemoteClient) GetEnrichedWorkspaces(ctx context.Context, opts *enrichment.EnrichmentOptions) ([]*enrichment.EnrichedWorkspace, error) {
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

	var workspaces []*enrichment.EnrichedWorkspace
	if err := json.NewDecoder(resp.Body).Decode(&workspaces); err != nil {
		return nil, fmt.Errorf("failed to decode workspaces: %w", err)
	}
	return workspaces, nil
}

// GetPlanStats returns aggregated plan statistics indexed by workspace path.
// Extracts PlanStats from enriched workspaces.
func (c *RemoteClient) GetPlanStats(ctx context.Context) (map[string]*enrichment.PlanStats, error) {
	enriched, err := c.GetEnrichedWorkspaces(ctx, nil)
	if err != nil {
		return nil, err
	}

	stats := make(map[string]*enrichment.PlanStats)
	for _, ew := range enriched {
		if ew.PlanStats != nil {
			stats[ew.Path] = ew.PlanStats
		}
	}
	return stats, nil
}

// GetNoteCounts returns aggregated note counts indexed by workspace name.
// Extracts NoteCounts from enriched workspaces.
func (c *RemoteClient) GetNoteCounts(ctx context.Context) (map[string]*enrichment.NoteCounts, error) {
	enriched, err := c.GetEnrichedWorkspaces(ctx, nil)
	if err != nil {
		return nil, err
	}

	counts := make(map[string]*enrichment.NoteCounts)
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

// Ensure RemoteClient implements Client interface.
var _ Client = (*RemoteClient)(nil)
