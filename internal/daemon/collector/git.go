package collector

import (
	"context"
	"time"

	"github.com/grovetools/core/git"
	"github.com/grovetools/core/internal/daemon/store"
	"github.com/grovetools/core/pkg/enrichment"
)

// GitStatusCollector updates git status for all workspaces.
type GitStatusCollector struct {
	interval time.Duration
}

// NewGitStatusCollector creates a new GitStatusCollector.
func NewGitStatusCollector() *GitStatusCollector {
	return &GitStatusCollector{
		interval: 10 * time.Second,
	}
}

// Name returns the collector's name.
func (c *GitStatusCollector) Name() string { return "git" }

// Run starts the git status collection loop.
func (c *GitStatusCollector) Run(ctx context.Context, st *store.Store, updates chan<- store.Update) error {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	scan := func() {
		state := st.Get()

		// Clone existing state and update git status
		newWorkspaces := make(map[string]*enrichment.EnrichedWorkspace)

		// Clone existing state
		for k, v := range state.Workspaces {
			// Shallow copy struct
			cpy := *v
			newWorkspaces[k] = &cpy
		}

		changed := false
		for _, ws := range newWorkspaces {
			status, err := git.GetExtendedStatus(ws.Path)
			if err == nil {
				ws.GitStatus = status
				changed = true
			}
		}

		if changed {
			updates <- store.Update{
				Type:    store.UpdateWorkspaces,
				Payload: newWorkspaces,
			}
		}
	}

	// Wait for workspaces to be populated first
	time.Sleep(1 * time.Second)
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
