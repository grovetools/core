package git

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mattsolo1/grove-core/command"
)

// GetRepoInfo returns the repository name and current branch
func GetRepoInfo(dir string) (repo string, branch string, err error) {
	cmdBuilder := command.NewSafeBuilder()

	// Find git root first to ensure context is correct for worktrees
	gitRoot, err := GetGitRoot(dir)
	if err != nil {
		return "", "", fmt.Errorf("could not find git root: %w", err)
	}

	// Get current branch from the specific worktree directory
	cmd, err := cmdBuilder.Build(context.Background(), "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", "", fmt.Errorf("failed to build command: %w", err)
	}
	execCmd := cmd.Exec()
	execCmd.Dir = dir // Use original dir for branch
	output, err := execCmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("get current branch: %w", err)
	}
	branch = strings.TrimSpace(string(output))

	// Get repository name from remote URL using the git root
	cmd, err = cmdBuilder.Build(context.Background(), "git", "config", "--get", "remote.origin.url")
	if err != nil {
		return "", "", fmt.Errorf("failed to build command: %w", err)
	}
	execCmd = cmd.Exec()
	execCmd.Dir = gitRoot // Use gitRoot for repo-level info
	output, err = execCmd.Output()
	if err != nil {
		// Fallback to the basename of the git root directory
		repo = filepath.Base(gitRoot)
		return repo, branch, nil
	}

	remoteURL := strings.TrimSpace(string(output))
	repo = extractRepoName(remoteURL)

	return repo, branch, nil
}

// extractRepoName extracts repository name from git URL
func extractRepoName(url string) string {
	// Remove .git suffix
	url = strings.TrimSuffix(url, ".git")

	// Handle SSH URLs (git@github.com:user/repo)
	if strings.HasPrefix(url, "git@") {
		parts := strings.Split(url, ":")
		if len(parts) >= 2 {
			url = parts[1]
		}
	}

	// Handle HTTPS URLs
	if strings.Contains(url, "github.com/") {
		parts := strings.Split(url, "github.com/")
		if len(parts) >= 2 {
			url = parts[1]
		}
	}

	// Get the last part of the path
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}

	return "unknown"
}

// IsGitRepo checks if the given directory is inside a git repository
func IsGitRepo(dir string) bool {
	cmdBuilder := command.NewSafeBuilder()
	cmd, err := cmdBuilder.Build(context.Background(), "git", "rev-parse", "--git-dir")
	if err != nil {
		return false
	}
	execCmd := cmd.Exec()
	execCmd.Dir = dir
	err = execCmd.Run()
	return err == nil
}

// GetGitRoot returns the root directory of the git repository
func GetGitRoot(dir string) (string, error) {
	cmdBuilder := command.NewSafeBuilder()
	cmd, err := cmdBuilder.Build(context.Background(), "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("failed to build command: %w", err)
	}
	execCmd := cmd.Exec()
	execCmd.Dir = dir
	output, err := execCmd.Output()
	if err != nil {
		return "", fmt.Errorf("get git root: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// GetSuperprojectRoot returns the root directory of the superproject if in a submodule
func GetSuperprojectRoot(dir string) (string, error) {
	cmdBuilder := command.NewSafeBuilder()
	cmd, err := cmdBuilder.Build(context.Background(), "git", "rev-parse", "--show-superproject-working-tree")
	if err != nil {
		return "", fmt.Errorf("failed to build command: %w", err)
	}
	execCmd := cmd.Exec()
	execCmd.Dir = dir
	output, err := execCmd.Output()
	if err != nil {
		return "", fmt.Errorf("get superproject root: %w", err)
	}

	superprojectRoot := strings.TrimSpace(string(output))
	if superprojectRoot == "" {
		// Not in a submodule, return regular git root
		return GetGitRoot(dir)
	}

	return superprojectRoot, nil
}

// ResolveRef resolves a git ref (branch name, tag, or commit) to its full commit hash.
// Returns empty string and error if resolution fails.
func ResolveRef(dir, ref string) (string, error) {
	cmdBuilder := command.NewSafeBuilder()
	cmd, err := cmdBuilder.Build(context.Background(), "git", "rev-parse", ref)
	if err != nil {
		return "", fmt.Errorf("failed to build command: %w", err)
	}
	execCmd := cmd.Exec()
	execCmd.Dir = dir
	output, err := execCmd.Output()
	if err != nil {
		return "", fmt.Errorf("resolve ref %s: %w", ref, err)
	}
	return strings.TrimSpace(string(output)), nil
}

// GetHeadCommit returns the current HEAD commit hash for a repository.
func GetHeadCommit(dir string) (string, error) {
	return ResolveRef(dir, "HEAD")
}