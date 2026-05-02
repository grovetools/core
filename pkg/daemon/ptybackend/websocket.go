package ptybackend

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	grovelogging "github.com/grovetools/core/logging"
)

type controlMessage struct {
	Type string `json:"type"`
	Rows uint16 `json:"rows,omitempty"`
	Cols uint16 `json:"cols,omitempty"`
	Code int    `json:"code,omitempty"`
}

// WebSocketBackend streams PTY I/O over a WebSocket connection to a daemon's
// /api/pty/attach/{id} endpoint. Binary frames carry raw PTY output/input;
// text frames carry JSON control messages (resize, exit).
type WebSocketBackend struct {
	sessionID  string
	socketPath string
	wsURL      string

	mu   sync.Mutex
	conn *websocket.Conn

	readMu     sync.Mutex
	currentMsg io.Reader

	closed        chan struct{}
	closeErr      error
	sessionExited bool
	exitCode      int
}

// NewWebSocketBackend dials the daemon PTY attach endpoint and returns a ready backend.
func NewWebSocketBackend(sessionID, socketPath string) (*WebSocketBackend, error) {
	ulog := grovelogging.NewUnifiedLogger("ptybackend")

	b := &WebSocketBackend{
		sessionID:  sessionID,
		socketPath: socketPath,
		wsURL:      "ws://unix/api/pty/attach/" + sessionID,
		closed:     make(chan struct{}),
	}

	if err := b.dial(); err != nil {
		return nil, err
	}

	ulog.Debug("Connected to daemon PTY").
		Field("pty_id", sessionID).
		StructuredOnly().Log(context.Background())

	return b, nil
}

func (b *WebSocketBackend) dial() error {
	dialer := websocket.Dialer{
		NetDial: func(_, _ string) (net.Conn, error) {
			return net.Dial("unix", b.socketPath)
		},
		HandshakeTimeout: 5 * time.Second,
	}

	conn, _, err := dialer.Dial(b.wsURL, nil)
	if err != nil {
		return fmt.Errorf("ws dial %s: %w", b.wsURL, err)
	}

	b.mu.Lock()
	b.conn = conn
	b.mu.Unlock()
	return nil
}

func (b *WebSocketBackend) Read(p []byte) (int, error) {
	b.readMu.Lock()
	defer b.readMu.Unlock()

	for {
		if b.currentMsg != nil {
			n, err := b.currentMsg.Read(p)
			if n > 0 {
				return n, nil
			}
			if err == io.EOF {
				b.currentMsg = nil
				continue
			}
			return 0, err
		}

		select {
		case <-b.closed:
			return 0, io.EOF
		default:
		}

		b.mu.Lock()
		conn := b.conn
		b.mu.Unlock()

		if conn == nil {
			if err := b.reconnect(); err != nil {
				return 0, err
			}
			continue
		}

		msgType, reader, err := conn.NextReader()
		if err != nil {
			if reconnErr := b.reconnect(); reconnErr != nil {
				return 0, reconnErr
			}
			continue
		}

		switch msgType {
		case websocket.BinaryMessage:
			b.currentMsg = reader
			continue
		case websocket.TextMessage:
			var ctrl controlMessage
			if decErr := json.NewDecoder(reader).Decode(&ctrl); decErr == nil && ctrl.Type == "exit" {
				b.sessionExited = true
				b.exitCode = ctrl.Code
				return 0, io.EOF
			}
			continue
		}
	}
}

func (b *WebSocketBackend) Write(p []byte) (int, error) {
	b.mu.Lock()
	conn := b.conn
	b.mu.Unlock()

	if conn == nil {
		return 0, fmt.Errorf("not connected")
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (b *WebSocketBackend) Close() error {
	select {
	case <-b.closed:
		return b.closeErr
	default:
	}
	close(b.closed)

	b.mu.Lock()
	conn := b.conn
	b.conn = nil
	b.mu.Unlock()

	if conn != nil {
		b.closeErr = conn.Close()
	}
	return b.closeErr
}

// Resize sends a resize control message to the daemon PTY.
func (b *WebSocketBackend) Resize(rows, cols uint16) error {
	msg := controlMessage{Type: "resize", Rows: rows, Cols: cols}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	b.mu.Lock()
	conn := b.conn
	b.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}

// Kill sends a POST /api/pty/kill/{id} request to terminate the daemon PTY session.
func (b *WebSocketBackend) Kill() error {
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", b.socketPath)
			},
		},
		Timeout: 5 * time.Second,
	}
	url := fmt.Sprintf("http://unix/api/pty/kill/%s", b.sessionID)
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("kill PTY %s: %w", b.sessionID, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("kill PTY %s: status %d", b.sessionID, resp.StatusCode)
	}
	return nil
}

func (b *WebSocketBackend) Name() string        { return b.sessionID }
func (b *WebSocketBackend) Pid() int            { return -1 }
func (b *WebSocketBackend) Fd() uintptr         { return ^uintptr(0) }
func (b *WebSocketBackend) SessionExited() bool { return b.sessionExited }
func (b *WebSocketBackend) ExitCode() int       { return b.exitCode }
func (b *WebSocketBackend) SessionID() string   { return b.sessionID }
func (b *WebSocketBackend) PtyID() string       { return b.sessionID }

func (b *WebSocketBackend) reconnect() error {
	b.mu.Lock()
	if b.conn != nil {
		b.conn.Close()
		b.conn = nil
	}
	b.mu.Unlock()

	backoff := 100 * time.Millisecond
	maxBackoff := 5 * time.Second

	for i := 0; i < 20; i++ {
		select {
		case <-b.closed:
			return io.EOF
		default:
		}

		time.Sleep(backoff)

		if err := b.dial(); err == nil {
			return nil
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
	return fmt.Errorf("failed to reconnect to daemon PTY %s after retries", b.sessionID)
}
