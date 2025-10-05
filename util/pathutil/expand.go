package pathutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mattsolo1/grove-core/git"
)

// Expand expands home directory (~), environment variables, and git variables in a path.
// It returns an absolute path.
func Expand(path string) (string, error) {
	// 1. Expand home directory character '~'.
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("could not get user home directory: %w", err)
		}
		path = filepath.Join(home, path[2:])
	}

	// 2. Expand environment variables.
	path = os.ExpandEnv(path)

	// 3. Expand git-related variables.
	if strings.Contains(path, "${REPO}") || strings.Contains(path, "${BRANCH}") || strings.Contains(path, "{{REPO}}") || strings.Contains(path, "{{BRANCH}}") {
		repo, branch, err := git.GetRepoInfo(".")
		if err != nil {
			// Don't fail if not in a git repo, just don't replace variables.
		} else {
			path = strings.ReplaceAll(path, "${REPO}", repo)
			path = strings.ReplaceAll(path, "${BRANCH}", branch)
			path = strings.ReplaceAll(path, "{{REPO}}", repo)
			path = strings.ReplaceAll(path, "{{BRANCH}}", branch)
		}
	}

	return filepath.Abs(path)
}
