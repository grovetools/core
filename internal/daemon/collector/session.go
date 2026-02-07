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

// NewSessionCollector creates a new SessionCollector with the specified interval.
// If interval is 0, defaults to 2 seconds.
func NewSessionCollector(interval time.Duration) *SessionCollector {
	if interval == 0 {
		interval = 2 * time.Second
	}
	return &SessionCollector{
		interval: interval,
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
