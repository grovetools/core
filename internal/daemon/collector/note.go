package collector

import (
	"context"
	"time"

	"github.com/grovetools/core/internal/daemon/store"
	"github.com/grovetools/core/pkg/enrichment"
)

// NoteCollector updates note counts for all workspaces.
type NoteCollector struct {
	interval time.Duration
}

// NewNoteCollector creates a new NoteCollector with the specified interval.
// If interval is 0, defaults to 60 seconds.
func NewNoteCollector(interval time.Duration) *NoteCollector {
	if interval == 0 {
		interval = 60 * time.Second
	}
	return &NoteCollector{
		interval: interval,
	}
}

// Name returns the collector's name.
func (c *NoteCollector) Name() string { return "note" }

// Run starts the note counts collection loop.
func (c *NoteCollector) Run(ctx context.Context, st *store.Store, updates chan<- store.Update) error {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	scan := func() {
		// FetchNoteCountsMap returns counts by workspace name, not path
		noteCounts, err := enrichment.FetchNoteCountsMap()
		if err != nil {
			return
		}

		state := st.Get()

		// Clone existing workspaces and update note counts
		newWorkspaces := make(map[string]*enrichment.EnrichedWorkspace)

		for k, v := range state.Workspaces {
			cpy := *v
			// Note counts are indexed by workspace name, not path
			if cpy.WorkspaceNode != nil {
				if counts, ok := noteCounts[cpy.Name]; ok {
					cpy.NoteCounts = counts
				}
			}
			newWorkspaces[k] = &cpy
		}

		updates <- store.Update{
			Type:    store.UpdateWorkspaces,
			Payload: newWorkspaces,
		}
	}

	// Wait for workspaces to be populated first
	time.Sleep(3 * time.Second)
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
