package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mattsolo1/grove-core/config"
	"github.com/mattsolo1/grove-core/util/pathutil"
	"github.com/sirupsen/logrus"
)

// assignNotebookName sets the NotebookName field for a node based on grove configuration.
// It finds the most specific grove (longest path match) that contains the node's path
// and assigns the notebook configured for that grove.
func assignNotebookName(node *WorkspaceNode, cfg *config.Config) {
	if cfg == nil || cfg.Groves == nil || len(cfg.Groves) == 0 {
		return
	}

	defaultNotebook := ""
	if cfg.Notebooks != nil && cfg.Notebooks.Rules != nil {
		defaultNotebook = cfg.Notebooks.Rules.Default
	}

	var bestMatchGrove string
	var bestMatchNotebook string

	// Normalize the node path for comparison
	normalizedNodePath, err := pathutil.NormalizeForLookup(node.Path)
	if err != nil {
		normalizedNodePath = node.Path
	}

	// Find the grove that this node's path falls under
	for _, groveCfg := range cfg.Groves {
		// Normalize paths for comparison
		expandedPath := expandPath(groveCfg.Path)
		grovePath, err := filepath.Abs(expandedPath)
		if err != nil {
			continue
		}

		// Normalize the grove path for comparison (handles case-insensitive filesystems)
		normalizedGrovePath, err := pathutil.NormalizeForLookup(grovePath)
		if err != nil {
			normalizedGrovePath = grovePath
		}

		// Check if node path is under this grove path
		if strings.HasPrefix(normalizedNodePath, normalizedGrovePath) {
			// Use the longest matching grove (most specific)
			if len(normalizedGrovePath) > len(bestMatchGrove) {
				bestMatchGrove = normalizedGrovePath
				bestMatchNotebook = groveCfg.Notebook
			}
		}
	}

	if bestMatchNotebook != "" {
		node.NotebookName = bestMatchNotebook
	} else {
		node.NotebookName = defaultNotebook
	}
}

// findRootEcosystemPath finds the top-most ecosystem containing a given directory.
// It traverses upward from startDir to find the highest-level ecosystem in a chain.
func findRootEcosystemPath(startDir string) string {
	var rootPath string
	current := startDir
	for {
		p := config.FindEcosystemConfig(current)
		if p == "" {
			break
		}
		rootPath = filepath.Dir(p)
		parent := filepath.Dir(rootPath)
		if parent == rootPath { // Filesystem root
			break
		}
		current = parent
	}
	return rootPath
}

// isGitWorktree checks if a directory is a git worktree by examining the .git file.
// This distinguishes true git worktrees from git submodules, which both have .git as a file.
// Worktrees point to .git/worktrees/, while submodules point to .git/modules/.
func isGitWorktree(dir string) bool {
	gitPath := filepath.Join(dir, ".git")
	stat, err := os.Stat(gitPath)
	if err != nil {
		return false
	}

	if stat.IsDir() {
		return false // Regular git repo
	}

	// It's a file - read it to distinguish worktree from submodule
	content, err := os.ReadFile(gitPath)
	if err != nil {
		return false
	}

	gitdir := strings.TrimSpace(string(content))
	// Worktrees point to .git/worktrees/, submodules point to .git/modules/
	return strings.Contains(gitdir, "/worktrees/") ||
		strings.Contains(gitdir, string(filepath.Separator)+"worktrees"+string(filepath.Separator))
}

