package workspace

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/util/pathutil"
)

// MatchReason classifies how an identifier/name resolved to a WorkspaceNode.
// Callers (e.g. cx's loud @a: fallback notice) use it to decide when a choice
// needs to be surfaced to the user.
type MatchReason int

const (
	// MatchNone means no node matched.
	MatchNone MatchReason = iota
	// ExactIdentifier: a fully-qualified colon identifier matched a node exactly.
	ExactIdentifier
	// MatchedUnique: a short name matched exactly one node.
	MatchedUnique
	// MatchedByContext: a short name was ambiguous and disambiguated using the
	// current path's ecosystem context.
	MatchedByContext
	// MatchedByFallback: a short name was ambiguous and no context applied, so a
	// deterministic canonical-first fallback ordering chose the node. This is the
	// case callers should surface loudly when the chosen root is not the current
	// worktree.
	MatchedByFallback
	// MatchedBySuffix: a multi-component identifier matched a node's identifier suffix.
	MatchedBySuffix
)

// String renders a MatchReason for diagnostics.
func (r MatchReason) String() string {
	switch r {
	case ExactIdentifier:
		return "exact-identifier"
	case MatchedUnique:
		return "unique-name"
	case MatchedByContext:
		return "context"
	case MatchedByFallback:
		return "fallback"
	case MatchedBySuffix:
		return "suffix"
	default:
		return "none"
	}
}

// isCanonicalNode reports whether a node is a canonical (main) checkout rather
// than a worktree or a copy living inside an ecosystem worktree. Used to bias
// deterministic tie-breaks toward the "real" checkout so that, e.g.,
// `@a:grove-anthropic` from an unrelated cwd means the main checkout, never a
// random worktree.
func isCanonicalNode(n *WorkspaceNode) bool {
	if n.IsWorktree() {
		return false
	}
	switch n.Kind {
	case KindEcosystemWorktreeSubProject, KindEcosystemWorktreeSubProjectWorktree:
		// A sub-project living inside an ecosystem worktree is a copy.
		return false
	}
	return true
}

// sortNodesDeterministic orders candidate nodes so that resolution is stable
// run-to-run and prefers the canonical checkout. Ordering: canonical checkouts
// first, then ascending Depth (shallowest first), then lexicographic Path.
//
// Note: Depth is not serialized across the daemon boundary (json:"-"), so
// daemon-seeded nodes fall back to the canonical-then-Path ordering — still
// fully deterministic.
func sortNodesDeterministic(nodes []*WorkspaceNode) {
	sort.SliceStable(nodes, func(i, j int) bool {
		a, b := nodes[i], nodes[j]
		ac, bc := isCanonicalNode(a), isCanonicalNode(b)
		if ac != bc {
			return ac // canonical first
		}
		if a.Depth != b.Depth {
			return a.Depth < b.Depth
		}
		return a.Path < b.Path
	})
}

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

	return NewProviderFromNodes(nodes)
}

