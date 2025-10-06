package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/mattsolo1/grove-core/git"
)

// ExtendedGitStatus holds git status info plus additional line change stats
type ExtendedGitStatus struct {
	*git.StatusInfo
	LinesAdded   int `json:"lines_added"`
	LinesDeleted int `json:"lines_deleted"`
}

// claudeSessionRaw represents the raw JSON structure from grove-hooks
type claudeSessionRaw struct {
	ID                   string `json:"id"`
	Type                 string `json:"type"`
	PID                  int    `json:"pid"`
	Status               string `json:"status"`
	WorkingDirectory     string `json:"working_directory"`
	StateDuration        string `json:"state_duration"`
	StateDurationSeconds int    `json:"state_duration_seconds"`
}

// EnrichmentOptions controls which data to fetch and for which projects
type EnrichmentOptions struct {
	// FetchNoteCounts controls whether to fetch note count data from nb
	FetchNoteCounts bool

	// FetchClaudeSessions controls whether to fetch Claude session data
	FetchClaudeSessions bool

	// FetchGitStatus controls whether to fetch Git status
	FetchGitStatus bool

	// GitStatusPaths limits Git status fetching to specific paths
	// If nil or empty, fetches for all projects
	GitStatusPaths map[string]bool
}

// DefaultEnrichmentOptions returns options that fetch everything for all projects
func DefaultEnrichmentOptions() *EnrichmentOptions {
	return &EnrichmentOptions{
		FetchNoteCounts:     true,
		FetchClaudeSessions: true,
		FetchGitStatus:      true,
		GitStatusPaths:      nil, // nil means all projects
	}
}

// EnrichProjects updates ProjectInfo items in-place with Git and Claude session data.
// This function fetches runtime state concurrently and efficiently.
// Use options to control what data is fetched and for which projects.
func EnrichProjects(ctx context.Context, projects []*ProjectInfo, opts *EnrichmentOptions) error {
	if opts == nil {
		opts = DefaultEnrichmentOptions()
	}

	// Fetch note counts once if requested
	var noteCountsMap map[string]*NoteCounts
	if opts.FetchNoteCounts {
		var err error
		noteCountsMap, err = fetchNoteCountsMap(projects)
		if err != nil {
			// Non-fatal, just means we can't show note counts
		}
	}

	// Fetch Claude sessions once if requested
	var claudeSessionMap map[string]*ClaudeSessionInfo
	if opts.FetchClaudeSessions {
		var err error
		claudeSessionMap, err = fetchClaudeSessionMap()
		if err != nil {
			// Non-fatal - just log and continue without Claude session enrichment
			// In production, consider using a logger
		}
	}

	// Create a wait group for concurrent Git status fetching
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 10) // Limit to 10 concurrent git operations

	// Enrich each project
	for i := range projects {
		project := projects[i]

		// Attach note count info if available
		if noteCountsMap != nil {
			if counts, ok := noteCountsMap[project.Path]; ok {
				project.NoteCounts = counts
			}
		}

		// Attach Claude session info if available
		if claudeSessionMap != nil {
			if session, ok := claudeSessionMap[project.Path]; ok {
				project.ClaudeSession = session
			}
		}

		// Fetch Git status concurrently if requested
		if opts.FetchGitStatus {
			// Check if this project should have Git status fetched
			shouldFetch := opts.GitStatusPaths == nil || opts.GitStatusPaths[project.Path]

			if shouldFetch {
				wg.Add(1)
				go func(p *ProjectInfo) {
					defer wg.Done()
					semaphore <- struct{}{}        // Acquire semaphore
					defer func() { <-semaphore }() // Release semaphore

					if extStatus, err := fetchGitStatusForPath(p.Path); err == nil {
						p.GitStatus = extStatus
					}
				}(project)
			}
		}
	}

	// Wait for all git status fetches to complete
	wg.Wait()

	return nil
}

// fetchGitStatusForPath gets extended git status for a given path
func fetchGitStatusForPath(path string) (*ExtendedGitStatus, error) {
	cleanPath := filepath.Clean(path)

	// Check if it's a git repo before getting status
	if !git.IsGitRepo(cleanPath) {
		return nil, nil // Not an error, just not a git repo
	}

	status, err := git.GetStatus(cleanPath)
	if err != nil {
		return nil, err
	}

	extStatus := &ExtendedGitStatus{
		StatusInfo: status,
	}

	// Get line stats using git diff --numstat
	cmd := exec.Command("git", "diff", "--numstat")
	cmd.Dir = cleanPath
	output, err := cmd.Output()
	if err == nil {
		extStatus.LinesAdded, extStatus.LinesDeleted = parseNumstat(string(output))
	}

	// Also get staged changes
	cmd = exec.Command("git", "diff", "--cached", "--numstat")
	cmd.Dir = cleanPath
	output, err = cmd.Output()
	if err == nil {
		stagedAdded, stagedDeleted := parseNumstat(string(output))
		extStatus.LinesAdded += stagedAdded
		extStatus.LinesDeleted += stagedDeleted
	}

	return extStatus, nil
}