// GetProjectByPath finds a workspace by path using an efficient upward traversal.
// It starts from the given path and walks up the directory tree looking for workspace markers.
// Once a project root is found, it generates the WorkspaceNode for that project and returns
// the most specific node that contains the original path.
//
// This approach is significantly faster than a full discovery scan (typically <10ms vs 100-500ms)
// and uses the same centralized classification logic to ensure consistency.
func GetProjectByPath(path string) (*WorkspaceNode, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("path does not exist or is not a directory: %s", absPath)
	}

	// Normalize path for case-insensitive filesystems and resolve symlinks
	absPath, err = pathutil.NormalizeForLookup(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to normalize path: %w", err)
	}

	// If we were given a file, start from its directory
	if !info.IsDir() {
		absPath = filepath.Dir(absPath)
	}

	// Perform upward traversal to find the containing workspace root
	current := absPath
	var foundRootPath string
	var foundCfg *config.Config
	var foundType directoryType

	for {
		// Use the centralized classifier to check if this directory is a workspace root
		dirType, cfg, err := classifyWorkspaceRoot(current)
		if err != nil {
			// Log but continue on classification errors
			logrus.Warnf("Error classifying directory %s: %v", current, err)
		}

		if dirType == typeProject || dirType == typeEcosystem || dirType == typeNonGroveRepo {
			// Found a workspace root
			foundRootPath = current
			foundCfg = cfg
			foundType = dirType
			break
		}

		// Move up one directory
		parent := filepath.Dir(current)
		if parent == current {
			// Reached filesystem root without finding a workspace
			return nil, fmt.Errorf("no workspace found containing path: %s", absPath)
		}
		current = parent
	}

	// Now we have found the root. Process it to generate WorkspaceNodes for this project.
	// We need to handle the different types appropriately.
	var nodes []*WorkspaceNode

	switch foundType {
	case typeNonGroveRepo:
		// Simple case: just return a single NonGroveRepo node
		nodes = []*WorkspaceNode{
			{
				Name: filepath.Base(foundRootPath),
				Path: foundRootPath,
				Kind: KindNonGroveRepo,
			},
		}

	case typeEcosystem:
		// Check if this ecosystem is itself a worktree
		// by checking both: (1) .git is a file, and (2) path contains .grove-worktrees
		isWorktree := isGitWorktree(foundRootPath) && strings.Contains(foundRootPath, ".grove-worktrees")

		// Find the true root ecosystem and parent ecosystem
		rootEcosystemPath := findRootEcosystemPath(foundRootPath)
		if rootEcosystemPath == "" {
			rootEcosystemPath = foundRootPath
		}

		// Find immediate parent ecosystem if this is a worktree
		var parentEcosystemPath string
		if isWorktree {
			// The parent should be found by looking for parent grove.yml
			checkDir := filepath.Dir(filepath.Dir(foundRootPath)) // Go up past .grove-worktrees
			_, cfg, err := findGroveConfig(checkDir)
			if err == nil && len(cfg.Workspaces) > 0 {
				parentEcosystemPath = checkDir
			}
		}

		// Determine the correct kind
		kind := KindEcosystemRoot
		if isWorktree && parentEcosystemPath != "" {
			kind = KindEcosystemWorktree
		}

		// If this ecosystem is a worktree, use the worktree name instead of ecosystem name
		nodeName := foundCfg.Name
		if nodeName == "" {
			nodeName = filepath.Base(foundRootPath)
		}
		if isWorktree {
			// For worktrees, use the directory name as the node name
			nodeName = filepath.Base(foundRootPath)
		}

		// Generate the ecosystem node
		ecoNode := &WorkspaceNode{
			Name:                nodeName,
			Path:                foundRootPath,
			Kind:                kind,
			ParentProjectPath:   parentEcosystemPath,
			ParentEcosystemPath: parentEcosystemPath,
			RootEcosystemPath:   rootEcosystemPath,
		}
		nodes = append(nodes, ecoNode)

		// Only check for worktree subdirectories if this is not already a worktree
		if !isWorktree {
			worktreeBase := filepath.Join(foundRootPath, ".grove-worktrees")
			if strings.HasPrefix(absPath, worktreeBase+string(filepath.Separator)) {
				// The path is inside a worktree
				// Find the specific worktree
				if entries, readErr := os.ReadDir(worktreeBase); readErr == nil {
					for _, entry := range entries {
						if entry.IsDir() {
							wtPath := filepath.Join(worktreeBase, entry.Name())
							if absPath == wtPath || strings.HasPrefix(absPath, wtPath+string(filepath.Separator)) {
								// This is the worktree containing the path
								nodes = append(nodes, &WorkspaceNode{
									Name:                entry.Name(),
									Path:                wtPath,
									Kind:                KindEcosystemWorktree,
									ParentProjectPath:   foundRootPath,
									ParentEcosystemPath: foundRootPath,
									RootEcosystemPath:   rootEcosystemPath,
								})
							}
						}
					}
				}
			}
		}

	case typeProject:
		// For a project, generate the primary workspace and any worktrees
		projectName := foundCfg.Name
		if projectName == "" {
			projectName = filepath.Base(foundRootPath)
		}

		// Check if this project is a worktree
		projectIsWorktree := isGitWorktree(foundRootPath)

		// If this is a worktree, use the directory name instead of config name
		if projectIsWorktree && strings.Contains(foundRootPath, ".grove-worktrees") {
			projectName = filepath.Base(foundRootPath)
		}

		// Determine if this project is inside an ecosystem
		parentEcosystemPath := ""
		// Check if we're inside an ecosystem by looking for a grove.yml with workspaces in parents
		checkDir := filepath.Dir(foundRootPath)
		for checkDir != filepath.Dir(checkDir) {
			_, cfg, err := findGroveConfig(checkDir)
			if err == nil && len(cfg.Workspaces) > 0 {
				parentEcosystemPath = checkDir
				break
			}
			checkDir = filepath.Dir(checkDir)
		}

		// Find the root ecosystem path
		rootEcosystemPath := ""
		if parentEcosystemPath != "" {
			rootEcosystemPath = findRootEcosystemPath(foundRootPath)
		}

		// Determine the kind for the primary workspace
		kind := KindStandaloneProject
		var parentProjectPath string

		// Check if this is a worktree of a standalone project (not in an ecosystem)
		if parentEcosystemPath == "" && strings.Contains(foundRootPath, ".grove-worktrees") {
			kind = KindStandaloneProjectWorktree
			// Set parent project path by going up past .grove-worktrees
			parentProjectPath = filepath.Dir(filepath.Dir(foundRootPath))
		} else if parentEcosystemPath != "" {
			// Check if the parent ecosystem is itself a worktree
			parentIsWorktree := strings.Contains(parentEcosystemPath, ".grove-worktrees")

			// It's inside an ecosystem
			if parentIsWorktree {
				// The project is inside an ecosystem worktree
				if projectIsWorktree {
					kind = KindEcosystemWorktreeSubProjectWorktree
					// ParentProjectPath should point to the corresponding sub-project in the root ecosystem
					// e.g., /path/to/ecosystem/grove-mcp (not .grove-worktrees!)
					if rootEcosystemPath != "" {
						parentProjectPath = filepath.Join(rootEcosystemPath, projectName)
					}
				} else {
					kind = KindEcosystemWorktreeSubProject
				}
			} else {
				// Parent ecosystem is not a worktree, but the project might be
				if projectIsWorktree {
					kind = KindEcosystemSubProjectWorktree
					// Set parent project path by going up past .grove-worktrees
					parentProjectPath = filepath.Dir(filepath.Dir(foundRootPath))
				} else {
					kind = KindEcosystemSubProject
				}
			}
		}

		primaryNode := &WorkspaceNode{
			Name:                projectName,
			Path:                foundRootPath,
			Kind:                kind,
			ParentProjectPath:   parentProjectPath,
			ParentEcosystemPath: parentEcosystemPath,
			RootEcosystemPath:   rootEcosystemPath,
		}
		nodes = append(nodes, primaryNode)

		// Check for worktrees of this project
		worktreeBase := filepath.Join(foundRootPath, ".grove-worktrees")
		if entries, readErr := os.ReadDir(worktreeBase); readErr == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					wtPath := filepath.Join(worktreeBase, entry.Name())
					wtKind := KindStandaloneProjectWorktree
					if parentEcosystemPath != "" {
						if strings.Contains(foundRootPath, ".grove-worktrees") {
							wtKind = KindEcosystemWorktreeSubProjectWorktree
						} else {
							wtKind = KindEcosystemSubProjectWorktree
						}
					}

					nodes = append(nodes, &WorkspaceNode{
						Name:                entry.Name(),
						Path:                wtPath,
						Kind:                wtKind,
						ParentProjectPath:   foundRootPath,
						ParentEcosystemPath: parentEcosystemPath,
						RootEcosystemPath:   rootEcosystemPath,
					})
				}
			}
		}
	}

	// Load config to assign notebook names
	cfg, err := config.LoadDefault()
	if err != nil {
		// Log but don't fail - notebook assignment is optional
		logrus.Debugf("Failed to load config for notebook assignment: %v", err)
	}

	// Assign notebook names to all nodes based on grove configuration
	if cfg != nil {
		for _, node := range nodes {
			assignNotebookName(node, cfg)
		}
	}

	// Find the most specific node that contains the original path
	var bestMatch *WorkspaceNode
	normalizedAbsPath, _ := pathutil.NormalizeForLookup(absPath)

	for _, node := range nodes {
		normalizedNodePath, _ := pathutil.NormalizeForLookup(node.Path)

		// Check for exact match
		if normalizedNodePath == normalizedAbsPath {
			return node, nil
		}

		// Check if absPath is inside this node
		if strings.HasPrefix(normalizedAbsPath, normalizedNodePath+string(filepath.Separator)) {
			if bestMatch == nil || len(node.Path) > len(bestMatch.Path) {
				bestMatch = node
			}
		}
	}

	if bestMatch != nil {
		return bestMatch, nil
	}

	// Fallback: return the first node (should be the root)
	if len(nodes) > 0 {
		return nodes[0], nil
	}

	return nil, fmt.Errorf("no workspace found containing path: %s", absPath)
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
