package models

// RepoEnsureRequest is the daemon RPC payload for ensuring a repo+version is
// cloned and checked out. Defined here (not in pkg/repo) so daemon and cx can
// both depend on it without a daemon→repo→daemon import cycle.
type RepoEnsureRequest struct {
	URL     string `json:"url"`
	Version string `json:"version,omitempty"`
}

// RepoEnsureResponse carries the resolved worktree path and commit hash.
type RepoEnsureResponse struct {
	WorktreePath string `json:"worktree_path"`
	Commit       string `json:"commit"`
}
