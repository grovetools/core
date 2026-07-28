package daemon

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"
)

// TestGetSystemStats decodes a fixture /api/system/stats payload over a unix
// socket, proving the client hits the right path and the snake_case fields
// land in the model.
func TestGetSystemStats(t *testing.T) {
	sockPath := shortTempSocket(t)
	ul, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/system/stats", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"sampled_at": "2026-07-27T12:00:00Z",
			"runtime": {"goroutines": 412, "heap_alloc": 134217728, "heap_sys": 268435456,
			            "num_gc": 184, "gc_pause_total_ms": 1200.5, "gomemlimit": 2147483648,
			            "uptime_ms": 3600000},
			"self": {"pid": 27462, "cpu_pct": 41.0, "rss_kb": 727040, "procs": 63,
			         "top": {"pid": 999, "comm": "git", "cpu_pct": 12.5, "rss_kb": 40960},
			         "children": [{"pid": 999, "comm": "git", "cpu_pct": 12.5, "rss_kb": 40960}]},
			"counters": {},
			"warnings": []
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

	stats, err := client.GetSystemStats(ctx)
	if err != nil {
		t.Fatalf("GetSystemStats: %v", err)
	}
	if stats.Runtime.Goroutines != 412 || stats.Runtime.NumGC != 184 ||
		stats.Runtime.GCPauseTotalMS != 1200.5 || stats.Runtime.GoMemLimit != 2147483648 {
		t.Errorf("runtime mis-decoded: %+v", stats.Runtime)
	}
	if stats.Self.PID != 27462 || stats.Self.CPUPct != 41.0 || stats.Self.Procs != 63 {
		t.Errorf("self mis-decoded: %+v", stats.Self)
	}
	if stats.Self.Top == nil || stats.Self.Top.Comm != "git" {
		t.Errorf("self.top mis-decoded: %+v", stats.Self.Top)
	}
	if len(stats.Self.Children) != 1 || stats.Self.Children[0].PID != 999 {
		t.Errorf("self.children mis-decoded: %+v", stats.Self.Children)
	}
	if stats.Counters == nil || stats.Warnings == nil {
		t.Errorf("reserved fields decoded nil: counters=%v warnings=%v", stats.Counters, stats.Warnings)
	}
}

// TestGetSystemStats404 proves a stale daemon (no /api/system/stats route)
// surfaces errEndpointNotFound, and that the exported IsEndpointNotFound
// predicate recognizes it (the `groved stats` CLI depends on this).
func TestGetSystemStats404(t *testing.T) {
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

	_, err = client.GetSystemStats(ctx)
	if !errors.Is(err, errEndpointNotFound) {
		t.Fatalf("want errEndpointNotFound, got %v", err)
	}
	if !IsEndpointNotFound(err) {
		t.Fatalf("IsEndpointNotFound(%v) = false, want true", err)
	}
}
