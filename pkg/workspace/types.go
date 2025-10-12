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

// WorkspaceNode is the enriched display model for workspace entities.
// It represents a flattened, view-friendly node suitable for UIs, with explicit
// parent-child relationships that form a hierarchical tree structure.
type WorkspaceNode struct {
	Name string        `json:"name"`
	Path string        `json:"path"`
	Kind WorkspaceKind `json:"kind"` // The single source of truth for the entity's type.

	// ParentProjectPath is the path to the repository that manages this worktree.
	// It is set ONLY for kinds that are worktrees (e.g., StandaloneProjectWorktree,
	// EcosystemWorktree, EcosystemSubProjectWorktree, EcosystemWorktreeSubProjectWorktree).
	ParentProjectPath string `json:"parent_project_path,omitempty"`

	// ParentEcosystemPath is the path to the immediate parent that provides ecosystem context.
	// This could be an EcosystemRoot or an EcosystemWorktree.
	// It is set for ALL kinds that exist within an ecosystem context.
	ParentEcosystemPath string `json:"parent_ecosystem_path,omitempty"`

	// RootEcosystemPath is the path to the top-level EcosystemRoot for this node.
	// This allows quick grouping by the ultimate parent ecosystem and facilitates
	// traversing to the root of the hierarchy. It is set for all nodes within an ecosystem.
	RootEcosystemPath string `json:"root_ecosystem_path,omitempty"`

	// Presentation fields for TUI rendering (pre-calculated for performance)
	TreePrefix string `json:"-"` // Pre-calculated tree indentation and connectors (e.g., "  ├─ ")
	Depth      int    `json:"-"` // Cached depth in the hierarchy

	// Cloned repository-specific fields (populated by discovery)
	Version     string `json:"version,omitempty"`
	Commit      string `json:"commit,omitempty"`
	AuditStatus string `json:"audit_status,omitempty"`
	ReportPath  string `json:"report_path,omitempty"`
}

// IsWorktree returns true if this node represents a worktree
func (w *WorkspaceNode) IsWorktree() bool {
	switch w.Kind {
	case KindStandaloneProjectWorktree,
		KindEcosystemWorktree,
		KindEcosystemSubProjectWorktree,
		KindEcosystemWorktreeSubProjectWorktree:
		return true
	default:
		return false
	}
}

// IsEcosystem returns true if this node represents an ecosystem (root or worktree)
func (w *WorkspaceNode) IsEcosystem() bool {
	return w.Kind == KindEcosystemRoot || w.Kind == KindEcosystemWorktree
}

// GetDepth returns the nesting depth of this node in the hierarchy
// 0 = standalone projects and root ecosystems
// 1 = ecosystem worktrees and sub-projects
// 2 = sub-project worktrees and worktree sub-projects
// 3 = worktree sub-project worktrees
func (w *WorkspaceNode) GetDepth() int {
	switch w.Kind {
	case KindStandaloneProject, KindEcosystemRoot, KindNonGroveRepo:
		return 0
	case KindStandaloneProjectWorktree, KindEcosystemWorktree, KindEcosystemSubProject:
		return 1
	case KindEcosystemSubProjectWorktree, KindEcosystemWorktreeSubProject:
		return 2
	case KindEcosystemWorktreeSubProjectWorktree:
		return 3
	default:
		return 0
	}
}

// GetHierarchicalParent returns the logical parent path for hierarchical display.
// This considers both ParentProjectPath (for worktrees) and ParentEcosystemPath (for sub-projects).
func (w *WorkspaceNode) GetHierarchicalParent() string {
	// Worktrees have their project as parent
	if w.ParentProjectPath != "" {
		return w.ParentProjectPath
	}
	// Sub-projects have their ecosystem as parent
	if w.ParentEcosystemPath != "" {
		return w.ParentEcosystemPath
	}
	// Top-level nodes have no parent
	return ""
}
