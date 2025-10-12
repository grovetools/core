package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mattsolo1/grove-core/config"
	"github.com/sirupsen/logrus"
)

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

	// Evaluate symlinks to get the canonical path
	absPath, err = filepath.EvalSymlinks(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path: %w", err)
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
		// For an ecosystem, we need to check if the target path is in a worktree
		// Generate the ecosystem node and check for worktrees
		ecoNode := &WorkspaceNode{
			Name:              foundCfg.Name,
			Path:              foundRootPath,
			Kind:              KindEcosystemRoot,
			RootEcosystemPath: foundRootPath,
		}
		if ecoNode.Name == "" {
			ecoNode.Name = filepath.Base(foundRootPath)
		}
		nodes = append(nodes, ecoNode)

		// Check if the path is in a worktree
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
								RootEcosystemPath:   foundRootPath,
							})
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

		// Determine if this project is inside an ecosystem
		parentEcosystemPath := ""
		// Check if we're inside an ecosystem by looking for a grove.yml with workspaces in parents
		checkDir := filepath.Dir(foundRootPath)
		for checkDir != filepath.Dir(checkDir) {
			if groveYml := filepath.Join(checkDir, "grove.yml"); fileExists(groveYml) {
				if cfg, err := config.Load(groveYml); err == nil && len(cfg.Workspaces) > 0 {
					parentEcosystemPath = checkDir
					break
				}
			}
			checkDir = filepath.Dir(checkDir)
		}

		// Determine the kind for the primary workspace
		kind := KindStandaloneProject
		if parentEcosystemPath != "" {
			// It's inside an ecosystem
			if strings.Contains(foundRootPath, filepath.Join(parentEcosystemPath, ".grove-worktrees")) {
				// Check if it's a git worktree (linked) or full checkout
				gitPath := filepath.Join(foundRootPath, ".git")
				if stat, err := os.Stat(gitPath); err == nil && !stat.IsDir() {
					kind = KindEcosystemWorktreeSubProjectWorktree
				} else {
					kind = KindEcosystemWorktreeSubProject
				}
			} else {
				kind = KindEcosystemSubProject
			}
		}

		primaryNode := &WorkspaceNode{
			Name:                projectName,
			Path:                foundRootPath,
			Kind:                kind,
			ParentEcosystemPath: parentEcosystemPath,
		}
		nodes = append(nodes, primaryNode)

		// Check for worktrees
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
					})
				}
			}
		}
	}

	// Find the most specific node that contains the original path
	var bestMatch *WorkspaceNode
	var bestMatchLen int

	// On macOS, use case-insensitive comparison
	caseInsensitive := runtime.GOOS == "darwin"

	for _, node := range nodes {
		nodePath := node.Path
		checkPath := absPath

		if caseInsensitive {
			nodePath = strings.ToLower(nodePath)
			checkPath = strings.ToLower(checkPath)
		}

		// Check for exact match
		if checkPath == nodePath {
			return node, nil
		}

		// Check if absPath is inside this node
		if strings.HasPrefix(checkPath, nodePath+string(filepath.Separator)) {
			if len(node.Path) > bestMatchLen {
				bestMatch = node
				bestMatchLen = len(node.Path)
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