// parseNumstat parses git diff --numstat output to count lines added/deleted
func parseNumstat(output string) (added, deleted int) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			// Skip binary files (shown as "-")
			if fields[0] != "-" {
				if a, err := strconv.Atoi(fields[0]); err == nil {
					added += a
				}
			}
			if fields[1] != "-" {
				if d, err := strconv.Atoi(fields[1]); err == nil {
					deleted += d
				}
			}
		}
	}
	return added, deleted
}

// fetchClaudeSessionMap fetches active Claude sessions and returns a map keyed by path
func fetchClaudeSessionMap() (map[string]*ClaudeSessionInfo, error) {
	sessionMap := make(map[string]*ClaudeSessionInfo)

	// Execute `grove-hooks sessions list --active --json`
	groveHooksPath := filepath.Join(os.Getenv("HOME"), ".grove", "bin", "grove-hooks")
	var cmd *exec.Cmd
	if _, err := os.Stat(groveHooksPath); err == nil {
		cmd = exec.Command(groveHooksPath, "sessions", "list", "--active", "--json")
	} else {
		cmd = exec.Command("grove-hooks", "sessions", "list", "--active", "--json")
	}

	output, err := cmd.Output()
	if err != nil {
		return sessionMap, err
	}

	var claudeSessions []claudeSessionRaw
	if err := json.Unmarshal(output, &claudeSessions); err != nil {
		return sessionMap, err
	}

	for _, session := range claudeSessions {
		// Only include sessions with type "claude_session"
		if session.Type == "claude_session" && session.WorkingDirectory != "" {
			absPath, err := filepath.Abs(expandPath(session.WorkingDirectory))
			if err != nil {
				continue
			}
			cleanPath := filepath.Clean(absPath)

			sessionMap[cleanPath] = &ClaudeSessionInfo{
				ID:       session.ID,
				PID:      session.PID,
				Status:   session.Status,
				Duration: session.StateDuration,
			}
		}
	}

	return sessionMap, nil
}

// fetchNoteCountsMap fetches note counts for all workspaces and returns a map keyed by project path.
func fetchNoteCountsMap(projects []*ProjectInfo) (map[string]*NoteCounts, error) {
	resultsByPath := make(map[string]*NoteCounts)

	// Build a map of workspace names to paths for efficient lookup
	nameToPath := make(map[string]string)
	for _, p := range projects {
		if p != nil {
			nameToPath[p.Name] = p.Path
		}
	}

	// Execute `nb list --workspaces --json`
	// First, try the standard grove binary path
	nbPath := filepath.Join(os.Getenv("HOME"), ".grove", "bin", "nb")
	var cmd *exec.Cmd
	if _, err := os.Stat(nbPath); err == nil {
		cmd = exec.Command(nbPath, "list", "--workspaces", "--json")
	} else {
		// Fallback to searching the system PATH
		cmd = exec.Command("nb", "list", "--workspaces", "--json")
	}

	output, err := cmd.Output()
	if err != nil {
		// If `nb` is not found or fails, we return an empty map without an error.
		// This is a non-fatal operation.
		return resultsByPath, nil
	}

	// Define a local struct to unmarshal only the necessary fields
	type nbNote struct {
		Type      string `json:"type"`
		Workspace string `json:"workspace"`
	}

	var notes []nbNote
	if err := json.Unmarshal(output, &notes); err != nil {
		return resultsByPath, fmt.Errorf("failed to unmarshal nb output: %w", err)
	}

	// Aggregate counts by workspace name
	countsByName := make(map[string]*NoteCounts)
	for _, note := range notes {
		if _, ok := countsByName[note.Workspace]; !ok {
			countsByName[note.Workspace] = &NoteCounts{}
		}
		switch note.Type {
		case "current":
			countsByName[note.Workspace].Current++
		case "issues":
			countsByName[note.Workspace].Issues++
		}
	}

	// Convert the name-keyed map to a path-keyed map
	for name, counts := range countsByName {
		if path, ok := nameToPath[name]; ok {
			resultsByPath[path] = counts
		}
	}

	return resultsByPath, nil
}
