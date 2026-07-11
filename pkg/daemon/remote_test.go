package daemon

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// shortTempSocket returns a short unix-socket path (macOS caps sun_path length,
// which t.TempDir() paths can exceed).
func shortTempSocket(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "remote")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "s")
}

// serveStreamUnix serves a one-shot /api/stream SSE endpoint on a unix socket.
// It emits a single data frame with update_type "test-marker" then blocks until
// the request context is cancelled.
func serveStreamUnix(t *testing.T, sockPath string) {
	t.Helper()
	ul, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen unix %s: %v", sockPath, err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "data: {\"update_type\":\"test-marker\"}\n\n")
		if fl, ok := w.(http.Flusher); ok {
			fl.Flush()
		}
		<-r.Context().Done()
	})
	srv := &http.Server{Handler: mux}
	go srv.Serve(ul)
	t.Cleanup(func() { srv.Close(); ul.Close() })
}

// TestStreamStateOverInjectedDialer proves the c.dial seam reaches the SSE
// stream transports (not just NewRemoteClient's request client): a client built
// via NewRemoteClientWithDialer streams over the injected dialer, and the
// dialer's call counter increments — i.e. StreamState was refactored to route
// through newStreamTransport().
func TestStreamStateOverInjectedDialer(t *testing.T) {
	sockPath := shortTempSocket(t)
	serveStreamUnix(t, sockPath)

	var dialCount atomic.Int32
	dial := func(ctx context.Context) (net.Conn, error) {
		dialCount.Add(1)
		var d net.Dialer
		return d.DialContext(ctx, "unix", sockPath)
	}

	client, err := NewRemoteClientWithDialer(dial)
	if err != nil {
		t.Fatalf("NewRemoteClientWithDialer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := client.StreamState(ctx)
	if err != nil {
		t.Fatalf("StreamState: %v", err)
	}

	select {
	case u := <-ch:
		if u.UpdateType != "test-marker" {
			t.Fatalf("unexpected update_type: got %q want %q", u.UpdateType, "test-marker")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no update received over injected dialer")
	}

	if n := dialCount.Load(); n == 0 {
		t.Fatalf("injected dialer was never used — StreamState did not route through c.dial")
	}
}

// TestNewRemoteClientSeededDialer is the behavior-preservation check: the
// default NewRemoteClient(socketPath) construction still dials its unix socket
// through the seeded closure and streams state.
func TestNewRemoteClientSeededDialer(t *testing.T) {
	sockPath := shortTempSocket(t)
	serveStreamUnix(t, sockPath)

	client, err := NewRemoteClient(sockPath)
	if err != nil {
		t.Fatalf("NewRemoteClient: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := client.StreamState(ctx)
	if err != nil {
		t.Fatalf("StreamState: %v", err)
	}

	select {
	case u := <-ch:
		if u.UpdateType != "test-marker" {
			t.Fatalf("unexpected update_type: got %q want %q", u.UpdateType, "test-marker")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no update received over seeded unix dialer")
	}
}
