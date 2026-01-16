package alias

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/pkg/profiling"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/sirupsen/logrus"
)

// aliasLineParts holds the parsed components of a rule line containing an alias.
type aliasLineParts struct {
	// The full original line.
	OriginalLine string
	// Any prefix before the alias directive (e.g., "!", "@view: ").
	Prefix string
	// The core alias string (e.g., "ecosystem:repo").
	Alias string
	// Any path or glob pattern appended to the alias (e.g., "/src/**/*.go").
	Pattern string
	// The full resolved path pattern (e.g., "!/path/to/project/src/**/*.go").
	ResolvedLine string
}

// AliasResolver discovers available workspaces and resolves aliases to their absolute paths.
type AliasResolver struct {
	Provider        *workspace.Provider
	providerOnce    sync.Once
	DiscoverErr     error
	configPath      string // Optional: custom config path for testing
	workDir         string // Current working directory for context-aware resolution
	notebookLocator *workspace.NotebookLocator
}

// NewAliasResolver creates a new, uninitialized alias resolver.
// Discovery happens lazily on first use.
func NewAliasResolver() *AliasResolver {
	return &AliasResolver{}
}

// NewAliasResolverWithWorkDir creates a new alias resolver with a working directory for context-aware resolution.
func NewAliasResolverWithWorkDir(workDir string) *AliasResolver {
	return &AliasResolver{workDir: workDir}
}

// NewAliasResolverWithConfig creates a new alias resolver with a custom config path for testing.
func NewAliasResolverWithConfig(configPath string) *AliasResolver {
	return &AliasResolver{configPath: configPath}
}

// InitProvider performs the workspace discovery process once and initializes the provider.
func (r *AliasResolver) InitProvider() {
	r.providerOnce.Do(func() {
		defer profiling.Start("alias.InitProvider(DiscoverAll)").Stop()
		logger := logrus.New()
		logger.SetLevel(logrus.WarnLevel)

		discoveryService := workspace.NewDiscoveryService(logger)
		if r.configPath != "" {
			discoveryService = discoveryService.WithConfigPath(r.configPath)
		}

		discoveryResult, err := discoveryService.DiscoverAll()
		if err != nil {
			r.DiscoverErr = fmt.Errorf("failed to discover workspaces: %w", err)
			return
		}

		r.Provider = workspace.NewProvider(discoveryResult)

		// Initialize NotebookLocator with config
		cfg, _ := config.LoadDefault()
		r.notebookLocator = workspace.NewNotebookLocator(cfg)
	})
}

