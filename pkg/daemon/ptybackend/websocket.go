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

	// lastRows/lastCols hold the most recent size requested via Resize,
	// whether or not the send succeeded. A resize sent while the socket is
	// down would otherwise be lost forever — the daemon PTY keeps its stale
	// (often creation-default) size and the client's terminal mirror
	// disagrees with it, rendering a shrunken/mangled pane. reconnect()
	// replays this size after every successful re-dial.
	lastRows uint16
	lastCols uint16

	readMu     sync.Mutex
	currentMsg io.Reader

	writeMu sync.Mutex // serializes websocket writes (gorilla forbids concurrent writes)

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
				grovelogging.NewUnifiedLogger("ptybackend.read").Warn("Read returning terminal error (nil-conn reconnect failed)").Field("pty_id", b.sessionID).Field("err", err.Error()).Field("session_exited", b.sessionExited).StructuredOnly().Log(context.Background())
				return 0, err
			}
			continue
		}

		msgType, reader, err := conn.NextReader()
		if err != nil {
			grovelogging.NewUnifiedLogger("ptybackend.read").Debug("NextReader error -> reconnecting").Field("pty_id", b.sessionID).Field("err", err.Error()).StructuredOnly().Log(context.Background())
			if reconnErr := b.reconnect(); reconnErr != nil {
				grovelogging.NewUnifiedLogger("ptybackend.read").Warn("Read returning terminal error (reconnect failed)").Field("pty_id", b.sessionID).Field("err", reconnErr.Error()).Field("session_exited", b.sessionExited).StructuredOnly().Log(context.Background())
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
				grovelogging.NewUnifiedLogger("ptybackend.read").Debug("Read got genuine exit control msg").Field("pty_id", b.sessionID).Field("code", ctrl.Code).StructuredOnly().Log(context.Background())
				return 0, io.EOF
			}
			continue
		}
	}
}

func (b *WebSocketBackend) Write(p []byte) (int, error) {
	b.writeMu.Lock()
	defer b.writeMu.Unlock()

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
	grovelogging.NewUnifiedLogger("ptybackend.close").Warn("Backend Close() called").Field("pty_id", b.sessionID).Field("session_exited", b.sessionExited).StructuredOnly().Log(context.Background())
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
	b.mu.Lock()
	b.lastRows, b.lastCols = rows, cols
	b.mu.Unlock()
	return b.sendResize(rows, cols)
}

func (b *WebSocketBackend) sendResize(rows, cols uint16) error {
	msg := controlMessage{Type: "resize", Rows: rows, Cols: cols}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	b.writeMu.Lock()
	defer b.writeMu.Unlock()

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
	ulog := grovelogging.NewUnifiedLogger("ptybackend.reconnect")
	ctx := context.Background()
	ulog.Debug("PTY reconnect started").Field("pty_id", b.sessionID).StructuredOnly().Log(ctx)

	b.mu.Lock()
	if b.conn != nil {
		b.conn.Close()
		b.conn = nil
	}
	b.mu.Unlock()

	backoff := 100 * time.Millisecond
	maxBackoff := 5 * time.Second

	// Retry indefinitely. A slow groved upgrade can take longer than the old
	// 20-attempt (~80s) budget to rebind; exhausting it surfaced a terminal
	// error to the read pump and tore down a still-live agent pane. The only
	// legitimate exits are an explicit Close (b.closed) or — handled before
	// reconnect is ever entered — a genuine session exit (sets sessionExited,
	// returns io.EOF). The exponential backoff is capped at maxBackoff, so this
	// just polls until the successor daemon answers. A periodic Warn keeps a
	// genuinely-stuck rebind visible in the workspace log.
	for i := 0; ; i++ {
		select {
		case <-b.closed:
			ulog.Debug("PTY reconnect aborted (backend closed)").Field("pty_id", b.sessionID).Field("attempt", i+1).StructuredOnly().Log(ctx)
			return io.EOF
		default:
		}

		time.Sleep(backoff)

		if err := b.dial(); err == nil {
			ulog.Debug("PTY reconnect SUCCEEDED").Field("pty_id", b.sessionID).Field("attempt", i+1).StructuredOnly().Log(ctx)
			// Replay the last requested size: any resize sent while the
			// socket was down was silently lost, and no host layer retries
			// an unchanged size on its own.
			b.mu.Lock()
			rows, cols := b.lastRows, b.lastCols
			b.mu.Unlock()
			if rows > 0 && cols > 0 {
				_ = b.sendResize(rows, cols)
			}
			return nil
		} else {
			ulog.Debug("PTY reconnect dial failed").Field("pty_id", b.sessionID).Field("attempt", i+1).Field("err", err.Error()).StructuredOnly().Log(ctx)
		}

		if i > 0 && (i+1)%20 == 0 {
			ulog.Warn("PTY reconnect still retrying (slow daemon rebind?)").Field("pty_id", b.sessionID).Field("attempts", i+1).StructuredOnly().Log(ctx)
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}
