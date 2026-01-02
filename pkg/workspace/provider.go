package workspace

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mattsolo1/grove-core/config"
	"github.com/mattsolo1/grove-core/util/pathutil"
)

// Provider acts as a read-only, in-memory store for a snapshot of discovered workspaces.
// It provides fast lookups and access to the workspace hierarchy.
//
// The caching strategy is explicit and consumer-controlled. The Provider itself does not
// perform discovery; it is initialized with the results of a DiscoveryService.DiscoverAll()
// call. Short-lived applications should create a Provider once at startup. Long-running
// services are responsible for deciding when to refresh the data by performing a new
// discovery and creating a new Provider instance.
type Provider struct {
	nodes   []*WorkspaceNode
	pathMap map[string]*WorkspaceNode
}

// NewProvider creates a new workspace provider from a discovery result.
// It transforms the raw discovery data into the final WorkspaceNode representation
// and builds internal indexes for fast lookups.
func NewProvider(result *DiscoveryResult) *Provider {
	// Load config for notebook name resolution (best effort)
	cfg, _ := config.LoadDefault()
	if cfg == nil {
		cfg = &config.Config{}
	}

	nodes := TransformToWorkspaceNodes(result, cfg)
	nodes = BuildWorkspaceTree(nodes)

	pathMap := make(map[string]*WorkspaceNode, len(nodes))
	for _, node := range nodes {
		// Normalize the path for the lookup map.
		normalizedPath, err := pathutil.NormalizeForLookup(node.Path)
		if err == nil {
			pathMap[normalizedPath] = node
		}
	}

	p := &Provider{
		nodes:   nodes,
		pathMap: pathMap,
	}

	// p.validateAndWarnCollisions()

	return p
}

// All returns a slice of all discovered WorkspaceNodes.
func (p *Provider) All() []*WorkspaceNode {
	return p.nodes
}

// FindByName returns the first workspace node that matches the given name.
// It returns nil if no matching workspace is found.
func (p *Provider) FindByName(name string) *WorkspaceNode {
	for _, node := range p.nodes {
		if node.Name == name {
			return node
		}
	}
	return nil
}

// FindByPath returns the WorkspaceNode for a given absolute path.
// It performs a fast lookup using an internal map.
func (p *Provider) FindByPath(path string) *WorkspaceNode {
	normalizedPath, err := pathutil.NormalizeForLookup(path)
	if err != nil {
		// Cannot normalize, fallback to simple lookup
		return p.pathMap[path]
	}

	// First, try an exact match with the normalized path.
	if node, exists := p.pathMap[normalizedPath]; exists {
		return node
	}

	// If no exact match, find the containing workspace using normalized paths.
	var bestMatch *WorkspaceNode
	for _, node := range p.nodes {
		normalizedNodePath, _ := pathutil.NormalizeForLookup(node.Path)
		// Check if the normalized node's path is a prefix of the normalized search path.
		if strings.HasPrefix(normalizedPath, normalizedNodePath+string(filepath.Separator)) {
			if bestMatch == nil || len(normalizedNodePath) > len(bestMatch.Path) {
				bestMatch = node
			}
		}
	}
	return bestMatch
}

