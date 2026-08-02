package daemon

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/grovetools/core/pkg/forge"
	"github.com/grovetools/core/pkg/models"
)

// serveForgeUnix serves a fake GET /api/forge/state on a unix socket, echoing
// the daemon's wire shape (daemon/internal/daemon/server/forge.go). body is
// written verbatim; an empty body makes the route answer 404 so the
// old-daemon path can be exercised.
func serveForgeUnix(t *testing.T, sockPath, body string) {
	t.Helper()
	ul, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen unix %s: %v", sockPath, err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/forge/state", func(w http.ResponseWriter, r *http.Request) {
		if body == "" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ul) }()
	t.Cleanup(func() { _ = srv.Close(); _ = ul.Close() })
}

// TestGetForgeStateDecodesWirePayload proves the client decodes the daemon's
// forge snapshot, including the fields that date it and the check rollups.
func TestGetForgeStateDecodesWirePayload(t *testing.T) {
	sockPath := shortTempSocket(t)
	serveForgeUnix(t, sockPath, `{
		"enabled": true,
		"provider": "github",
		"repos": [{
			"provider": "github",
			"repo": "github.com/acme/flow",
			"state": "stale",
			"fetched_at": "2026-08-02T10:00:00Z",
			"last_attempt_at": "2026-08-02T11:55:00Z",
			"last_error": "gh: rate limited",
			"prs": [{"number": 7, "title": "t", "state": "open", "head_sha": "abc"}],
			"checks": {"7": {"ref": "abc", "state": "failure"}}
		}]
	}`)

	client, err := NewRemoteClient(sockPath)
	if err != nil {
		t.Fatalf("NewRemoteClient: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	snap, err := client.GetForgeState(ctx)
	if err != nil {
		t.Fatalf("GetForgeState: %v", err)
	}
	if !snap.Enabled || snap.Provider != "github" || len(snap.Repos) != 1 {
		t.Fatalf("headline fields mis-decoded: %+v", snap)
	}
	repo := snap.Repos[0]
	if repo.State != models.ForgeStateStale {
		t.Errorf("state = %q, want stale", repo.State)
	}
	if repo.LastError == "" {
		t.Error("LastError must survive the wire — it is why the entry is stale")
	}
	// The gap between these two is how a surface reports "failing since"; a
	// client that dropped either would render a failing repo as merely old.
	if repo.FetchedAt.IsZero() || repo.LastAttemptAt.IsZero() {
		t.Errorf("both timestamps must decode, got fetched=%v attempt=%v", repo.FetchedAt, repo.LastAttemptAt)
	}
	if len(repo.PRs) != 1 || repo.PRs[0].Number != 7 || repo.PRs[0].State != forge.PRStateOpen {
		t.Errorf("PRs mis-decoded: %+v", repo.PRs)
	}
	if repo.Checks[7].State != forge.CheckStateFailure {
		t.Errorf("check rollup mis-decoded: %+v", repo.Checks)
	}
}

// TestGetForgeStateDisabledIsNotEmpty proves the client preserves the
// distinction the endpoint exists to carry: a daemon that answers
// enabled=false is telling the caller nothing is known, which is not the same
// as an empty repo list from a running poller.
func TestGetForgeStateDisabledIsNotEmpty(t *testing.T) {
	sockPath := shortTempSocket(t)
	serveForgeUnix(t, sockPath, `{"enabled": false}`)

	client, err := NewRemoteClient(sockPath)
	if err != nil {
		t.Fatalf("NewRemoteClient: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	snap, err := client.GetForgeState(ctx)
	if err != nil {
		t.Fatalf("GetForgeState: %v", err)
	}
	if snap.Enabled {
		t.Fatal("Enabled must round-trip as false")
	}
	if len(snap.Repos) != 0 {
		t.Errorf("a disabled snapshot carries no repos, got %d", len(snap.Repos))
	}
}

// TestGetForgeState404 proves a daemon predating the endpoint yields
// errEndpointNotFound, so a caller can say "forge state unavailable" instead
// of showing an empty list.
func TestGetForgeState404(t *testing.T) {
	sockPath := shortTempSocket(t)
	serveForgeUnix(t, sockPath, "")

	client, err := NewRemoteClient(sockPath)
	if err != nil {
		t.Fatalf("NewRemoteClient: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if _, err := client.GetForgeState(ctx); !errors.Is(err, errEndpointNotFound) {
		t.Fatalf("GetForgeState 404: want errEndpointNotFound, got %v", err)
	}
}

// TestLocalClientGetForgeStateNotSupported proves the daemonless client refuses
// rather than inventing an answer: without groved there is no poller and no
// cache, and an empty snapshot would read as "no pull requests".
func TestLocalClientGetForgeStateNotSupported(t *testing.T) {
	var c LocalClient
	if _, err := c.GetForgeState(context.Background()); !errors.Is(err, ErrNotSupported) {
		t.Fatalf("want ErrNotSupported, got %v", err)
	}
}
