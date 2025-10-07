package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mattsolo1/grove-core/command"
)

// WorktreeInfo contains information about a git worktree
type WorktreeInfo struct {
	Path   string
	Branch string
	Commit string
	Bare   bool
	Head   string
}

// WorktreeManager manages git worktrees
type WorktreeManager struct {
	cmdBuilder *command.SafeBuilder
}

// Ensure it implements the interface
var _ WorktreeProvider = (*WorktreeManager)(nil)

// NewWorktreeManager creates a new worktree manager
func NewWorktreeManager() *WorktreeManager {
	return &WorktreeManager{
		cmdBuilder: command.NewSafeBuilder(),
	}
}

// ListWorktrees returns all worktrees for the current repository
func (m *WorktreeManager) ListWorktrees(ctx context.Context, repoPath string) ([]WorktreeInfo, error) {
	cmd, err := m.cmdBuilder.Build(ctx, "git", "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("failed to build command: %w", err)
	}

	execCmd := cmd.Exec()
	execCmd.Dir = repoPath

	output, err := execCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list worktrees: %w", err)
	}

	return m.parseWorktreeList(string(output)), nil
}

// GetCurrentWorktree returns info about the current worktree
func (m *WorktreeManager) GetCurrentWorktree(ctx context.Context, path string) (*WorktreeInfo, error) {
	worktrees, err := m.ListWorktrees(ctx, path)
	if err != nil {
		return nil, err
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	for _, wt := range worktrees {
		if wt.Path == absPath {
			return &wt, nil
		}
	}

	return nil, fmt.Errorf("current directory is not a worktree")
}

// CreateWorktree creates a new worktree
func (m *WorktreeManager) CreateWorktree(ctx context.Context, basePath, worktreePath, branch string, createBranch bool) error {
	// Validate branch name
	if err := m.cmdBuilder.Validate("gitRef", branch); err != nil {
		return fmt.Errorf("invalid branch name: %w", err)
	}

	args := []string{"worktree", "add"}

	if createBranch {
		args = append(args, "-b", branch)
	}

	args = append(args, worktreePath)

	if !createBranch {
		args = append(args, branch)
	}

	cmd, err := m.cmdBuilder.Build(ctx, "git", args...)
	if err != nil {
		return fmt.Errorf("failed to build command: %w", err)
	}

	execCmd := cmd.Exec()
	execCmd.Dir = basePath

	output, err := execCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("create worktree: %s", output)
	}

	return nil
}

// RemoveWorktree removes a worktree
func (m *WorktreeManager) RemoveWorktree(ctx context.Context, basePath, worktreePath string) error {
	cmd, err := m.cmdBuilder.Build(ctx, "git", "worktree", "remove", worktreePath)
	if err != nil {
		return fmt.Errorf("failed to build command: %w", err)
	}

	execCmd := cmd.Exec()
	execCmd.Dir = basePath

	output, err := execCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("remove worktree: %s", output)
	}

	return nil
}

// parseWorktreeList parses git worktree list output
func (m *WorktreeManager) parseWorktreeList(output string) []WorktreeInfo {
	var worktrees []WorktreeInfo
	lines := strings.Split(output, "\n")

	var current WorktreeInfo
	for _, line := range lines {
		if line == "" {
			if current.Path != "" {
				worktrees = append(worktrees, current)
				current = WorktreeInfo{}
			}
			continue
		}

		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}

		switch parts[0] {
		case "worktree":
			current.Path = parts[1]
		case "HEAD":
			current.Commit = parts[1]
		case "branch":
			current.Branch = strings.TrimPrefix(parts[1], "refs/heads/")
		case "bare":
			current.Bare = true
		}
	}

	if current.Path != "" {
		worktrees = append(worktrees, current)
	}

	return worktrees
}

// GetWorktreeRoot returns the main worktree root
func (m *WorktreeManager) GetWorktreeRoot(ctx context.Context, path string) (string, error) {
	cmd, err := m.cmdBuilder.Build(ctx, "git", "worktree", "list")
	if err != nil {
		return "", fmt.Errorf("failed to build command: %w", err)
	}

	execCmd := cmd.Exec()
	execCmd.Dir = path

	output, err := execCmd.Output()
	if err != nil {
		return "", fmt.Errorf("get worktree root: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) > 0 {
		fields := strings.Fields(lines[0])
		if len(fields) > 0 {
			return fields[0], nil
		}
	}

	return "", fmt.Errorf("could not determine worktree root")
}

// GetOrPrepareWorktree gets an existing worktree or creates a new one
// This method is used by orchestration executors to ensure a consistent worktree setup
func (m *WorktreeManager) GetOrPrepareWorktree(ctx context.Context, basePath, worktreeName, branchName string) (string, error) {
	if worktreeName == "" {
		return "", fmt.Errorf("worktree name cannot be empty")
	}

	// Define standardized paths
	worktreesBaseDir := filepath.Join(basePath, ".grove-worktrees")
	worktreePath := filepath.Join(worktreesBaseDir, worktreeName)
	
	// If no branch name is provided, use the worktree name as the branch name
	if branchName == "" {
		branchName = worktreeName
	}

	// Need to find the git root for worktree operations
	gitRoot, err := GetGitRoot(basePath)
	if err != nil {
		return "", fmt.Errorf("get git root: %w", err)
	}

	// Check if worktree already exists
	worktrees, err := m.ListWorktrees(ctx, gitRoot)
	if err != nil {
		return "", fmt.Errorf("list worktrees: %w", err)
	}

	for _, wt := range worktrees {
		// Check if the branch is already checked out in any worktree
		if wt.Branch == branchName {
			// Verify the directory actually exists
			if _, err := os.Stat(wt.Path); err == nil {
				return wt.Path, nil // Branch already checked out in existing worktree
			}
		}

		if wt.Path == worktreePath {
			// Verify the directory actually exists
			if _, err := os.Stat(worktreePath); err == nil {
				return wt.Path, nil // Worktree exists and directory is present
			}
			// Directory doesn't exist, need to remove stale worktree entry
			if err := m.RemoveWorktree(ctx, gitRoot, worktreePath); err != nil {
				// Log warning but continue
				fmt.Printf("Warning: failed to remove stale worktree: %v\n", err)
			}
		}
	}

	// Ensure the base directory exists
	if err := os.MkdirAll(worktreesBaseDir, 0o755); err != nil {
		return "", fmt.Errorf("create worktrees base directory: %w", err)
	}

	// Create the worktree with a new branch
	if err := m.CreateWorktree(ctx, gitRoot, worktreePath, branchName, true); err != nil {
		// If branch already exists, try to create worktree using existing branch
		if strings.Contains(err.Error(), "already exists") {
			if err := m.CreateWorktree(ctx, gitRoot, worktreePath, branchName, false); err != nil {
				return "", fmt.Errorf("create worktree with existing branch: %w", err)
			}
		} else {
			return "", fmt.Errorf("create worktree: %w", err)
		}
	}

	return worktreePath, nil
}