// FindByWorktree finds a workspace node for a worktree within an ecosystem.
// It is used to resolve a job's `worktree` field to the correct workspace node.
// - baseProjectNode: The base project node (can be ecosystem root or subproject).
// - worktreeName: The name of the worktree.
//
// This handles two cases:
// 1. Ecosystem worktrees: /ecosystem/.grove-worktrees/<name>/<subproject>
// 2. Subproject worktrees: /ecosystem/<subproject>/.grove-worktrees/<name>
func (p *Provider) FindByWorktree(baseProjectNode *WorkspaceNode, worktreeName string) *WorkspaceNode {
	// Determine the ecosystem path
	ecosystemPath := baseProjectNode.RootEcosystemPath
	if ecosystemPath == "" {
		ecosystemPath = baseProjectNode.ParentEcosystemPath
	}
	if ecosystemPath == "" && baseProjectNode.IsEcosystem() {
		ecosystemPath = baseProjectNode.Path
	}

	if ecosystemPath == "" {
		// This is a standalone project, it cannot have ecosystem worktrees.
		return nil
	}

	// Case 1: If base is a subproject, look for its worktree
	// e.g., grove-mcp/.grove-worktrees/1986
	if !baseProjectNode.IsEcosystem() {
		targetPath := filepath.Join(baseProjectNode.Path, ".grove-worktrees", worktreeName)
		if node := p.FindByPath(targetPath); node != nil && node.Path == targetPath {
			return node
		}
	}

	// Case 2: If base is ecosystem root, search for worktree in any subproject
	// This handles jobs stored at ecosystem level that reference subproject worktrees
	if baseProjectNode.IsEcosystem() {
		for _, node := range p.nodes {
			// Check if this is a worktree with the matching name in this ecosystem
			if node.IsWorktree() && node.Name == worktreeName && node.RootEcosystemPath == ecosystemPath {
				return node
			}
		}
	}

	// Case 3: Look for ecosystem worktree subproject
	// e.g., /ecosystem/.grove-worktrees/test444/grove-core
	if !baseProjectNode.IsEcosystem() {
		targetPath := filepath.Join(ecosystemPath, ".grove-worktrees", worktreeName, baseProjectNode.Name)
		if node := p.FindByPath(targetPath); node != nil && node.Path == targetPath {
			return node
		}
	}

	return nil
}

// Ecosystems returns all nodes that are ecosystem roots.
func (p *Provider) Ecosystems() []*WorkspaceNode {
	var ecosystems []*WorkspaceNode
	for _, node := range p.nodes {
		if node.IsEcosystem() {
			ecosystems = append(ecosystems, node)
		}
	}
	return ecosystems
}

// LocalWorkspaces returns a map of workspace names to their paths,
// suitable for use in submodule setup operations.
//
// WARNING: This method may silently overwrite entries if multiple workspaces
// have the same name (e.g., "grove-core" in different ecosystems).
// For ecosystem-aware operations, use LocalWorkspacesInEcosystem instead.
//
// Deprecated: Use LocalWorkspacesInEcosystem for more reliable lookups
// that avoid name collision issues.
func (p *Provider) LocalWorkspaces() map[string]string {
	result := make(map[string]string)
	for _, node := range p.nodes {
		result[node.Name] = node.Path
	}
	return result
}

// LocalWorkspacesInEcosystem returns a map of workspace names to their paths,
// filtered to only include workspaces within the specified ecosystem.
// This avoids name collisions between workspaces in different ecosystems.
func (p *Provider) LocalWorkspacesInEcosystem(ecosystemPath string) map[string]string {
	result := make(map[string]string)
	for _, node := range p.nodes {
		if node.RootEcosystemPath == ecosystemPath {
			result[node.Name] = node.Path
		}
	}
	return result
}

// validateAndWarnCollisions is called internally during provider construction
// to automatically detect and warn about workspace name collisions.
// It only reports true collisions between unrelated projects, not between
// a project and its worktrees which legitimately share the same name.
func (p *Provider) validateAndWarnCollisions() {
	// Group all nodes by name
	seen := make(map[string][]*WorkspaceNode)
	for _, node := range p.nodes {
		seen[node.Name] = append(seen[node.Name], node)
	}

	var collisions []error
	for name, nodes := range seen {
		if len(nodes) <= 1 {
			continue
		}

		// Group by logical source project to distinguish true collisions
		// from a project and its worktrees (which share the same name)
		sourceProjects := make(map[string][]string)
		for _, node := range nodes {
			// A worktree's source is its parent project, otherwise it's its own path
			sourcePath := node.Path
			if node.ParentProjectPath != "" {
				sourcePath = node.ParentProjectPath
			}
			sourceProjects[sourcePath] = append(sourceProjects[sourcePath], node.Path)
		}

		// A collision occurs only if there are multiple different source projects
		// with the same name. A project and its worktrees share one source.
		if len(sourceProjects) > 1 {
			var allPaths []string
			for _, paths := range sourceProjects {
				allPaths = append(allPaths, paths...)
			}
			collisions = append(collisions, fmt.Errorf(
				"duplicate workspace name '%s' found for different projects at: %v (consider renaming in grove.yml to avoid collisions)",
				name, allPaths))
		}
	}

	if len(collisions) > 0 {
		fmt.Printf("⚠️  Warning: workspace name collisions detected:\n")
		for _, collision := range collisions {
			fmt.Printf("   %v\n", collision)
		}
	}
}