// Resolve translates an alias string into an absolute filesystem path. It supports
// two primary types of aliases:
//
// 1. Project/Workspace Aliases (default): Used to resolve the root path of a project,
//    ecosystem, or worktree. Can optionally use "project:" prefix for clarity.
//    Examples:
//      - "project-name"                           // Standalone project
//      - "ecosystem:repo"                         // Ecosystem sub-project
//      - "repo:worktree"                          // Worktree
//      - "ecosystem:repo:worktree"                // Ecosystem repo worktree
//      - "worktree:repo/pkg/alias/**"             // Path within worktree
//      - "project:ecosystem:repo/cmd/**"          // Explicit (optional)
//
// 2. Notebook Resource Aliases (requires "nb:" prefix): Used to resolve paths to
//    specific files or directories within a workspace's notebook structure.
//    The "nb:" prefix is REQUIRED to distinguish from project paths.
//    Examples:
//      - "nb:workspace-name:plans/my-plan"        // Plan directory
//      - "nb:workspace-name:inbox/my-note.md"     // Note file
//      - "nb:workspace-name:chats/conversation"   // Chat file
//
// Alias Syntax in Different Contexts:
//
// In .rules files, use the @a: or @alias: directive:
//   @a:grove-core/pkg/**                         // Project paths (default)
//   @a:concepts-eco:grove-core/cmd/**            // Worktree paths
//   @a:nb:grove-core:inbox/note.md               // Notebook resources (nb: required)
//
// In YAML configuration files (like concept manifests):
//   related_plans:
//     - nb:grove-core:plans/architecture         // nb: prefix required
//   related_notes:
//     - nb:grove-ecosystem:inbox/brainstorm.md   // nb: prefix required
//
// The "nb:" prefix is essential because both project and notebook aliases can contain
// colons and slashes (e.g., "worktree:repo/path" vs "nb:workspace:path"), making them
// ambiguous without an explicit marker.
func (r *AliasResolver) Resolve(alias string) (string, error) {
	defer profiling.Start("alias.Resolve").Stop()
	r.InitProvider()
	if r.DiscoverErr != nil {
		return "", r.DiscoverErr
	}
	if r.Provider == nil {
		return "", fmt.Errorf("workspace provider not initialized")
	}

	// Check for explicit nb: prefix for notebook resources
	if strings.HasPrefix(alias, "nb:") {
		return r.ResolveResourceAlias(strings.TrimPrefix(alias, "nb:"))
	}

	// Optional: support explicit project: prefix (can be omitted)
	if strings.HasPrefix(alias, "project:") {
		alias = strings.TrimPrefix(alias, "project:")
	}

	allNodes := r.Provider.All()
	components := strings.Split(alias, ":")

	// Context-aware resolution for single-component aliases
	if len(components) == 1 {
		name := components[0]
		if r.workDir != "" {
			// Normalize the workDir to handle macOS /private/var symlink
			normalizedWorkDir := r.workDir
			// On macOS, /private/var is symlinked to /var, but EvalSymlinks doesn't always resolve it
			// Try stripping /private prefix if it exists
			if strings.HasPrefix(normalizedWorkDir, "/private/") {
				normalizedWorkDir = strings.TrimPrefix(normalizedWorkDir, "/private")
			}

			currentNode := r.Provider.FindByPath(normalizedWorkDir)
			// If not found with /private stripped, try the original path
			if currentNode == nil && normalizedWorkDir != r.workDir {
				currentNode = r.Provider.FindByPath(r.workDir)
			}

			if currentNode != nil {
				// Priority 1: If the current node is an ecosystem (e.g., an ecosystem worktree root),
				// prioritize finding a direct child project with the alias name.
				if currentNode.IsEcosystem() {
					for _, node := range allNodes {
						if node.Name == name && node.ParentEcosystemPath == currentNode.Path {
							return node.Path, nil // Found a direct child.
						}
					}
				}

				// Priority 2: If the current node is a project within an ecosystem, prioritize finding a sibling project.
				// This handles resolving aliases between projects in the same ecosystem or ecosystem worktree.
				if currentNode.ParentEcosystemPath != "" {
					for _, node := range allNodes {
						if node.Name == name && node.ParentEcosystemPath == currentNode.ParentEcosystemPath {
							return node.Path, nil // Found a sibling.
						}
					}
				}
			}
		}

		// Fallback for single-component alias: find a top-level project or best match
		var topLevelMatch *workspace.WorkspaceNode
		var shallowerMatch *workspace.WorkspaceNode
		var anyMatch *workspace.WorkspaceNode
		for _, node := range allNodes {
			if node.Name == name {
				depth := node.Depth // Use pre-calculated depth
				if depth == 0 {     // Top-level nodes (standalone projects, ecosystems)
					if topLevelMatch == nil {
						topLevelMatch = node
					}
				}
				// Prefer shallower nodes (e.g., ecosystem sub-projects over worktree sub-projects)
				if shallowerMatch == nil || node.Depth < shallowerMatch.Depth {
					shallowerMatch = node
				}
				if anyMatch == nil {
					anyMatch = node
				}
			}
		}
		if topLevelMatch != nil {
			return topLevelMatch.Path, nil
		}
		if shallowerMatch != nil {
			return shallowerMatch.Path, nil
		}
		if anyMatch != nil {
			return anyMatch.Path, nil
		}
		return "", fmt.Errorf("alias not found: '%s'", alias)
	}

	// Resolution for multi-component aliases
	for _, node := range allNodes {
		switch len(components) {
		case 2: // ecosystem:repo OR repo:worktree OR eco-worktree:project
			comp1 := components[0]
			comp2 := components[1]
			// ecosystem:repo
			if node.Kind == workspace.KindEcosystemSubProject && filepath.Base(node.ParentEcosystemPath) == comp1 && node.Name == comp2 {
				return node.Path, nil
			}
			// repo:worktree
			if node.IsWorktree() && node.ParentProjectPath != "" && filepath.Base(node.ParentProjectPath) == comp1 && node.Name == comp2 {
				return node.Path, nil
			}
			// eco-worktree:project (e.g., general-refactoring:grove-core)
			if node.ParentEcosystemPath != "" && filepath.Base(node.ParentEcosystemPath) == comp1 && node.Name == comp2 {
				return node.Path, nil
			}

		case 3: // ecosystem:repo:worktree OR root-eco:eco-worktree:project
			comp1 := components[0]
			comp2 := components[1]
			comp3 := components[2]
			// ecosystem:repo:worktree
			if node.IsWorktree() && node.ParentProjectPath != "" && node.ParentEcosystemPath != "" &&
				filepath.Base(node.ParentEcosystemPath) == comp1 &&
				filepath.Base(node.ParentProjectPath) == comp2 &&
				node.Name == comp3 {
				return node.Path, nil
			}
			// root-eco:eco-worktree:project (e.g., grove-ecosystem:general-refactoring:grove-core)
			if node.ParentEcosystemPath != "" && node.Name == comp3 && filepath.Base(node.ParentEcosystemPath) == comp2 {
				// Check for root ecosystem name by traversing up from the parent ecosystem path
				// ParentEcosystemPath is like /path/to/root-eco/.grove-worktrees/eco-worktree
				ecoWorktreeParentDir := filepath.Dir(node.ParentEcosystemPath)
				if filepath.Base(ecoWorktreeParentDir) == ".grove-worktrees" {
					rootEcoPath := filepath.Dir(ecoWorktreeParentDir)
					if filepath.Base(rootEcoPath) == comp1 {
						return node.Path, nil
					}
				}
			}
		default:
			return "", fmt.Errorf("invalid alias format '%s', must have 1 to 3 components separated by ':'", alias)
		}
	}

	return "", fmt.Errorf("alias not found: '%s'", alias)
}

