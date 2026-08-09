package ptybackend

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// resizeEvent is a control message plus the moment the server saw it. The
// timestamp is the point of the redraw contract: the nudge only works if the
// two sizes reach the child far enough apart to be observed as two sizes.
type resizeEvent struct {
	msg controlMessage
	at  time.Time
}

// startTestDaemon serves the PTY attach endpoint on a unix socket. handle runs
// once per connection with its 0-based index, so a test can close the first
// connection and assert on what the client does when it comes back.
func startTestDaemon(t *testing.T, handle func(conn *websocket.Conn, idx int)) (socketPath string, queries chan string) {
	t.Helper()

	// Unix socket paths are short (104 bytes on macOS); testing.TempDir can
	// exceed that once the test name is included.
	socketPath = fmt.Sprintf("/tmp/ptybackend-%d-%d.sock", os.Getpid(), time.Now().UnixNano())
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	queries = make(chan string, 8)
	var idx atomic.Int64
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	httpServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries <- r.URL.Query().Get("history")
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		handle(conn, int(idx.Add(1)-1))
	})}
	go func() { _ = httpServer.Serve(listener) }()
	t.Cleanup(func() {
		_ = httpServer.Close()
		_ = listener.Close()
		_ = os.Remove(socketPath)
	})
	return socketPath, queries
}

// readResize returns the next resize control message the client sends.
func readResize(conn *websocket.Conn) (resizeEvent, bool) {
	for {
		typ, data, err := conn.ReadMessage()
		if err != nil {
			return resizeEvent{}, false
		}
		if typ != websocket.TextMessage {
			continue
		}
		var msg controlMessage
		if json.Unmarshal(data, &msg) != nil || msg.Type != "resize" {
			continue
		}
		return resizeEvent{msg: msg, at: time.Now()}, true
	}
}

