// Package server provides the HTTP server for the grove daemon.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/grovetools/core/internal/daemon/engine"
	"github.com/grovetools/core/internal/daemon/store"
	"github.com/grovetools/core/pkg/enrichment"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// Server manages the daemon's HTTP server over a Unix socket.
type Server struct {
	logger *logrus.Entry
	server *http.Server
	engine *engine.Engine
}

// New creates a new Server instance.
func New(logger *logrus.Entry) *Server {
	return &Server{
		logger: logger,
	}
}

// SetEngine sets the collector engine for the server.
func (s *Server) SetEngine(eng *engine.Engine) {
	s.engine = eng
}

// ListenAndServe starts the daemon on the given unix socket path.
// It blocks until the server stops or fails.
func (s *Server) ListenAndServe(socketPath string) error {
	// Cleanup stale socket
	if _, err := os.Stat(socketPath); err == nil {
		if err := os.Remove(socketPath); err != nil {
			return fmt.Errorf("failed to remove stale socket: %w", err)
		}
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(socketPath), 0755); err != nil {
		return fmt.Errorf("failed to create socket directory: %w", err)
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on socket: %w", err)
	}

	// Set restrictive permissions on socket
	if err := os.Chmod(socketPath, 0600); err != nil {
		_ = listener.Close()
		return fmt.Errorf("failed to set socket permissions: %w", err)
	}

	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// State API endpoints
	mux.HandleFunc("/api/state", s.handleGetState)
	mux.HandleFunc("/api/workspaces", s.handleGetWorkspaces)
	mux.HandleFunc("/api/sessions", s.handleGetSessions)
	mux.HandleFunc("/api/stream", s.handleStreamState)

	s.server = &http.Server{
		Handler: h2c.NewHandler(mux, &http2.Server{}),
	}

	s.logger.WithField("socket", socketPath).Info("Daemon listening")
	return s.server.Serve(listener)
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down server...")
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

// handleGetState returns the complete daemon state as JSON.
func (s *Server) handleGetState(w http.ResponseWriter, r *http.Request) {
	if s.engine == nil {
		http.Error(w, "engine not initialized", http.StatusServiceUnavailable)
		return
	}

	state := s.engine.Store().Get()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state)
}

// handleGetWorkspaces returns all enriched workspaces as JSON.
func (s *Server) handleGetWorkspaces(w http.ResponseWriter, r *http.Request) {
	if s.engine == nil {
		http.Error(w, "engine not initialized", http.StatusServiceUnavailable)
		return
	}

	workspaces := s.engine.Store().GetWorkspaces()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(workspaces)
}

// handleGetSessions returns all active sessions as JSON.
func (s *Server) handleGetSessions(w http.ResponseWriter, r *http.Request) {
	if s.engine == nil {
		http.Error(w, "engine not initialized", http.StatusServiceUnavailable)
		return
	}

	sessions := s.engine.Store().GetSessions()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessions)
}

// handleStreamState provides Server-Sent Events (SSE) for real-time state updates.
// Clients can subscribe to this endpoint to receive updates whenever the daemon state changes.
func (s *Server) handleStreamState(w http.ResponseWriter, r *http.Request) {
	if s.engine == nil {
		http.Error(w, "engine not initialized", http.StatusServiceUnavailable)
		return
	}

	// Ensure the connection supports flushing
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Subscribe to store updates
	ch := s.engine.Store().Subscribe()
	defer s.engine.Store().Unsubscribe(ch)

	// Send initial ping to confirm connection
	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	s.logger.Debug("SSE client connected")

	// Send current state immediately so client has data right away
	state := s.engine.Store().Get()
	if len(state.Workspaces) > 0 {
		workspaces := make([]*enrichment.EnrichedWorkspace, 0, len(state.Workspaces))
		for _, ws := range state.Workspaces {
			workspaces = append(workspaces, ws)
		}
		initialUpdate := &apiStateUpdate{
			Workspaces: workspaces,
			UpdateType: "initial",
		}
		if data, err := json.Marshal(initialUpdate); err == nil {
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}

	for {
		select {
		case <-r.Context().Done():
			s.logger.Debug("SSE client disconnected")
			return
		case update := <-ch:
			// Convert internal store.Update to public API format
			apiUpdate := convertToAPIUpdate(update)
			if apiUpdate == nil {
				continue
			}

			data, err := json.Marshal(apiUpdate)
			if err != nil {
				s.logger.WithError(err).Error("Failed to marshal update")
				continue
			}
			// SSE format: "data: {json}\n\n"
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// apiStateUpdate matches the daemon.StateUpdate type for SSE streaming.
type apiStateUpdate struct {
	Workspaces []*enrichment.EnrichedWorkspace `json:"workspaces,omitempty"`
	UpdateType string                          `json:"update_type"`
}

// convertToAPIUpdate converts internal store.Update to the public API format.
func convertToAPIUpdate(u store.Update) *apiStateUpdate {
	switch u.Type {
	case store.UpdateWorkspaces:
		if wsMap, ok := u.Payload.(map[string]*enrichment.EnrichedWorkspace); ok {
			workspaces := make([]*enrichment.EnrichedWorkspace, 0, len(wsMap))
			for _, ws := range wsMap {
				workspaces = append(workspaces, ws)
			}
			return &apiStateUpdate{
				Workspaces: workspaces,
				UpdateType: "workspaces",
			}
		}
	case store.UpdateSessions:
		// Sessions updates are handled separately if needed
		return &apiStateUpdate{
			UpdateType: "sessions",
		}
	}
	return nil
}
