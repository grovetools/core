package workspace

import "path/filepath"

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
	nodes := TransformToWorkspaceNodes(result)
	nodes = BuildWorkspaceTree(nodes)

	pathMap := make(map[string]*WorkspaceNode, len(nodes))
	for _, node := range nodes {
		pathMap[node.Path] = node
	}

	return &Provider{
		nodes:   nodes,
		pathMap: pathMap,
	}
}

// All returns a slice of all discovered WorkspaceNodes.
func (p *Provider) All() []*WorkspaceNode {
	return p.nodes
}

// FindByPath returns the WorkspaceNode for a given absolute path.
// It performs a fast lookup using an internal map.
func (p *Provider) FindByPath(path string) *WorkspaceNode {
	// First, try an exact match
	if node, exists := p.pathMap[path]; exists {
		return node
	}

	// If no exact match, find the containing workspace
	var bestMatch *WorkspaceNode
	for _, node := range p.nodes {
		if node.Path == path {
			return node
		}
		// Check if the node's path is a prefix of the search path
		if len(path) > len(node.Path) && path[len(node.Path)] == filepath.Separator && path[:len(node.Path)] == node.Path {
			if bestMatch == nil || len(node.Path) > len(bestMatch.Path) {
				bestMatch = node
			}
		}
	}
	return bestMatch
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
func (p *Provider) LocalWorkspaces() map[string]string {
	result := make(map[string]string)
	for _, node := range p.nodes {
		result[node.Name] = node.Path
	}
	return result
}
