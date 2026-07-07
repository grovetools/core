package mux

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeTuimuxServer starts an httptest server bound to a unix socket that
// implements the subset of the tuimux /api/pty surface the RecordClient uses.
// It returns the socket path; the server is torn down via t.Cleanup. The socket
// lives under a short /tmp dir because macOS caps unix socket paths at ~104
// bytes — t.TempDir() paths are too long.
func fakeTuimuxServer(t *testing.T, h http.Handler) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "grrec")
	if err != nil {
		t.Fatalf("mkdtemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "d.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	srv := httptest.NewUnstartedServer(h)
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	return sock
}

func TestListPTYs(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/pty/list", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]PtyInfo{
			{ID: "pty-1", PID: 100, ForegroundProcess: "nvim", Tags: map[string]string{"label": "editor"}},
			{ID: "pty-2", PID: 200, Recording: true, RecordingPath: "/tmp/x.cast"},
		})
	})
	client := NewRecordClient(fakeTuimuxServer(t, mux))

	ptys, err := client.ListPTYs()
	if err != nil {
		t.Fatalf("ListPTYs: %v", err)
	}
	if len(ptys) != 2 {
		t.Fatalf("want 2 ptys, got %d", len(ptys))
	}
	if ptys[0].ID != "pty-1" || ptys[0].Tags["label"] != "editor" {
		t.Errorf("pty-1 decode wrong: %+v", ptys[0])
	}
	if !ptys[1].Recording || ptys[1].RecordingPath != "/tmp/x.cast" {
		t.Errorf("pty-2 recording fields wrong: %+v", ptys[1])
	}
}

func TestStartRecordingStatusMapping(t *testing.T) {
	cases := []struct {
		id      string
		status  int
		wantErr error
	}{
		{"ok", http.StatusCreated, nil},
		{"missing", http.StatusNotFound, ErrPtyNotFound},
		{"busy", http.StatusConflict, ErrAlreadyRecording},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/pty/record/start/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/pty/record/start/")
		switch id {
		case "ok":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "recording", "path": "/tmp/ok.cast"})
		case "missing":
			http.Error(w, "session not found", http.StatusNotFound)
		case "busy":
			http.Error(w, "already recording", http.StatusConflict)
		}
	})
	client := NewRecordClient(fakeTuimuxServer(t, mux))

	for _, tc := range cases {
		err := client.StartRecording(tc.id, RecordStartRequest{Path: "/tmp/ok.cast", Version: 3})
		if !errors.Is(err, tc.wantErr) {
			t.Errorf("StartRecording(%s): got %v, want %v", tc.id, err, tc.wantErr)
		}
	}
}

func TestStopRecordingStatusMapping(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/pty/record/stop/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/pty/record/stop/")
		switch id {
		case "ok":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(RecordStopResult{Path: "/tmp/ok.cast", Duration: 12.34, Events: 57, Bytes: 84213})
		case "missing":
			http.Error(w, "session not found", http.StatusNotFound)
		case "notrec":
			http.Error(w, "not recording", http.StatusConflict)
		}
	})
	client := NewRecordClient(fakeTuimuxServer(t, mux))

	info, err := client.StopRecording("ok")
	if err != nil {
		t.Fatalf("StopRecording(ok): %v", err)
	}
	if info.Duration != 12.34 || info.Events != 57 || info.Bytes != 84213 || info.Path != "/tmp/ok.cast" {
		t.Errorf("stop result decode wrong: %+v", info)
	}

	if _, err := client.StopRecording("missing"); !errors.Is(err, ErrPtyNotFound) {
		t.Errorf("StopRecording(missing): got %v, want ErrPtyNotFound", err)
	}
	if _, err := client.StopRecording("notrec"); !errors.Is(err, ErrNotRecording) {
		t.Errorf("StopRecording(notrec): got %v, want ErrNotRecording", err)
	}
}

func TestResolveRecordSocketExplicitSocketOverride(t *testing.T) {
	t.Setenv(EnvGroveTuimuxSocket, "/custom/daemon.sock")
	if got := ResolveRecordSocket(""); got != "/custom/daemon.sock" {
		t.Errorf("ResolveRecordSocket() with GROVE_TUIMUX_SOCKET = %q, want /custom/daemon.sock", got)
	}
}
