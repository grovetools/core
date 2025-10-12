package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/sirupsen/logrus"
)

// GetProjectByPath finds a workspace by path using the full discovery service.
// It discovers all workspaces and returns the one that contains the given path.
// If a path is inside a subdirectory of a workspace, it returns the containing workspace.
//
// Note: This function performs a full workspace discovery, which may take 100-500ms
// depending on the number of configured search paths. This ensures consistency with
// the rest of the system by using the canonical discovery and classification logic.
func GetProjectByPath(path string) (*WorkspaceNode, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("path does not exist or is not a directory: %s", absPath)
	}

	// Evaluate symlinks to get the canonical path for comparison
	// This resolves /var -> /private/var on macOS and other symlinks
	absPath, err = filepath.EvalSymlinks(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path: %w", err)
	}

	// Create a logger with minimal output (suppress discovery logs)
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Only show errors, not warnings
	logger.SetOutput(os.Stderr)         // Ensure output goes to stderr

	// Use the discovery service to get all workspaces
	nodes, err := GetProjects(logger)
	if err != nil {
		return nil, fmt.Errorf("failed to discover workspaces: %w", err)
	}

	// Find the workspace that contains this path
	// We want the longest matching prefix to get the most specific workspace
	var bestMatch *WorkspaceNode
	var bestMatchLen int

	// On macOS, use case-insensitive comparison since the default filesystem is case-insensitive
	caseInsensitive := runtime.GOOS == "darwin"

	for _, node := range nodes {
		// Resolve symlinks in the node path as well
		nodePath, err := filepath.EvalSymlinks(node.Path)
		if err != nil {
			// If we can't resolve, use the original path
			nodePath = node.Path
		}

		checkPath := absPath

		// Normalize case for comparison on case-insensitive filesystems
		if caseInsensitive {
			nodePath = strings.ToLower(nodePath)
			checkPath = strings.ToLower(checkPath)
		}

		// Check if absPath is equal to or inside this workspace
		if checkPath == nodePath {
			// Exact match - this is the best possible match
			return node, nil
		}

		// Check if absPath is a subdirectory of this workspace
		if strings.HasPrefix(checkPath, nodePath+string(filepath.Separator)) {
			// This node contains the path
			if len(nodePath) > bestMatchLen {
				bestMatch = node
				bestMatchLen = len(nodePath)
			}
		}
	}

	if bestMatch != nil {
		return bestMatch, nil
	}

	return nil, fmt.Errorf("no workspace found containing path: %s", absPath)
}