// drainInput keeps reading (and discarding) client frames so the connection
// stays open.
func drainInput(conn *websocket.Conn) {
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

// pumpReads stands in for the host's readPTYLoop: something must consume PTY
// output for the backend to know its nudge provoked a repaint.
func pumpReads(b *WebSocketBackend) {
	go func() {
		buf := make([]byte, 256)
		for {
			if _, err := b.Read(buf); err != nil {
				return
			}
		}
	}()
}

func expectRedrawPair(t *testing.T, resizes chan resizeEvent, label string, wantRows uint16) {
	t.Helper()
	var seen [2]resizeEvent
	for i, wantRow := range [2]uint16{wantRows - 1, wantRows} {
		select {
		case evt := <-resizes:
			if evt.msg.Rows != wantRow || evt.msg.Cols != 120 {
				t.Fatalf("%s resize %d = %+v, want rows=%d cols=120", label, i, evt.msg, wantRow)
			}
			seen[i] = evt
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out waiting for %s resize %d", label, i)
		}
	}
	// The whole point of the pair: two ioctls sent back to back coalesce into a
	// single SIGWINCH reporting the unchanged final size, which a differential
	// renderer answers with silence — the blank agent pane after a restart.
	if gap := seen[1].at.Sub(seen[0].at); gap < redrawNudgeGap {
		t.Fatalf("%s redraw gap = %v, want >= %v (nudge would coalesce)", label, gap, redrawNudgeGap)
	}
}

func TestReplaySafeBackendSkipsHistoryAndNudgesRedrawOnAttachAndReconnect(t *testing.T) {
	resizes := make(chan resizeEvent, 16)
	socketPath, queries := startTestDaemon(t, func(conn *websocket.Conn, idx int) {
		for i := 0; i < 2; i++ {
			evt, ok := readResize(conn)
			if !ok {
				return
			}
			resizes <- evt
		}
		// Answer the redraw the way a live agent does, so the driver sees the
		// repaint it asked for and stops.
		_ = conn.WriteMessage(websocket.BinaryMessage, []byte("repainted"))
		if idx == 0 {
			// Close, so the client notices, reconnects replay-free, and redraws
			// again — output missed while disconnected is unrecoverable
			// otherwise.
			return
		}
		drainInput(conn)
	})

	backend, err := NewReplaySafeWebSocketBackend("agent-pty", socketPath)
	if err != nil {
		t.Fatalf("new backend: %v", err)
	}
	defer backend.Close()
	pumpReads(backend)

	assertHistoryMode := func(label string) {
		t.Helper()
		select {
		case mode := <-queries:
			if mode != "none" {
				t.Fatalf("%s history mode = %q, want none", label, mode)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("server did not observe %s", label)
		}
	}
	assertHistoryMode("attachment")

	if err := backend.Resize(40, 120); err != nil {
		t.Fatalf("resize: %v", err)
	}
	expectRedrawPair(t, resizes, "attach", 40)

	assertHistoryMode("reconnect")
	expectRedrawPair(t, resizes, "reconnect", 40)
}

// A nudge the child ignores leaves the pane exactly as blank as no nudge at
// all, so the driver retries until output shows up — bounded, because a pane
// that answers none of them is wedged rather than slow.
func TestRedrawRetriesWhileNoOutputArrivesThenGivesUp(t *testing.T) {
	resizes := make(chan resizeEvent, 32)
	socketPath, _ := startTestDaemon(t, func(conn *websocket.Conn, _ int) {
		for {
			evt, ok := readResize(conn)
			if !ok {
				return
			}
			resizes <- evt
		}
	})

	backend, err := NewReplaySafeWebSocketBackend("agent-pty", socketPath)
	if err != nil {
		t.Fatalf("new backend: %v", err)
	}
	defer backend.Close()

	if err := backend.Resize(40, 120); err != nil {
		t.Fatalf("resize: %v", err)
	}
	for attempt := 0; attempt < redrawAttempts; attempt++ {
		expectRedrawPair(t, resizes, fmt.Sprintf("attempt %d", attempt), 40)
	}

	select {
	case evt := <-resizes:
		t.Fatalf("resize after %d attempts = %+v, want none", redrawAttempts, evt.msg)
	case <-time.After(redrawNudgeGap + redrawVerifyWindow + 200*time.Millisecond):
	}
}

// A relayout that lands mid-jiggle must win: the restore uses the size most
// recently asked for, not the one the driver started with.
func TestRedrawRestoresTheLatestRequestedSize(t *testing.T) {
	resizes := make(chan resizeEvent, 16)
	socketPath, _ := startTestDaemon(t, func(conn *websocket.Conn, _ int) {
		for {
			evt, ok := readResize(conn)
			if !ok {
				return
			}
			resizes <- evt
		}
	})

	backend, err := NewReplaySafeWebSocketBackend("agent-pty", socketPath)
	if err != nil {
		t.Fatalf("new backend: %v", err)
	}
	defer backend.Close()

	if err := backend.Resize(40, 120); err != nil {
		t.Fatalf("resize: %v", err)
	}
	select {
	case evt := <-resizes:
		if evt.msg.Rows != 39 {
			t.Fatalf("nudge = %+v, want rows=39", evt.msg)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for nudge")
	}

	// Land a real relayout inside the gap.
	if err := backend.Resize(30, 100); err != nil {
		t.Fatalf("relayout resize: %v", err)
	}
	select {
	case evt := <-resizes:
		if evt.msg.Rows != 30 || evt.msg.Cols != 100 {
			t.Fatalf("relayout = %+v, want rows=30 cols=100", evt.msg)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for relayout resize")
	}
	select {
	case evt := <-resizes:
		if evt.msg.Rows != 30 || evt.msg.Cols != 100 {
			t.Fatalf("restore = %+v, want the relayout size rows=30 cols=100", evt.msg)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for redraw restore")
	}
}

// Shells keep raw history and must never be jiggled: a plain resize is one
// message, at the size asked for.
func TestRawHistoryBackendResizesWithoutNudging(t *testing.T) {
	resizes := make(chan resizeEvent, 8)
	socketPath, queries := startTestDaemon(t, func(conn *websocket.Conn, _ int) {
		for {
			evt, ok := readResize(conn)
			if !ok {
				return
			}
			resizes <- evt
		}
	})

	backend, err := NewWebSocketBackend("shell-pty", socketPath)
	if err != nil {
		t.Fatalf("new backend: %v", err)
	}
	defer backend.Close()

	if mode := <-queries; mode != "" {
		t.Fatalf("history mode = %q, want the raw default", mode)
	}
	if err := backend.Resize(40, 120); err != nil {
		t.Fatalf("resize: %v", err)
	}
	select {
	case evt := <-resizes:
		if evt.msg.Rows != 40 || evt.msg.Cols != 120 {
			t.Fatalf("resize = %+v, want rows=40 cols=120", evt.msg)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for resize")
	}
	select {
	case evt := <-resizes:
		t.Fatalf("second resize = %+v, want none (no nudge for raw history)", evt.msg)
	case <-time.After(redrawNudgeGap + 200*time.Millisecond):
	}
}
