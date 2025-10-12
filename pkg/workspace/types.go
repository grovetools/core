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
	Projects            []Project   `json:"projects"`
	Ecosystems          []Ecosystem `json:"ecosystems"`
	NonGroveDirectories []string    `json:"non_grove_directories,omitempty"`
}

// WorkspaceKind provides an unambiguous classification for a discovered workspace entity.
type WorkspaceKind string

const (
	// --- Standalone Projects (not part of an Ecosystem) ---

	// KindStandaloneProject: A standard project with a grove.yml, not within an Ecosystem.
	// Diagram:
	// /path/to/my-project/  (grove.yml, .git/)
	KindStandaloneProject WorkspaceKind = "StandaloneProject"

	// KindStandaloneProjectWorktree: A git worktree of a StandaloneProject.
	// Diagram:
	// /path/to/my-project/
	//   ├─ .git/
	//   └─ .grove-worktrees/
	//        └─ feature-branch/ (grove.yml, .git file) <-- This
	KindStandaloneProjectWorktree WorkspaceKind = "StandaloneProjectWorktree"

	// --- Ecosystem Root and its immediate children ---

	// KindEcosystemRoot: The main repository of an ecosystem (has grove.yml with a 'workspaces' key).
	// Diagram:
	// /path/to/my-ecosystem/ (grove.yml with 'workspaces', .git/) <-- This
	KindEcosystemRoot WorkspaceKind = "EcosystemRoot"

	// KindEcosystemWorktree: A git worktree of an EcosystemRoot. It also functions as an ecosystem.
	// Diagram:
	// /path/to/my-ecosystem/
	//   ├─ .git/
	//   └─ .grove-worktrees/
	//        └─ eco-feature/ (grove.yml with 'workspaces', .git file) <-- This
	KindEcosystemWorktree WorkspaceKind = "EcosystemWorktree"

	// KindEcosystemSubProject: A project (e.g., submodule) located directly inside an EcosystemRoot.
	// Diagram:
	// /path/to/my-ecosystem/ (EcosystemRoot)
	//   └─ sub-project/ (grove.yml, .git/) <-- This
	KindEcosystemSubProject WorkspaceKind = "EcosystemSubProject"

	// KindEcosystemSubProjectWorktree: A git worktree of an EcosystemSubProject.
	// Diagram:
	// /path/to/my-ecosystem/ (EcosystemRoot)
	//   └─ sub-project/
	//        ├─ .git/
	//        └─ .grove-worktrees/
	//             └─ sub-feature/ (grove.yml, .git file) <-- This
	KindEcosystemSubProjectWorktree WorkspaceKind = "EcosystemSubProjectWorktree"

	// --- Projects within an Ecosystem Worktree ---

	// KindEcosystemWorktreeSubProject: A project located inside an EcosystemWorktree.
	// This occurs when a submodule is initialized with `git submodule update` instead of as a linked worktree.
	// Diagram:
	// /path/to/my-ecosystem/.grove-worktrees/eco-feature/ (EcosystemWorktree)
	//   └─ sub-project/ (grove.yml, .git/) <-- This
	KindEcosystemWorktreeSubProject WorkspaceKind = "EcosystemWorktreeSubProject"

	// KindEcosystemWorktreeSubProjectWorktree: A git worktree of an EcosystemWorktreeSubProject.
	// This is the preferred "linked development" state for a sub-project in an ecosystem worktree.
	// Diagram:
	// /path/to/my-ecosystem/.grove-worktrees/eco-feature/ (EcosystemWorktree)
	//   └─ sub-project/ (grove.yml, .git file) <-- This
	KindEcosystemWorktreeSubProjectWorktree WorkspaceKind = "EcosystemWorktreeSubProjectWorktree"

	// --- Other ---

	// KindNonGroveRepo: A directory with a .git folder but no grove.yml.
	// Diagram:
	// /path/to/other-repo/ (.git/ only, no grove.yml) <-- This
	KindNonGroveRepo WorkspaceKind = "NonGroveRepo"
)

// ProjectInfo is the enriched display model for projects.
// It represents a flattened, view-friendly project item suitable for UIs.
type ProjectInfo struct {
	Name string        `json:"name"`
	Path string        `json:"path"`
	Kind WorkspaceKind `json:"kind"` // The single source of truth for the entity's type.

	// ParentProjectPath is the path to the repository that manages this worktree.
	// It is set ONLY for kinds that are worktrees.
	ParentProjectPath string `json:"parent_project_path,omitempty"`

	// ParentEcosystemPath is the path to the containing ecosystem's root directory.
	// It is set for ALL kinds that exist within an ecosystem context.
	ParentEcosystemPath string `json:"parent_ecosystem_path,omitempty"`

	// Cloned repository-specific fields (populated by discovery)
	Version     string `json:"version,omitempty"`
	Commit      string `json:"commit,omitempty"`
	AuditStatus string `json:"audit_status,omitempty"`
	ReportPath  string `json:"report_path,omitempty"`
}
