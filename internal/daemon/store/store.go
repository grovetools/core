package store

import (
	"sync"

	"github.com/grovetools/core/pkg/enrichment"
	"github.com/grovetools/core/pkg/models"
)

// Store is the in-memory state store for the daemon.
// It is thread-safe and supports pub/sub for real-time updates.
type Store struct {
	mu          sync.RWMutex
	state       *State
	subscribers map[chan Update]struct{}
}

// New creates a new Store instance.
func New() *Store {
	return &Store{
		state: &State{
			Workspaces: make(map[string]*enrichment.EnrichedWorkspace),
			Sessions:   make(map[string]*models.Session),
		},
		subscribers: make(map[chan Update]struct{}),
	}
}

// Get returns a copy of the current state.
func (s *Store) Get() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// Return shallow copy
	return *s.state
}

// GetWorkspaces returns a slice of all enriched workspaces.
func (s *Store) GetWorkspaces() []*enrichment.EnrichedWorkspace {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*enrichment.EnrichedWorkspace, 0, len(s.state.Workspaces))
	for _, ws := range s.state.Workspaces {
		result = append(result, ws)
	}
	return result
}

// GetSessions returns a slice of all sessions.
func (s *Store) GetSessions() []*models.Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*models.Session, 0, len(s.state.Sessions))
	for _, sess := range s.state.Sessions {
		result = append(result, sess)
	}
	return result
}

// ApplyUpdate modifies the state and notifies subscribers.
func (s *Store) ApplyUpdate(u Update) {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch u.Type {
	case UpdateWorkspaces:
		if workspaces, ok := u.Payload.(map[string]*enrichment.EnrichedWorkspace); ok {
			s.state.Workspaces = workspaces
		}
	case UpdateSessions:
		if sessions, ok := u.Payload.([]*models.Session); ok {
			// Rebuild map
			newMap := make(map[string]*models.Session)
			for _, sess := range sessions {
				newMap[sess.ID] = sess
			}
			s.state.Sessions = newMap
		}
	}

	// Broadcast to subscribers
	for ch := range s.subscribers {
		select {
		case ch <- u:
		default:
			// Non-blocking send to prevent slow clients from stalling the daemon
		}
	}
}

// Subscribe creates a new subscription channel for state updates.
func (s *Store) Subscribe() chan Update {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := make(chan Update, 100) // Buffered
	s.subscribers[ch] = struct{}{}
	return ch
}

// Unsubscribe removes a subscription and closes its channel.
func (s *Store) Unsubscribe(ch chan Update) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.subscribers, ch)
	close(ch)
}
