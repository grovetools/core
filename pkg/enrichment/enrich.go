package enrichment

import (
	"context"
	"sync"

	"github.com/grovetools/core/git"
	"github.com/grovetools/core/pkg/workspace"
)

// EnrichmentOptions controls which data to fetch and for which projects
type EnrichmentOptions struct {
	FetchNoteCounts bool
	FetchGitStatus  bool
	FetchPlanStats  bool
	GitStatusPaths  map[string]bool // nil means all projects
}

// DefaultEnrichmentOptions returns options that fetch everything for all projects
func DefaultEnrichmentOptions() *EnrichmentOptions {
	return &EnrichmentOptions{
		FetchNoteCounts: true,
		FetchGitStatus:  true,
		FetchPlanStats:  true,
		GitStatusPaths:  nil,
	}
}

// EnrichWorkspaces updates workspace nodes with runtime enrichment data.
// Returns a slice of EnrichedWorkspace with the requested data populated.
func EnrichWorkspaces(ctx context.Context, nodes []*workspace.WorkspaceNode, opts *EnrichmentOptions) []*EnrichedWorkspace {
	if opts == nil {
		opts = DefaultEnrichmentOptions()
	}

	enriched := make([]*EnrichedWorkspace, len(nodes))
	for i, node := range nodes {
		enriched[i] = &EnrichedWorkspace{WorkspaceNode: node}
	}

	// Fetch note counts (indexed by workspace name)
	var noteCountsByName map[string]*NoteCounts
	if opts.FetchNoteCounts {
		noteCountsByName, _ = FetchNoteCountsMap()
	}

	// Fetch plan stats (indexed by path)
	var planStatsMap map[string]*PlanStats
	if opts.FetchPlanStats {
		planStatsMap, _ = FetchPlanStatsMap()
	}

	// Apply non-git enrichments
	for _, ws := range enriched {
		if noteCountsByName != nil {
			if counts, ok := noteCountsByName[ws.Name]; ok {
				ws.NoteCounts = counts
			}
		}
		if planStatsMap != nil {
			if stats, ok := planStatsMap[ws.Path]; ok {
				ws.PlanStats = stats
			}
		}
	}

	// Fetch git status concurrently
	if opts.FetchGitStatus {
		var wg sync.WaitGroup
		semaphore := make(chan struct{}, 10)

		for _, ws := range enriched {
			shouldFetch := opts.GitStatusPaths == nil || opts.GitStatusPaths[ws.Path]
			if !shouldFetch {
				continue
			}

			wg.Add(1)
			go func(w *EnrichedWorkspace) {
				defer wg.Done()
				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				if extStatus, err := git.GetExtendedStatus(w.Path); err == nil {
					w.GitStatus = extStatus
				}
			}(ws)
		}
		wg.Wait()
	}

	return enriched
}
