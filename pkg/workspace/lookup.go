package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mattsolo1/grove-core/config"
)

// GetProjectByPath analyzes a single directory path and constructs a complete,
// enriched ProjectInfo object for it. It centralizes all logic for classifying
// a project (e.g., ecosystem, worktree, etc.).
func GetProjectByPath(path string) (*ProjectInfo, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("path does not exist or is not a directory: %s", absPath)
	}

	// Find parent ecosystem using the core config function
	ecoPath := config.FindEcosystemConfig(absPath)

	// Detect if it's a worktree by checking if .git is a file
	var isWorktree bool
	var parentPath string
	gitDirFile := filepath.Join(absPath, ".git")
	if stat, err := os.Stat(gitDirFile); err == nil && !stat.IsDir() {
		isWorktree = true
		// Determine parent path if it's in a .grove-worktrees dir
		if strings.Contains(absPath, ".grove-worktrees") {
			parts := strings.Split(absPath, ".grove-worktrees")
			if len(parts) > 0 {
				parentPath = strings.TrimSuffix(parts[0], string(filepath.Separator))
			}
		} else {
			// Fallback for worktrees not in our standard directory
			// This is less reliable but a reasonable guess.
			parentPath, _ = config.FindConfigFile(filepath.Dir(absPath))
			if parentPath != "" {
				parentPath = filepath.Dir(parentPath)
			}
		}
	}

	var parentEcosystemPath, worktreeName, worktreeRootPath string
	var isEcosystem bool

	// Extract worktree name if this project is inside any .grove-worktrees directory
	// This needs to be done before checking if absPath == ecoDir because an ecosystem
	// worktree is both an ecosystem AND a worktree
	if strings.Contains(absPath, string(filepath.Separator)+".grove-worktrees"+string(filepath.Separator)) {
		// Find the .grove-worktrees segment in the path
		parts := strings.Split(absPath, string(filepath.Separator)+".grove-worktrees"+string(filepath.Separator))
		if len(parts) >= 2 {
			// Get the first path segment after .grove-worktrees
			afterWorktrees := parts[1]
			worktreeParts := strings.Split(afterWorktrees, string(filepath.Separator))
			if len(worktreeParts) > 0 {
				worktreeName = worktreeParts[0]
				// Construct the worktree root path
				worktreeRootPath = parts[0] + string(filepath.Separator) + ".grove-worktrees" + string(filepath.Separator) + worktreeName
			}

			// For worktrees, also try to find the parent ecosystem by looking up from
			// the directory before .grove-worktrees
			parentDir := parts[0]
			if parentEcoPath := config.FindEcosystemConfig(parentDir); parentEcoPath != "" {
				parentEcosystemPath = filepath.Dir(parentEcoPath)
			}
		}
	}

	if ecoPath != "" {
		ecoDir := filepath.Dir(ecoPath) // The ecosystem root is the directory containing grove.yml

		if absPath == ecoDir {
			isEcosystem = true // This is the ecosystem root
		} else {
			// Only set parentEcosystemPath if we haven't already found it from worktree detection
			if parentEcosystemPath == "" {
				parentEcosystemPath = ecoDir
			}
		}
	}

	// An ecosystem worktree is both a worktree and an ecosystem
	isEcosystemWorktree := false
	if parentEcosystemPath != "" && isWorktree {
		relPath, err := filepath.Rel(parentEcosystemPath, absPath)
		if err == nil {
			parts := strings.Split(relPath, string(filepath.Separator))
			// It's a worktree directory if path is like: .grove-worktrees/some-name
			if len(parts) == 2 && parts[0] == ".grove-worktrees" {
				isEcosystemWorktree = true
			}
		}
	}
	if isEcosystemWorktree {
		isEcosystem = true
	}

	return &ProjectInfo{
		Name:                filepath.Base(absPath),
		Path:                absPath,
		ParentPath:          parentPath,
		IsWorktree:          isWorktree,
		WorktreeName:        worktreeName,
		WorktreeRootPath:    worktreeRootPath,
		ParentEcosystemPath: parentEcosystemPath,
		IsEcosystem:         isEcosystem,
	}, nil
}
