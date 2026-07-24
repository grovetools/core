package ptybackend

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestReplaySafeBackendSkipsHistoryAndNudgesRedrawOnAttachAndReconnect(t *testing.T) {
	// Unix socket paths are short (104 bytes on macOS); testing.TempDir can
	// exceed that once the test name is included.
	socketPath := fmt.Sprintf("/tmp/ptybackend-%d-%d.sock", os.Getpid(), time.Now().UnixNano())
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	defer os.Remove(socketPath)

	queryCh := make(chan string, 1)
	resizeCh := make(chan controlMessage, 2)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	httpServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queryCh <- r.URL.Query().Get("history")
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for i := 0; i < 2; i++ {
			typ, data, err := conn.ReadMessage()
			if err != nil || typ != websocket.TextMessage {
				return
			}
			var msg controlMessage
			if json.Unmarshal(data, &msg) == nil {
				resizeCh <- msg
			}
		}
	})}
	go func() { _ = httpServer.Serve(listener) }()
	defer httpServer.Close()

	backend, err := NewReplaySafeWebSocketBackend("agent-pty", socketPath)
	if err != nil {
		t.Fatalf("new backend: %v", err)
	}
	defer backend.Close()

	select {
	case mode := <-queryCh:
		if mode != "none" {
			t.Fatalf("history mode = %q, want none", mode)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not observe attachment")
	}

	if err := backend.Resize(40, 120); err != nil {
		t.Fatalf("resize: %v", err)
	}
	assertRedraw := func(label string) {
		t.Helper()
		for i, want := range []uint16{39, 40} {
			select {
			case msg := <-resizeCh:
				if msg.Type != "resize" || msg.Rows != want || msg.Cols != 120 {
					t.Fatalf("%s resize %d = %+v, want rows=%d cols=120", label, i, msg, want)
				}
			case <-time.After(2 * time.Second):
				t.Fatalf("timed out waiting for %s resize %d", label, i)
			}
		}
	}
	assertRedraw("attach")

	// The test server closes after the first redraw. Reading notices that close,
	// reconnects with the same replay-free URL, and redraws at the remembered
	// size so output missed during the disconnect cannot leave a stale screen.
	readDone := make(chan error, 1)
	go func() {
		var buf [1]byte
		_, err := backend.Read(buf[:])
		readDone <- err
	}()
	select {
	case mode := <-queryCh:
		if mode != "none" {
			t.Fatalf("reconnect history mode = %q, want none", mode)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not observe reconnect")
	}
	assertRedraw("reconnect")
}
