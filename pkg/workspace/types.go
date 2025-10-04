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

// WorkspaceType defines whether a workspace is the main checkout or a git worktree.
type WorkspaceType string

const (
	WorkspaceTypePrimary  WorkspaceType = "Primary"
	WorkspaceTypeWorktree WorkspaceType = "Worktree"
)

// DiscoveredWorkspace represents a specific, checked-out instance of a Project.
type DiscoveredWorkspace struct {
	Name              string        `json:"name"`
	Path              string        `json:"path"`
	Type              WorkspaceType `json:"type"`
	ParentProjectPath string        `json:"parent_project_path"`
}

// Project represents a single software repository.
type Project struct {
	Name                string                `json:"name"`
	Path                string                `json:"path"`
	Type                string                `json:"type"`
	ModulePath          string                `json:"module_path,omitempty"`
	ParentEcosystemPath string                `json:"parent_ecosystem_path,omitempty"`
	Workspaces          []DiscoveredWorkspace `json:"workspaces"`
}

// Ecosystem represents a top-level meta-repository.
type Ecosystem struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"` // "Grove" or "User"
}

// DiscoveryResult is the comprehensive output of the DiscoveryService.
type DiscoveryResult struct {
	Projects            []Project  `json:"projects"`
	Ecosystems          []Ecosystem `json:"ecosystems"`
	NonGroveDirectories []string   `json:"non_grove_directories,omitempty"`
}