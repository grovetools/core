package daemon

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"
)

// serveSyncUnix serves fake /api/sync/* endpoints on a unix socket, echoing
// the daemon's wire shapes (sync_handler.go syncStatusResponse /
// syncOutboxResponse / syncConflictResponse). It records the workspace query
// param it saw so tests can assert the client passes filters through.
func serveSyncUnix(t *testing.T, sockPath string, lastWorkspace *string) {
	t.Helper()
	ul, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen unix %s: %v", sockPath, err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/sync/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"enabled": true,
			"db_path": "/tmp/sync.db",
			"origin_id": "laptop-1",
			"documents": 42,
			"documents_diverged": 1,
			"outbox_pending": 3,
			"outbox_parked": 2,
			"workspaces": [{
				"name": "notes",
				"cursor": 137,
				"last_synced_at": "2026-07-12T10:00:00Z",
				"hydration": {
					"workspace": "notes",
					"running": true,
					"scanned": 500,
					"enqueued": 12,
					"quarantined": 1,
					"started_at": "2026-07-12T09:59:00Z",
					"files_per_sec": 83.5
				}
			}]
		}`))
	})
	mux.HandleFunc("/api/sync/outbox", func(w http.ResponseWriter, r *http.Request) {
		*lastWorkspace = r.URL.Query().Get("workspace")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{
			"id": 7,
			"document_id": "doc-1",
			"workspace": "notes",
			"event_type": "upsert",
			"path": "inbox/todo.md",
			"content_hash": "abc123",
			"created_at": "2026-07-12T09:00:00Z",
			"parked": true,
			"attempts": 4,
			"next_retry_at": "2026-07-12T11:00:00Z",
			"park_reason": "secret_quarantine"
		}]`))
	})
	mux.HandleFunc("/api/sync/conflicts", func(w http.ResponseWriter, r *http.Request) {
		*lastWorkspace = r.URL.Query().Get("workspace")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{
			"workspace": "notes",
			"path": "plans/roadmap.md",
			"document_id": "doc-9",
			"artifact": "plans/roadmap.md.doc-9.conflict.md",
			"artifact_content": "<<<",
			"base_content": "base"
		}]`))
	})
	srv := &http.Server{Handler: mux}
	go srv.Serve(ul)
	t.Cleanup(func() { srv.Close(); ul.Close() })
}

// TestGetSyncStatusDecodesWirePayload proves GetSyncStatus decodes the
// daemon's syncStatusResponse shape, including nested workspace/hydration.
func TestGetSyncStatusDecodesWirePayload(t *testing.T) {
	sockPath := shortTempSocket(t)
	var ws string
	serveSyncUnix(t, sockPath, &ws)

	client, err := NewRemoteClient(sockPath)
	if err != nil {
		t.Fatalf("NewRemoteClient: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	st, err := client.GetSyncStatus(ctx)
	if err != nil {
		t.Fatalf("GetSyncStatus: %v", err)
	}
	if !st.Enabled || st.OriginID != "laptop-1" || st.Documents != 42 ||
		st.DocumentsDiverged != 1 || st.OutboxPending != 3 || st.OutboxParked != 2 {
		t.Fatalf("headline fields mis-decoded: %+v", st)
	}
	if len(st.Workspaces) != 1 {
		t.Fatalf("want 1 workspace, got %d", len(st.Workspaces))
	}
	w := st.Workspaces[0]
	if w.Name != "notes" || w.Cursor != 137 || w.LastSyncedAt.IsZero() {
		t.Fatalf("workspace fields mis-decoded: %+v", w)
	}
	if w.Hydration == nil || !w.Hydration.Running || w.Hydration.Scanned != 500 ||
		w.Hydration.Enqueued != 12 || w.Hydration.Quarantined != 1 || w.Hydration.FilesPerSec != 83.5 {
		t.Fatalf("hydration fields mis-decoded: %+v", w.Hydration)
	}
}

// TestGetSyncOutboxDecodesAndFilters proves GetSyncOutbox decodes parked
// metadata and forwards the workspace query parameter.
func TestGetSyncOutboxDecodesAndFilters(t *testing.T) {
	sockPath := shortTempSocket(t)
	var ws string
	serveSyncUnix(t, sockPath, &ws)

	client, err := NewRemoteClient(sockPath)
	if err != nil {
		t.Fatalf("NewRemoteClient: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	entries, err := client.GetSyncOutbox(ctx, "notes")
	if err != nil {
		t.Fatalf("GetSyncOutbox: %v", err)
	}
	if ws != "notes" {
		t.Fatalf("workspace filter not forwarded: got %q", ws)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.ID != 7 || e.Path != "inbox/todo.md" || !e.Parked || e.Attempts != 4 ||
		e.ParkReason != "secret_quarantine" || e.NextRetryAt.IsZero() {
		t.Fatalf("outbox fields mis-decoded: %+v", e)
	}
}

// TestGetSyncConflictsDecodes proves GetSyncConflicts decodes the artifact
// payload; the empty workspace arg must not emit a query parameter.
func TestGetSyncConflictsDecodes(t *testing.T) {
	sockPath := shortTempSocket(t)
	var ws string
	serveSyncUnix(t, sockPath, &ws)

	client, err := NewRemoteClient(sockPath)
	if err != nil {
		t.Fatalf("NewRemoteClient: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conflicts, err := client.GetSyncConflicts(ctx, "")
	if err != nil {
		t.Fatalf("GetSyncConflicts: %v", err)
	}
	if ws != "" {
		t.Fatalf("unexpected workspace filter: got %q", ws)
	}
	if len(conflicts) != 1 {
		t.Fatalf("want 1 conflict, got %d", len(conflicts))
	}
	c := conflicts[0]
	if c.Workspace != "notes" || c.Path != "plans/roadmap.md" || c.DocumentID != "doc-9" ||
		c.Artifact != "plans/roadmap.md.doc-9.conflict.md" || c.BaseContent != "base" {
		t.Fatalf("conflict fields mis-decoded: %+v", c)
	}
}

// TestSyncEndpoints404 proves a stale daemon (no /api/sync routes) surfaces
// errEndpointNotFound rather than a decode error, mirroring satellites.
func TestSyncEndpoints404(t *testing.T) {
	sockPath := shortTempSocket(t)
	ul, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	srv := &http.Server{Handler: http.NotFoundHandler()}
	go srv.Serve(ul)
	t.Cleanup(func() { srv.Close(); ul.Close() })

	client, err := NewRemoteClient(sockPath)
	if err != nil {
		t.Fatalf("NewRemoteClient: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if _, err := client.GetSyncStatus(ctx); !errors.Is(err, errEndpointNotFound) {
		t.Fatalf("GetSyncStatus 404: want errEndpointNotFound, got %v", err)
	}
	if _, err := client.GetSyncOutbox(ctx, ""); !errors.Is(err, errEndpointNotFound) {
		t.Fatalf("GetSyncOutbox 404: want errEndpointNotFound, got %v", err)
	}
	if _, err := client.GetSyncConflicts(ctx, ""); !errors.Is(err, errEndpointNotFound) {
		t.Fatalf("GetSyncConflicts 404: want errEndpointNotFound, got %v", err)
	}
}
