package mux

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/tuimux"
)

// EnvGroveScope is the environment variable holding the ambient daemon scope.
// It mirrors what tuimux.DefaultSocketPath reads; we duplicate the name here so
// the record socket resolver can consult it explicitly.
const EnvGroveScope = "GROVE_SCOPE"

// Recording client errors, mapped from the tuimux server's HTTP status codes so
// callers can present human messages without inspecting raw responses.
var (
	// ErrPtyNotFound corresponds to a 404 from the record endpoints.
	ErrPtyNotFound = errors.New("pty session not found")
	// ErrAlreadyRecording corresponds to a 409 from record/start.
	ErrAlreadyRecording = errors.New("pty session is already recording")
	// ErrNotRecording corresponds to a 409 from record/stop.
	ErrNotRecording = errors.New("pty session is not recording")
)

// PtyInfo is one entry of the tuimux /api/pty/list response. The JSON tags are
// the wire contract emitted by tuimux's pty.SessionMetadata; keep them aligned.
type PtyInfo struct {
	ID                string            `json:"id"`
	Workspace         string            `json:"workspace"`
	Label             string            `json:"label"`
	CWD               string            `json:"cwd"`
	Tags              map[string]string `json:"labels"`
	PID               int               `json:"pid"`
	ForegroundProcess string            `json:"foreground_process"`
	Recording         bool              `json:"recording"`
	RecordingPath     string            `json:"recording_path"`
}

// RecordStartRequest is the body for POST /api/pty/record/start/{id}. Path must
// be absolute; the server creates parent directories. IncludeHistory defaults
// to true server-side when nil; Version 0 defaults to 3.
type RecordStartRequest struct {
	Path           string  `json:"path"`
	IncludeHistory *bool   `json:"include_history,omitempty"`
	Title          string  `json:"title,omitempty"`
	IdleCap        float64 `json:"idle_cap,omitempty"` // seconds; 0 disables
	Version        int     `json:"version,omitempty"`  // 2 or 3; 0 => 3
}

// RecordStopResult is the 200 body from POST /api/pty/record/stop/{id}.
type RecordStopResult struct {
	Path     string  `json:"path"`
	Duration float64 `json:"duration"`
	Events   int64   `json:"events"`
	Bytes    int64   `json:"bytes"`
}

// RecordClient talks to a tuimux daemon's /api/pty record + list surface over
// its unix socket. It is a thin, dependency-light client used by `grove record`
// (and any other consumer that wants to drive daemon-side asciicast recording).
type RecordClient struct {
	socketPath string
	http       *http.Client
	api        *tuimux.ApiClient
}

// NewRecordClient returns a client bound to the tuimux daemon at socketPath.
func NewRecordClient(socketPath string) *RecordClient {
	httpc := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
		Timeout: 10 * time.Second,
	}
	return &RecordClient{
		socketPath: socketPath,
		http:       httpc,
		api:        tuimux.NewApiClient(socketPath),
	}
}

// SocketPath returns the daemon socket this client targets.
func (c *RecordClient) SocketPath() string { return c.socketPath }

// Ping reports whether the daemon is reachable.
func (c *RecordClient) Ping() error { return c.api.Ping() }

// ListPTYs returns every live PTY session known to the daemon.
func (c *RecordClient) ListPTYs() ([]PtyInfo, error) {
	resp, err := c.http.Get("http://localhost/api/pty/list")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list ptys: %s: %s", resp.Status, bytes.TrimSpace(body))
	}
	var ptys []PtyInfo
	if err := json.NewDecoder(resp.Body).Decode(&ptys); err != nil {
		return nil, fmt.Errorf("decode pty list: %w", err)
	}
	return ptys, nil
}

// StartRecording begins recording PTY id to req.Path. It maps the server's
// status codes to ErrPtyNotFound / ErrAlreadyRecording.
func (c *RecordClient) StartRecording(id string, req RecordStartRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	resp, err := c.http.Post("http://localhost/api/pty/record/start/"+id, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusCreated:
		return nil
	case http.StatusNotFound:
		return ErrPtyNotFound
	case http.StatusConflict:
		return ErrAlreadyRecording
	default:
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("start recording: %s: %s", resp.Status, bytes.TrimSpace(msg))
	}
}

// StopRecording stops the active recording on PTY id and returns its summary.
// It maps the server's status codes to ErrPtyNotFound / ErrNotRecording.
func (c *RecordClient) StopRecording(id string) (RecordStopResult, error) {
	var out RecordStopResult
	resp, err := c.http.Post("http://localhost/api/pty/record/stop/"+id, "application/json", nil)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return out, fmt.Errorf("decode stop result: %w", err)
		}
		return out, nil
	case http.StatusNotFound:
		return out, ErrPtyNotFound
	case http.StatusConflict:
		return out, ErrNotRecording
	default:
		msg, _ := io.ReadAll(resp.Body)
		return out, fmt.Errorf("stop recording: %s: %s", resp.Status, bytes.TrimSpace(msg))
	}
}

// FocusedPTY best-effort resolves the currently focused pane to its PTY session.
// It joins the treemux dispatcher's list-panes (which carries the Active flag
// and a PID) against /api/pty/list by PID. It returns (nil, nil) when focus is
// not determinable — e.g. no treemux client is attached to drive the dispatcher
// model — so callers can fall back to prompting for --pane. It does not require
// any new server surface.
func (c *RecordClient) FocusedPTY() (*PtyInfo, error) {
	ptys, err := c.ListPTYs()
	if err != nil {
		return nil, err
	}
	byPID := make(map[int]*PtyInfo, len(ptys))
	for i := range ptys {
		byPID[ptys[i].PID] = &ptys[i]
	}

	servers, err := c.api.ListServers()
	if err != nil || len(servers) == 0 {
		return nil, nil //nolint:nilerr // focus is best-effort; absence is not an error
	}
	for _, srv := range servers {
		res, err := c.api.Execute(srv.Name, []string{"list-panes", "--json"})
		if err != nil || res == nil || res.ExitCode != 0 {
			continue
		}
		var panes []struct {
			PID    int  `json:"pid"`
			Active bool `json:"active"`
			IsPTY  bool `json:"is_pty"`
		}
		if json.Unmarshal([]byte(res.Output), &panes) != nil {
			continue
		}
		for _, p := range panes {
			if p.Active {
				if info, ok := byPID[p.PID]; ok {
					return info, nil
				}
			}
		}
	}
	return nil, nil
}

// ResolveRecordSocket resolves the tuimux daemon socket for a `grove record`
// invocation, mirroring how the rest of the ecosystem locates the scope-keyed
// socket:
//
//   - An explicit atDir (the --at override) is classified with
//     workspace.ResolveScope and mapped to its scoped socket.
//   - Otherwise GROVE_TUIMUX_SOCKET wins if set (explicit socket override).
//   - Otherwise the ambient GROVE_SCOPE is used if set (grove-managed shells).
//   - Otherwise the cwd is classified with workspace.ResolveScope so a plain
//     shell inside an ecosystem worktree still finds that worktree's daemon.
//
// An empty resolved scope maps to the legacy machine-wide socket.
func ResolveRecordSocket(atDir string) string {
	if atDir != "" {
		return tuimux.ScopedSocketPath(workspace.ResolveScope(atDir))
	}
	if s := os.Getenv(EnvGroveTuimuxSocket); s != "" {
		return s
	}
	if s := os.Getenv(EnvGroveScope); s != "" {
		return tuimux.ScopedSocketPath(s)
	}
	return tuimux.ScopedSocketPath(workspace.ResolveScope(""))
}
