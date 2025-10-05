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

	var parentEcosystemPath, worktreeName string
	var isEcosystem bool

	if ecoPath != "" {
		ecoDir := filepath.Dir(ecoPath) // The ecosystem root is the directory containing grove.yml

		if absPath == ecoDir {
			isEcosystem = true // This is the ecosystem root
		} else {
			parentEcosystemPath = ecoDir
			// Extract worktree name if this project is inside the ecosystem's .grove-worktrees
			if strings.HasPrefix(absPath, filepath.Join(ecoDir, ".grove-worktrees")) {
				relPath, err := filepath.Rel(ecoDir, absPath)
				if err == nil {
					parts := strings.Split(relPath, string(filepath.Separator))
					if len(parts) >= 2 && parts[0] == ".grove-worktrees" {
						worktreeName = parts[1]
					}
				}
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
		ParentEcosystemPath: parentEcosystemPath,
		IsEcosystem:         isEcosystem,
	}, nil
}
