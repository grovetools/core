package collector

import (
	"context"
	"time"

	"github.com/grovetools/core/internal/daemon/store"
	"github.com/grovetools/core/pkg/enrichment"
)

// PlanCollector updates plan statistics for all workspaces.
type PlanCollector struct {
	interval time.Duration
}

// NewPlanCollector creates a new PlanCollector.
func NewPlanCollector() *PlanCollector {
	return &PlanCollector{
		interval: 30 * time.Second,
	}
}

// Name returns the collector's name.
func (c *PlanCollector) Name() string { return "plan" }

// Run starts the plan stats collection loop.
func (c *PlanCollector) Run(ctx context.Context, st *store.Store, updates chan<- store.Update) error {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	scan := func() {
		planStats, err := enrichment.FetchPlanStatsMap()
		if err != nil {
			return
		}

		state := st.Get()

		// Clone existing workspaces and update plan stats
		newWorkspaces := make(map[string]*enrichment.EnrichedWorkspace)

		for k, v := range state.Workspaces {
			cpy := *v
			if stats, ok := planStats[k]; ok {
				cpy.PlanStats = stats
			}
			newWorkspaces[k] = &cpy
		}

		updates <- store.Update{
			Type:    store.UpdateWorkspaces,
			Payload: newWorkspaces,
		}
	}

	// Wait for workspaces to be populated first
	time.Sleep(2 * time.Second)
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
