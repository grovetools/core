package daemon

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/grovetools/core/pkg/models"
)

// Compile-level interface compliance: every client implementation must
// satisfy the full Client interface, including PublishWorkflowEvent.
// WithFallback is intentionally absent — it is a {Primary, Fallback Client}
// holder, not a Client implementation itself (it has no methods).
var (
	_ Client = (*RemoteClient)(nil)
	_ Client = (*LocalClient)(nil)
)

// startUnixServer serves the given handler on a temp unix socket and returns
// the socket path. The server is torn down via t.Cleanup.
func startUnixServer(t *testing.T, handler http.Handler) string {
	t.Helper()

	// Keep the socket path short: unix socket paths have a ~104-byte limit
	// on darwin and t.TempDir() paths can exceed it.
	dir, err := os.MkdirTemp("/tmp", "grove-wf-test-")
	if err != nil {
		t.Fatalf("mkdtemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	socketPath := filepath.Join(dir, "d.sock")
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}

	srv := &http.Server{Handler: handler}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	return socketPath
}

func TestRemoteClientPublishWorkflowEvent(t *testing.T) {
	ev := models.WorkflowEvent{
		Kind:            models.WorkflowAgentStarted,
		JobID:           "job-1",
		ClaudeSessionID: "sess-1",
		AgentID:         "a1",
		AgentType:       "workflow-subagent",
		Timestamp:       time.Now().UTC(),
		Source:          models.WorkflowSourceHooks,
	}

	t.Run("ok", func(t *testing.T) {
		var gotPath, gotMethod string
		socketPath := startUnixServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath, gotMethod = r.URL.Path, r.Method
			w.WriteHeader(http.StatusOK)
		}))

		c, err := NewRemoteClient(socketPath)
		if err != nil {
			t.Fatalf("NewRemoteClient: %v", err)
		}
		if err := c.PublishWorkflowEvent(context.Background(), ev); err != nil {
			t.Fatalf("PublishWorkflowEvent: %v", err)
		}
		if gotMethod != "POST" || gotPath != "/api/workflows/event" {
			t.Errorf("got %s %s, want POST /api/workflows/event", gotMethod, gotPath)
		}
	})

	t.Run("404 from older daemon is tolerated", func(t *testing.T) {
		socketPath := startUnixServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		}))

		c, err := NewRemoteClient(socketPath)
		if err != nil {
			t.Fatalf("NewRemoteClient: %v", err)
		}
		if err := c.PublishWorkflowEvent(context.Background(), ev); err != nil {
			t.Errorf("expected 404 to be treated as success, got: %v", err)
		}
	})

	t.Run("server error surfaces", func(t *testing.T) {
		socketPath := startUnixServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))

		c, err := NewRemoteClient(socketPath)
		if err != nil {
			t.Fatalf("NewRemoteClient: %v", err)
		}
		if err := c.PublishWorkflowEvent(context.Background(), ev); err == nil {
			t.Error("expected error on 500 response, got nil")
		}
	})
}

func TestRemoteClientGetWorkflowSnapshot(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		var gotPath, gotMethod string
		socketPath := startUnixServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath, gotMethod = r.URL.Path, r.Method
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"runs": {
					"wf_abc": {
						"run_id": "wf_abc",
						"job_id": "job-1",
						"claude_session_id": "sess-1",
						"name": "p3-impl",
						"phases": ["Phase 1"],
						"agents": {"a1": {"id": "a1", "status": "completed"}},
						"started_count": 2,
						"completed_count": 1,
						"stale": false
					}
				},
				"adhoc": {"job-2": {"a2": {"id": "a2", "status": "running"}}}
			}`))
		}))

		c, err := NewRemoteClient(socketPath)
		if err != nil {
			t.Fatalf("NewRemoteClient: %v", err)
		}
		snap, err := c.GetWorkflowSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetWorkflowSnapshot: %v", err)
		}
		if gotMethod != "GET" || gotPath != "/api/workflows" {
			t.Errorf("got %s %s, want GET /api/workflows", gotMethod, gotPath)
		}
		run, ok := snap.Runs["wf_abc"]
		if !ok {
			t.Fatalf("missing run wf_abc in snapshot: %+v", snap)
		}
		if run.Name != "p3-impl" || run.JobID != "job-1" || run.StartedCount != 2 || run.CompletedCount != 1 {
			t.Errorf("run decoded wrong: %+v", run)
		}
		if run.Agents["a1"] == nil || run.Agents["a1"].Status != "completed" {
			t.Errorf("agent a1 decoded wrong: %+v", run.Agents)
		}
		if snap.Adhoc["job-2"]["a2"] == nil || snap.Adhoc["job-2"]["a2"].Status != "running" {
			t.Errorf("adhoc decoded wrong: %+v", snap.Adhoc)
		}
	})

	t.Run("404 from older daemon surfaces as error", func(t *testing.T) {
		socketPath := startUnixServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		}))

		c, err := NewRemoteClient(socketPath)
		if err != nil {
			t.Fatalf("NewRemoteClient: %v", err)
		}
		if _, err := c.GetWorkflowSnapshot(context.Background()); err == nil {
			t.Error("expected error on 404 response, got nil")
		}
	})
}

func TestLocalClientGetWorkflowSnapshotTypedError(t *testing.T) {
	c := NewLocalClient()
	snap, err := c.GetWorkflowSnapshot(context.Background())
	if snap != nil {
		t.Errorf("expected nil snapshot, got %+v", snap)
	}
	if !errors.Is(err, ErrWorkflowSnapshotUnavailable) {
		t.Errorf("expected ErrWorkflowSnapshotUnavailable, got: %v", err)
	}
}

func TestLocalClientPublishWorkflowEventNoOp(t *testing.T) {
	c := NewLocalClient()
	err := c.PublishWorkflowEvent(context.Background(), models.WorkflowEvent{
		Kind:    models.WorkflowAgentCompleted,
		JobID:   "job-1",
		AgentID: "a1",
	})
	if err != nil {
		t.Errorf("LocalClient.PublishWorkflowEvent should be a no-op, got: %v", err)
	}
}