// ResolveResourceAlias resolves a notebook resource alias (e.g., "my-project:plans/my-plan")
// to its full absolute path.
func (r *AliasResolver) ResolveResourceAlias(alias string) (string, error) {
	r.InitProvider()
	if r.DiscoverErr != nil {
		return "", r.DiscoverErr
	}
	if r.Provider == nil {
		return "", fmt.Errorf("workspace provider not initialized")
	}

	// 1. Parse the alias into workspace and resource path.
	workspaceName, resourcePath, err := ParseResourceAlias(alias)
	if err != nil {
		return "", err
	}

	// 2. Resolve the workspace name to a WorkspaceNode.
	wsNode := r.Provider.FindByName(workspaceName)
	if wsNode == nil {
		// Attempt fuzzy matching for better error message.
		var suggestions []string
		for _, node := range r.Provider.All() {
			if strings.Contains(node.Name, workspaceName) {
				suggestions = append(suggestions, node.Name)
			}
		}
		if len(suggestions) > 0 {
			return "", fmt.Errorf("workspace not found: '%s'. Did you mean: %s?", workspaceName, strings.Join(suggestions, ", "))
		}
		return "", fmt.Errorf("workspace not found: '%s'", workspaceName)
	}

	// 3. Use NotebookLocator to get the directory based on the resource path
	// The resource path format is like "plans/my-plan" or "inbox/note.md"
	// We need to extract the first component to determine the resource type
	parts := strings.SplitN(resourcePath, "/", 2)
	groupName := parts[0] // e.g., "plans", "inbox", "chats"
	remainingPath := ""
	if len(parts) > 1 {
		remainingPath = parts[1] // e.g., "my-plan", "note.md"
	}

	// Use GetGroupDir to resolve the base directory for the resource type
	groupDir, err := r.notebookLocator.GetGroupDir(wsNode, groupName)
	if err != nil {
		return "", fmt.Errorf("could not resolve group directory for '%s' in workspace '%s': %w", groupName, workspaceName, err)
	}

	// 4. Join with the remaining path to get the final absolute path
	if remainingPath != "" {
		return filepath.Join(groupDir, remainingPath), nil
	}
	return groupDir, nil
}