// NewProviderFromNodes creates a workspace provider from pre-built WorkspaceNodes.
// This is useful when reusing already-discovered workspace data (e.g., from daemon cache)
// to avoid expensive re-discovery and path normalization.
func NewProviderFromNodes(nodes []*WorkspaceNode) *Provider {
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

// FindByName returns the workspace node that matches the given name, preferring
// the canonical checkout deterministically when several nodes share the name.
//
// Historically this returned the FIRST node in discovery order, which is
// arbitrary and unstable when a name collides across worktrees/ecosystems (the
// root cause behind `nb:`/`@a:` aliases rooting into random worktrees). It now
// applies the same canonical-first, shallowest-then-lexicographic ordering as
// FindByIdentifier's fallback. It returns nil if no matching workspace is found.
func (p *Provider) FindByName(name string) *WorkspaceNode {
	var matches []*WorkspaceNode
	for _, node := range p.nodes {
		if node.Name == name {
			matches = append(matches, node)
		}
	}
	if len(matches) == 0 {
		return nil
	}
	sortNodesDeterministic(matches)
	return matches[0]
}

// FindSubProjectByName returns the CANONICAL sub-project named name within the
// ecosystem rooted at ecosystemRoot. "Canonical" means the real, main checkout
// directly under the ecosystem root (a KindEcosystemSubProject / EcosystemRoot
// child) — NEVER a worktree copy and NEVER a node belonging to a different
// ecosystem.
//
// This exists because FindByName returns the FIRST node matching a name with no
// preference, so for a name that appears both as the ecosystem's sub-project AND
// inside some other worktree it can return the worktree copy (or a foreign
// ecosystem's node). Callers that must place artifacts under the canonical repo
// (e.g. `flow plan init --anchor <name>`) require this stricter resolution.
//
// What "canonical" excludes — and why RootEcosystemPath alone is insufficient:
// a sub-project living INSIDE an ecosystem worktree (e.g.
// <eco>/.grove-worktrees/<plan>/<name>, kind KindEcosystemWorktreeSubProject)
// can be discovered with IsWorktree()==false AND RootEcosystemPath==ecosystemRoot
// (its root traverses up past the worktree to the real ecosystem). Matching on
// the root alone would therefore return that copy. The discriminator that
// actually separates the canonical repo from a copy-inside-a-worktree is being
// a DIRECT child of the ecosystem root: ParentEcosystemPath == ecosystemRoot
// (a worktree-resident copy's ParentEcosystemPath is the worktree, not root).
//
// Selection rules, in order:
//  1. Exact path match: a non-worktree node whose Path == ecosystemRoot/name.
//     This is the strongest signal and handles the common direct-child case.
//  2. Otherwise, a non-worktree node named name that is a DIRECT child of the
//     ecosystem root (ParentEcosystemPath == ecosystemRoot).
//
// Returns nil when ecosystemRoot is empty or no canonical match exists.
func (p *Provider) FindSubProjectByName(name, ecosystemRoot string) *WorkspaceNode {
	if ecosystemRoot == "" {
		return nil
	}

	// A node is canonical-eligible only when it is not a worktree and does not
	// live inside an ecosystem worktree container.
	canonical := func(node *WorkspaceNode) bool {
		if node.Name != name || node.IsWorktree() {
			return false
		}
		switch node.Kind {
		case KindEcosystemWorktreeSubProject, KindEcosystemWorktreeSubProjectWorktree:
			// A sub-project inside an ecosystem worktree is a copy, never canonical.
			return false
		}
		return true
	}

	// Rule 1: prefer the node whose path is exactly ecosystemRoot/name. Compare
	// via normalized paths so symlink/case spellings agree with discovery.
	wantPath := filepath.Join(ecosystemRoot, name)
	normalizedWant, werr := pathutil.NormalizeForLookup(wantPath)
	for _, node := range p.nodes {
		if !canonical(node) {
			continue
		}
		if node.Path == wantPath {
			return node
		}
		if werr == nil {
			if normalizedNode, nerr := pathutil.NormalizeForLookup(node.Path); nerr == nil && normalizedNode == normalizedWant {
				return node
			}
		}
	}

	// Rule 2: any canonical-eligible direct child of this ecosystem root.
	for _, node := range p.nodes {
		if !canonical(node) {
			continue
		}
		if node.ParentEcosystemPath == ecosystemRoot {
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

	// If no exact match, find the containing workspace using pre-normalized paths from pathMap.
	// The pathMap keys are already normalized, so we avoid expensive re-normalization.
	var bestMatch *WorkspaceNode
	var bestMatchLen int
	for normalizedNodePath, node := range p.pathMap {
		// Check if the normalized node's path is a prefix of the normalized search path.
		if strings.HasPrefix(normalizedPath, normalizedNodePath+string(filepath.Separator)) {
			if bestMatch == nil || len(normalizedNodePath) > bestMatchLen {
				bestMatch = node
				bestMatchLen = len(normalizedNodePath)
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
		// Prefer the discovered node index: it is layout-independent.
		for _, node := range p.nodes {
			if node.IsWorktree() && node.Name == worktreeName && node.ParentProjectPath == baseProjectNode.Path {
				return node
			}
		}
		// Fallback: probe candidate paths under each worktree base.
		for _, base := range WorktreeBases(baseProjectNode.Path) {
			targetPath := filepath.Join(base, worktreeName)
			if node := p.FindByPath(targetPath); node != nil && node.Path == targetPath {
				return node
			}
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
		// Prefer the discovered node index: find the ecosystem worktree by
		// name, then the matching subproject inside it.
		for _, node := range p.nodes {
			if node.Kind == KindEcosystemWorktree && node.Name == worktreeName &&
				(node.RootEcosystemPath == ecosystemPath || node.ParentEcosystemPath == ecosystemPath) {
				targetPath := filepath.Join(node.Path, baseProjectNode.Name)
				if sub := p.FindByPath(targetPath); sub != nil && sub.Path == targetPath {
					return sub
				}
			}
		}
		// Fallback: probe candidate paths under each worktree base.
		for _, base := range WorktreeBases(ecosystemPath) {
			targetPath := filepath.Join(base, worktreeName, baseProjectNode.Name)
			if node := p.FindByPath(targetPath); node != nil && node.Path == targetPath {
				return node
			}
		}
	}

	return nil
}

// FindByIdentifier resolves a colon-delimited alias to a WorkspaceNode using
// progressive disambiguation. It tries exact fully-qualified matches first,
// then falls back to short name matching with context-aware scoping.
//
// It is a thin wrapper over FindByIdentifierWithInfo; callers that need to know
// HOW the node was chosen (e.g. to loudly surface an arbitrary fallback) should
// call FindByIdentifierWithInfo directly.
func (p *Provider) FindByIdentifier(identifier, currentPath string) *WorkspaceNode {
	node, _ := p.FindByIdentifierWithInfo(identifier, currentPath)
	return node
}

// FindByIdentifierWithInfo is FindByIdentifier plus a MatchReason describing how
// the node was selected. The resolution ladder is:
//
//  1. Exact fully-qualified colon identifier -> ExactIdentifier.
//  2. Short name: a single match -> MatchedUnique; multiple matches
//     disambiguated by the current path's ecosystem context -> MatchedByContext;
//     otherwise a deterministic canonical-first fallback -> MatchedByFallback.
//  3. Suffix match of a multi-component identifier -> MatchedBySuffix.
//
// Ambiguous short-name candidates are sorted deterministically up-front
// (sortNodesDeterministic), so both the context-priority ladder and the final
// fallback pick stable, reproducible nodes regardless of discovery order. This
// is what fixes the "same rules file roots into a different worktree every
// invocation" instability.
func (p *Provider) FindByIdentifierWithInfo(identifier, currentPath string) (*WorkspaceNode, MatchReason) {
	components := strings.Split(identifier, ":")

	// 1. For multi-component identifiers, try exact fully qualified match
	if len(components) > 1 {
		for _, node := range p.nodes {
			if node.Identifier(":") == identifier {
				return node, ExactIdentifier
			}
		}
	}

	// 2. Try matching against node names for short aliases (e.g. "cx")
	if len(components) == 1 {
		name := identifier
		var matches []*WorkspaceNode
		for _, node := range p.nodes {
			if node.Name == name {
				matches = append(matches, node)
			}
		}

		// Sort candidates deterministically so every "return first match" branch
		// below (context priorities and the final fallback) is reproducible.
		sortNodesDeterministic(matches)

		if len(matches) == 1 {
			return matches[0], MatchedUnique
		}

		// Disambiguate using current path's ecosystem
		if len(matches) > 1 && currentPath != "" {
			currentNode := p.FindByPath(currentPath)
			// Fallback: if FindByPath didn't match (e.g., path normalization issues),
			// try direct path comparison against node paths
			if currentNode == nil {
				for _, node := range p.nodes {
					if node.Path == currentPath {
						currentNode = node
						break
					}
				}
			}
			if currentNode != nil {
				// Priority 1: direct child of current ecosystem
				if currentNode.IsEcosystem() {
					for _, match := range matches {
						if match.ParentEcosystemPath == currentNode.Path {
							return match, MatchedByContext
						}
					}
				}
				// Priority 2: sibling in same ecosystem
				if currentNode.ParentEcosystemPath != "" {
					for _, match := range matches {
						if match.ParentEcosystemPath == currentNode.ParentEcosystemPath {
							return match, MatchedByContext
						}
					}
				}
				// Priority 3: same root ecosystem
				for _, match := range matches {
					if match.RootEcosystemPath == currentNode.RootEcosystemPath {
						return match, MatchedByContext
					}
				}
			}
		}

		// Fallback: deterministic canonical-first ordering (matches already sorted).
		if len(matches) > 0 {
			return matches[0], MatchedByFallback
		}
	}

	// 3. Try partial identifier matching for multi-component aliases
	// e.g., "eco-worktree:project" should match nodes where the last components match
	for _, node := range p.nodes {
		nodeID := node.Identifier(":")
		// Check if identifier matches a suffix of the node's full identifier
		if strings.HasSuffix(nodeID, identifier) {
			// Verify it's a clean component boundary (preceded by ":" or is the full string)
			prefixLen := len(nodeID) - len(identifier)
			if prefixLen == 0 || nodeID[prefixLen-1] == ':' {
				return node, MatchedBySuffix
			}
		}
	}

	return nil, MatchNone
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
		fmt.Printf("WARNING: workspace name collisions detected:\n")
		for _, collision := range collisions {
			fmt.Printf("   %v\n", collision)
		}
	}
}
