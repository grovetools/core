package daemon

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/grovetools/core/pkg/models"
)

// TestGetPTYResources decodes a fixture /api/pty/resources payload over a
// unix socket, proving the client hits the proxied path, forwards the
// detail/history flags, and lands every snake_case field in the model.
func TestGetPTYResources(t *testing.T) {
	sockPath := shortTempSocket(t)
	ul, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	gotQuery := make(chan string, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/pty/resources", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		gotQuery <- r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"sampled_at": "2026-07-28T12:00:00Z",
			"cadence_ms": 30000,
			"host": {"pid": 4242, "cpu_pct": 2.1, "rss_kb": 471040, "goroutines": 88},
			"sessions": [
				{"pty_id": "e6f0469", "root_pid": 52731, "workspace": "compositor",
				 "label": "Editor", "labels": {"job_id": "fix-nav-clamp"},
				 "attached_clients": 1, "cpu_pct": 12.4, "rss_kb": 890000, "procs": 3,
				 "top": {"pid": 52768, "comm": "gopls", "cpu_pct": 9.0, "rss_kb": 640000},
				 "procs_detail": [{"pid": 52731, "comm": "nvim", "cpu_pct": 3.1, "rss_kb": 180000}],
				 "history": {"cpu": [1.0, 2.0], "rss_kb": [100, 200]},
				 "last_detached": "2026-07-28T10:00:00Z"}
			],
			"orphans": [
				{"pid": 61102, "comm": "nvim", "cpu_pct": 0, "rss_kb": 210000, "procs": 2,
				 "reason": "dead-pty", "pty_id": "9af31c", "detached_at": "2026-07-28T09:00:00Z"},
				{"pid": 900, "comm": "gopls", "cpu_pct": 1.5, "rss_kb": 10, "reason": "unaccounted"}
			]
		}`)
	})
	srv := &http.Server{Handler: mux}
	go srv.Serve(ul)
	t.Cleanup(func() { srv.Close(); ul.Close() })

	client, err := NewRemoteClient(sockPath)
	if err != nil {
		t.Fatalf("NewRemoteClient: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	res, err := client.GetPTYResources(ctx, PTYResourcesOptions{Detail: true, History: true})
	if err != nil {
		t.Fatalf("GetPTYResources: %v", err)
	}
	if q := <-gotQuery; q != "detail=1&history=1" {
		t.Errorf("query = %q, want detail=1&history=1", q)
	}
	if res.CadenceMS != 30000 || res.SampledAt.IsZero() {
		t.Errorf("envelope mis-decoded: cadence=%d at=%v", res.CadenceMS, res.SampledAt)
	}
	if res.Host.PID != 4242 || res.Host.CPUPct != 2.1 || res.Host.RSSKB != 471040 || res.Host.Goroutines != 88 {
		t.Errorf("host mis-decoded: %+v", res.Host)
	}
	if len(res.Sessions) != 1 {
		t.Fatalf("sessions = %d", len(res.Sessions))
	}
	s := res.Sessions[0]
	if s.PTYID != "e6f0469" || s.RootPID != 52731 || s.Workspace != "compositor" ||
		s.Label != "Editor" || s.Labels["job_id"] != "fix-nav-clamp" || s.AttachedClients != 1 {
		t.Errorf("session identity mis-decoded: %+v", s)
	}
	if s.CPUPct != 12.4 || s.RSSKB != 890000 || s.Procs != 3 {
		t.Errorf("session rollup mis-decoded: %+v", s)
	}
	if s.Top == nil || s.Top.Comm != "gopls" {
		t.Errorf("session top mis-decoded: %+v", s.Top)
	}
	if len(s.ProcsDetail) != 1 || s.ProcsDetail[0].Comm != "nvim" {
		t.Errorf("procs_detail mis-decoded: %+v", s.ProcsDetail)
	}
	if s.History == nil || len(s.History.CPU) != 2 || len(s.History.RSSKB) != 2 {
		t.Errorf("history mis-decoded: %+v", s.History)
	}
	if s.LastDetached == nil {
		t.Errorf("last_detached mis-decoded")
	}
	if len(res.Orphans) != 2 {
		t.Fatalf("orphans = %d", len(res.Orphans))
	}
	if res.Orphans[0].Reason != models.OrphanReasonDeadPTY || res.Orphans[0].PTYID != "9af31c" ||
		res.Orphans[0].DetachedAt == nil {
		t.Errorf("dead-pty orphan mis-decoded: %+v", res.Orphans[0])
	}
	if res.Orphans[1].Reason != "unaccounted" {
		t.Errorf("unaccounted orphan mis-decoded: %+v", res.Orphans[1])
	}
}

// TestGetPTYResourcesOptionsQuery pins the query rendering, including the
// no-flags case (a bare summary request must not carry a "?").
func TestGetPTYResourcesOptionsQuery(t *testing.T) {
	cases := []struct {
		opts PTYResourcesOptions
		want string
	}{
		{PTYResourcesOptions{}, ""},
		{PTYResourcesOptions{Detail: true}, "detail=1"},
		{PTYResourcesOptions{History: true}, "history=1"},
		{PTYResourcesOptions{Detail: true, History: true}, "detail=1&history=1"},
	}
	for _, c := range cases {
		if got := c.opts.query(); got != c.want {
			t.Errorf("%+v.query() = %q, want %q", c.opts, got, c.want)
		}
	}
}

// TestGetPTYResources404 proves a tuimuxd predating the endpoint surfaces
// errEndpointNotFound, which is the inspector's signal to keep sampling
// client-side instead of showing an error.
func TestGetPTYResources404(t *testing.T) {
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

	_, err = client.GetPTYResources(ctx, PTYResourcesOptions{})
	if !errors.Is(err, errEndpointNotFound) {
		t.Fatalf("want errEndpointNotFound, got %v", err)
	}
	if !IsEndpointNotFound(err) {
		t.Fatalf("IsEndpointNotFound(%v) = false, want true", err)
	}
}

// TestLocalClientGetPTYResourcesNotSupported pins the daemon-less stub: there
// is no PTY daemon to attribute, so callers get the standard fallback signal.
func TestLocalClientGetPTYResourcesNotSupported(t *testing.T) {
	_, err := (&LocalClient{}).GetPTYResources(context.Background(), PTYResourcesOptions{})
	if !errors.Is(err, ErrNotSupported) {
		t.Fatalf("want ErrNotSupported, got %v", err)
	}
}