// ResolveLine parses a full rule line, resolves the alias, and reconstructs the line with an absolute path.
func (r *AliasResolver) ResolveLine(line string) (string, error) {
	defer profiling.Start("alias.ResolveLine").Stop()
	// Handle @a:nb: and @alias:nb: prefixes for notebook paths
	trimmedLine := strings.TrimSpace(line)
	prefix := ""
	if strings.HasPrefix(trimmedLine, "!") {
		prefix = "!"
		trimmedLine = strings.TrimSpace(strings.TrimPrefix(trimmedLine, "!"))
	}

	if strings.HasPrefix(trimmedLine, "@a:nb:") || strings.HasPrefix(trimmedLine, "@alias:nb:") {
		var resourceAlias string
		if strings.HasPrefix(trimmedLine, "@a:nb:") {
			resourceAlias = strings.TrimPrefix(trimmedLine, "@a:nb:")
		} else {
			resourceAlias = strings.TrimPrefix(trimmedLine, "@alias:nb:")
		}
		resourceAlias = strings.TrimSpace(resourceAlias)

		// Use the new unified resolver.
		resolvedPath, err := r.ResolveResourceAlias(resourceAlias)
		if err != nil {
			return "", err
		}
		return prefix + resolvedPath, nil
	}

	parts, err := r.parseAliasLine(line)
	if err != nil {
		return "", err
	}

	resolvedPath, err := r.Resolve(parts.Alias)
	if err != nil {
		// TODO: Add suggestions for similar aliases.
		return "", fmt.Errorf("on line '%s': %w", line, err)
	}

	// Reconstruct the line.
	var finalPath string
	if strings.HasPrefix(parts.Pattern, " @") {
		// Pattern is a directive like " @grep: \"query\""
		// If the resolved path is a bare directory (no glob), append /** before the directive
		if !strings.Contains(resolvedPath, "*") && !strings.Contains(resolvedPath, "?") {
			finalPath = resolvedPath + "/**" + parts.Pattern
		} else {
			finalPath = resolvedPath + parts.Pattern
		}
	} else if parts.Pattern == "" {
		// No pattern, just the alias - append /** to match all files
		finalPath = resolvedPath + "/**"
	} else {
		// Pattern is a file path like "/pkg/**" or a glob like "**/*.go"
		// If pattern doesn't start with /, prepend it to make it relative
		pattern := parts.Pattern
		if !strings.HasPrefix(pattern, "/") {
			pattern = "/" + pattern
		}
		// Use filepath.Join to combine paths
		finalPath = filepath.Join(resolvedPath, pattern)
	}

	parts.ResolvedLine = strings.TrimSpace(parts.Prefix) + finalPath
	// Normalize short forms to full forms in output
	if strings.Contains(parts.Prefix, "@view:") || strings.Contains(parts.Prefix, "@v:") {
		parts.ResolvedLine = "@view: " + finalPath
	}

	return parts.ResolvedLine, nil
}

// parseAliasLine deconstructs a rule line into its prefix, alias, and pattern.
func (r *AliasResolver) parseAliasLine(line string) (*aliasLineParts, error) {
	// Regex to find @alias: (or @a:) and capture prefix, alias, and optional pattern/directives.
	// It handles prefixes like '!', '@view:' (or '@v:'), and combinations.
	// Supports short forms: @a: for @alias:, @v: for @view:
	// Pattern can be:
	//   - /path/pattern (traditional with leading slash)
	//   - **/*.go or *.go (glob patterns without leading slash)
	//   - @directive: "query" (search directives)
	//   - (nothing)
	// The alias part matches everything except /, whitespace, @, and glob characters (*, ?, [)
	re := regexp.MustCompile(`^(?P<prefix>!?(?:\s*@(?:view|v):\s*)?)?\s*@(?:alias|a):(?P<alias>[^/\s@*?\[]+)(?P<pattern>/.+|[*?\[].*|\s+@(?:find|grep):.+)?$`)
	matches := re.FindStringSubmatch(line)
	if matches == nil {
		return nil, fmt.Errorf("invalid alias format in line: '%s'", line)
	}

	parts := &aliasLineParts{OriginalLine: line}
	for i, name := range re.SubexpNames() {
		if i > 0 && i <= len(matches) {
			switch name {
			case "prefix":
				parts.Prefix = matches[i]
			case "alias":
				parts.Alias = matches[i]
			case "pattern":
				parts.Pattern = matches[i]
			}
		}
	}
	return parts, nil
}
