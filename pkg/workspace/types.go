package workspace

// PrepareOptions holds configuration for preparing a workspace.
type PrepareOptions struct {
	GitRoot      string
	WorktreeName string
	BranchName   string
	PlanName     string   // Optional, for state management in grove-flow
	Repos        []string // For ecosystem worktrees
}

// WorkspaceInfo represents a workspace from grove ws list --json
type WorkspaceInfo struct {
	Name      string         `json:"name"`
	Path      string         `json:"path"`
	Worktrees []WorktreeInfo `json:"worktrees"`
}

// WorktreeInfo represents a worktree within a workspace
type WorktreeInfo struct {
	Path   string `json:"path"`
	Branch string `json:"branch"`
	IsMain bool   `json:"is_main"`
}