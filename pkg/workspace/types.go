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

	// Cloned repository-specific fields (populated by discovery for cx repo managed repos)
	Version     string `json:"version,omitempty"`
	Commit      string `json:"commit,omitempty"`
	AuditStatus string `json:"audit_status,omitempty"`
	ReportPath  string `json:"report_path,omitempty"`
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

// ClaudeSessionInfo holds information about an active Claude session.
type ClaudeSessionInfo struct {
	ID       string `json:"id"`
	PID      int    `json:"pid"`
	Status   string `json:"status"`
	Duration string `json:"duration"`
}

// ProjectInfo is the enriched display model for projects.
// It represents a flattened, view-friendly project item suitable for UIs.
// It can represent an ecosystem, a primary project repository, or a worktree.
type ProjectInfo struct {
	Name                string `json:"name"`
	Path                string `json:"path"`
	ParentPath          string `json:"parent_path,omitempty"`           // For worktrees, path to the parent repository
	IsWorktree          bool   `json:"is_worktree"`
	WorktreeName        string `json:"worktree_name,omitempty"`         // For projects inside an ecosystem worktree
	ParentEcosystemPath string `json:"parent_ecosystem_path,omitempty"` // For sub-projects, path to parent ecosystem
	IsEcosystem         bool   `json:"is_ecosystem"`

	// Optional, enriched data (populated by EnrichProjects)
	GitStatus     interface{}        `json:"git_status,omitempty"`      // Using interface{} to avoid circular import with git package
	ClaudeSession *ClaudeSessionInfo `json:"claude_session,omitempty"`

	// Cloned repository-specific fields (populated by discovery)
	Version     string `json:"version,omitempty"`
	Commit      string `json:"commit,omitempty"`
	AuditStatus string `json:"audit_status,omitempty"`
	ReportPath  string `json:"report_path,omitempty"`
}