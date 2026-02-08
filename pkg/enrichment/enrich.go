package enrichment

import (
	"context"
	"sync"

	"github.com/grovetools/core/git"
	"github.com/grovetools/core/pkg/workspace"
)

// EnrichmentOptions controls which data to fetch and for which projects
type EnrichmentOptions struct {
	FetchNoteCounts   bool
	FetchGitStatus    bool
	FetchPlanStats    bool
	FetchReleaseInfo  bool
	FetchBinaryStatus bool
	FetchCxStats      bool
	FetchRemoteURL    bool
	GitStatusPaths    map[string]bool // nil means all projects
}

// DefaultEnrichmentOptions returns options that fetch the fast/common enrichments.
// More expensive enrichments (release, binary, cx) are off by default.
func DefaultEnrichmentOptions() *EnrichmentOptions {
	return &EnrichmentOptions{
		FetchNoteCounts:   true,
		FetchGitStatus:    true,
		FetchPlanStats:    true,
		FetchReleaseInfo:  false, // Off by default - requires grove list
		FetchBinaryStatus: false, // Off by default - requires grove list
		FetchCxStats:      false, // Off by default - requires cx stats
		FetchRemoteURL:    false, // Off by default - requires git remote
		GitStatusPaths:    nil,
	}
}

// FullEnrichmentOptions returns options that fetch all enrichment types.
func FullEnrichmentOptions() *EnrichmentOptions {
	return &EnrichmentOptions{
		FetchNoteCounts:   true,
		FetchGitStatus:    true,
		FetchPlanStats:    true,
		FetchReleaseInfo:  true,
		FetchBinaryStatus: true,
		FetchCxStats:      true,
		FetchRemoteURL:    true,
		GitStatusPaths:    nil,
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

	// Fetch release info and binary status (both use grove list)
	var releaseInfoMap map[string]*ReleaseInfo
	var binaryStatusMap map[string]*BinaryStatus
	if opts.FetchReleaseInfo || opts.FetchBinaryStatus {
		releaseInfoMap, binaryStatusMap = FetchToolInfoMap(nodes, opts.FetchReleaseInfo, opts.FetchBinaryStatus)
	}

	// Fetch CX stats
	var cxStatsMap map[string]*CxStats
	if opts.FetchCxStats {
		cxStatsMap = FetchCxStatsMap(nodes)
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
		if releaseInfoMap != nil {
			if info, ok := releaseInfoMap[ws.Path]; ok {
				ws.ReleaseInfo = info
			}
		}
		if binaryStatusMap != nil {
			if status, ok := binaryStatusMap[ws.Path]; ok {
				ws.ActiveBinary = status
			}
		}
		if cxStatsMap != nil {
			if stats, ok := cxStatsMap[ws.Path]; ok {
				ws.CxStats = stats
			}
		}
	}

	// Fetch git status and remote URL concurrently
	if opts.FetchGitStatus || opts.FetchRemoteURL {
		var wg sync.WaitGroup
		semaphore := make(chan struct{}, 10)

		for _, ws := range enriched {
			shouldFetchGit := opts.FetchGitStatus && (opts.GitStatusPaths == nil || opts.GitStatusPaths[ws.Path])
			shouldFetchRemote := opts.FetchRemoteURL

			if !shouldFetchGit && !shouldFetchRemote {
				continue
			}

			wg.Add(1)
			go func(w *EnrichedWorkspace, fetchGit, fetchRemote bool) {
				defer wg.Done()
				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				if fetchGit {
					if extStatus, err := git.GetExtendedStatus(w.Path); err == nil {
						w.GitStatus = extStatus
					}
				}
				if fetchRemote {
					w.GitRemoteURL = GetRemoteURL(w.Path)
				}
			}(ws, shouldFetchGit, shouldFetchRemote)
		}
		wg.Wait()
	}

	return enriched
}
