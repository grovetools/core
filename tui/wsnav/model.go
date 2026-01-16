package wsnav

import (
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/grovetools/core/git"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/core/tui/components/navigator"
)

// Model wraps the navigator component and provides custom table-based rendering.
type Model struct {
	navigator       navigator.Model
	scrollOffset    int
	SelectedProject *workspace.WorkspaceNode // The project selected when Enter is pressed

	// ENRICHMENT EXAMPLE: These fields demonstrate how to store enrichment data.
	// The map is keyed by workspace path, and the mutex ensures thread-safe access
	// from concurrent enrichment commands. External callers should add similar
	// fields for their own enrichment data (e.g., sessionInfo, noteCounts, etc.)
	gitStatus      map[string]*git.StatusInfo
	gitStatusMutex sync.RWMutex
}

// Init is the first command that will be executed.
func (m *Model) Init() tea.Cmd {
	// Combine navigator init with git status fetching
	cmds := []tea.Cmd{m.navigator.Init()}

	// ENRICHMENT EXAMPLE: Kick off enrichment commands for all initial projects.
	// This demonstrates the pattern of dispatching concurrent enrichment fetches
	// at initialization time. External callers should follow this pattern to
	// populate enrichment data when the TUI starts.
	filtered := m.navigator.GetFiltered()
	for _, p := range filtered {
		cmds = append(cmds, m.fetchGitStatusCmd(p.Path))
	}

	return tea.Batch(cmds...)
}

// fetchGitStatusCmd asynchronously fetches git status for a given path.
//
// ENRICHMENT EXAMPLE: This method demonstrates how to create async commands
// that fetch enrichment data. Key principles:
//   1. Return a tea.Cmd (a function that returns tea.Msg)
//   2. Do expensive I/O operations inside the command function
//   3. Return a custom message type with the results
//   4. Handle errors gracefully by returning nil data
//
// External callers should create similar methods for each enrichment source,
// such as fetchSessionInfoCmd, fetchNoteCountsCmd, etc.
func (m *Model) fetchGitStatusCmd(path string) tea.Cmd {
	return func() tea.Msg {
		// Check if it's a git repository first
		if !git.IsGitRepo(path) {
			return gitStatusLoadedMsg{path: path, status: nil}
		}

		status, err := git.GetStatus(path)
		if err != nil {
			return gitStatusLoadedMsg{path: path, status: nil}
		}

		return gitStatusLoadedMsg{path: path, status: status}
	}
}
