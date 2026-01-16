package wsnav

import "github.com/grovetools/core/git"

// gitStatusLoadedMsg is sent when a workspace's Git status has been loaded.
//
// ENRICHMENT EXAMPLE: This message type demonstrates how to communicate
// asynchronously loaded enrichment data back to the TUI model. External
// callers implementing their own enrichment should follow this pattern:
// create custom message types for each enrichment source (git, sessions, etc.)
type gitStatusLoadedMsg struct {
	path   string
	status *git.StatusInfo
}
