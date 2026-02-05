package collector

import (
	"context"
	"time"

	"github.com/grovetools/core/internal/daemon/store"
	"github.com/grovetools/core/pkg/sessions"
)

// SessionCollector monitors active sessions.
type SessionCollector struct {
	interval time.Duration
}

// NewSessionCollector creates a new SessionCollector.
func NewSessionCollector() *SessionCollector {
	return &SessionCollector{
		interval: 2 * time.Second,
	}
}

// Name returns the collector's name.
func (c *SessionCollector) Name() string { return "session" }

// Run starts the session monitoring loop.
func (c *SessionCollector) Run(ctx context.Context, st *store.Store, updates chan<- store.Update) error {
	// Polling is cheap for PID checks
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	scan := func() {
		// Use core session discovery
		liveSessions, err := sessions.DiscoverLiveSessions()
		if err != nil {
			return
		}

		updates <- store.Update{
			Type:    store.UpdateSessions,
			Payload: liveSessions,
		}
	}

	scan()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			scan()
		}
	}
}
