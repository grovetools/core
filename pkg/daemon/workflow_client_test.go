package daemon

import (
	"context"
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
