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
			"server": "https://sync.example.com",
			"documents": 42,
			"documents_diverged": 1,
			"outbox_pending": 3,
			"outbox_parked": 2,
			"notespaces": [{
				"notespace_id": "01ARZ3NDEKTSV4RRFFQ69G5FAV",
				"notespace_name": "notes",
				"cursor": 137,
				"last_synced_at": "2026-07-12T10:00:00Z",
				"pull": true,
				"mode": "full",
				"role": "peer",
				"hydration": {
					"notespace": "01ARZ3NDEKTSV4RRFFQ69G5FAV",
					"root": "/Users/x/notebooks/nb/notespaces/notes",
					"running": true,
					"scanned": 500,
					"enqueued": 12,
					"quarantined": 1,
					"started_at": "2026-07-12T09:59:00Z",
					"files_per_sec": 83.5
				}
			}, {
				"notespace_id": "01ARZ3NDEKTSV4RRFFQ69G5FAW",
				"notespace_name": "wiki",
				"cursor": 4,
				"mode": "search-only"
			}]
		}`))
	})
	mux.HandleFunc("/api/sync/outbox", func(w http.ResponseWriter, r *http.Request) {
		*lastWorkspace = r.URL.Query().Get("notespace_id")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{
			"id": 7,
			"document_id": "doc-1",
			"notespace_id": "01ARZ3NDEKTSV4RRFFQ69G5FAV",
			"notespace_name": "notes",
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
		*lastWorkspace = r.URL.Query().Get("notespace_id")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{
			"notespace_id": "01ARZ3NDEKTSV4RRFFQ69G5FAV",
			"notespace_name": "notes",
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
		st.DocumentsDiverged != 1 || st.OutboxPending != 3 || st.OutboxParked != 2 ||
		st.Server != "https://sync.example.com" {
		t.Fatalf("headline fields mis-decoded: %+v", st)
	}
	if len(st.Notespaces) != 2 {
		t.Fatalf("want 2 notespaces, got %d", len(st.Notespaces))
	}
	w := st.Notespaces[0]
	// Identity is the stamp id, with the display name beside it. Decoding only
	// one of the two is how this client went blind: the daemon renamed the keys
	// with the notespace-identity work and every per-notespace row silently
	// stopped arriving.
	if w.NotespaceID != "01ARZ3NDEKTSV4RRFFQ69G5FAV" || w.NotespaceName != "notes" ||
		w.Cursor != 137 || w.LastSyncedAt.IsZero() {
		t.Fatalf("notespace fields mis-decoded: %+v", w)
	}
	// Direction: an explicit pull on the first entry, the push-only default
	// (an absent "pull" key) plus a filtered mode on the second.
	if !w.Pull || w.Mode != "full" || w.Role != "peer" {
		t.Fatalf("direction fields mis-decoded: %+v", w)
	}
	// The second entry is LEGACY — no role key at all — and must decode as an
	// empty role rather than borrowing the first entry's.
	if w2 := st.Notespaces[1]; w2.Pull || w2.Mode != "search-only" || w2.Role != "" {
		t.Fatalf("push-only notespace mis-decoded: %+v", w2)
	}
	if w.Hydration == nil || !w.Hydration.Running || w.Hydration.Scanned != 500 ||
		w.Hydration.Enqueued != 12 || w.Hydration.Quarantined != 1 || w.Hydration.FilesPerSec != 83.5 {
		t.Fatalf("hydration fields mis-decoded: %+v", w.Hydration)
	}
	// Root answers "which tree did those counters count", which is the whole
	// reason a wrong-root hydration is diagnosable at all.
	if w.Hydration.Notespace != "01ARZ3NDEKTSV4RRFFQ69G5FAV" ||
		w.Hydration.Root != "/Users/x/notebooks/nb/notespaces/notes" {
		t.Fatalf("hydration identity mis-decoded: %+v", w.Hydration)
	}
}

// TestGetSyncOutboxDecodesAndFilters proves GetSyncOutbox decodes parked
// metadata and forwards the notespace_id query parameter — by id, which is the
// only key the daemon's filter reads.
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

	entries, err := client.GetSyncOutbox(ctx, "01ARZ3NDEKTSV4RRFFQ69G5FAV")
	if err != nil {
		t.Fatalf("GetSyncOutbox: %v", err)
	}
	if ws != "01ARZ3NDEKTSV4RRFFQ69G5FAV" {
		t.Fatalf("notespace_id filter not forwarded: got %q", ws)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.ID != 7 || e.Path != "inbox/todo.md" || !e.Parked || e.Attempts != 4 ||
		e.ParkReason != "secret_quarantine" || e.NextRetryAt.IsZero() {
		t.Fatalf("outbox fields mis-decoded: %+v", e)
	}
	if e.NotespaceID != "01ARZ3NDEKTSV4RRFFQ69G5FAV" || e.NotespaceName != "notes" {
		t.Fatalf("outbox identity mis-decoded: %+v", e)
	}
}

// TestGetSyncConflictsDecodes proves GetSyncConflicts decodes the artifact
// payload; the empty notespace arg must not emit a query parameter.
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
		t.Fatalf("unexpected notespace_id filter: got %q", ws)
	}
	if len(conflicts) != 1 {
		t.Fatalf("want 1 conflict, got %d", len(conflicts))
	}
	c := conflicts[0]
	if c.NotespaceID != "01ARZ3NDEKTSV4RRFFQ69G5FAV" || c.NotespaceName != "notes" ||
		c.Path != "plans/roadmap.md" || c.DocumentID != "doc-9" ||
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
