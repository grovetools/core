package git

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/grovetools/core/command"
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

// PruneWorktrees runs `git worktree prune` in repoPath, clearing stale/dangling
// worktree registrations (entries whose backing directory was removed with
// `rm -rf` instead of `git worktree remove`, leaving a "gitdir file points to
// non-existent location" entry). Such stale entries can cause a subsequent
// `git worktree add` to fail, so callers prune before adding to keep creation
// robust. Errors are returned so callers can decide whether to proceed.
func (m *WorktreeManager) PruneWorktrees(ctx context.Context, repoPath string) error {
	cmd, err := m.cmdBuilder.Build(ctx, "git", "worktree", "prune")
	if err != nil {
		return fmt.Errorf("failed to build command: %w", err)
	}

	execCmd := cmd.Exec()
	execCmd.Dir = repoPath

	if output, err := execCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("prune worktrees: %s", output)
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
