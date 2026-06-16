package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/grovetools/core/command"
	"github.com/grovetools/core/pkg/paths"
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

// GetOrPrepareWorktree gets an existing worktree or creates a new one at the
// standardized legacy location (<basePath>/.grove-worktrees/<name>).
// This method is used by orchestration executors to ensure a consistent worktree setup
// Returns the worktree path, a boolean indicating if it was newly created, and an error
func (m *WorktreeManager) GetOrPrepareWorktree(ctx context.Context, basePath, worktreeName, branchName string) (string, bool, error) {
	if worktreeName == "" {
		return "", false, fmt.Errorf("worktree name cannot be empty")
	}
	// core/git cannot call workspace.ResolveNewWorktreePath (import cycle via
	// util/pathutil), so it joins against the shared layout constant.
	target := filepath.Join(basePath, paths.LegacyWorktreeDirName, worktreeName)
	return m.GetOrPrepareWorktreeAt(ctx, basePath, target, branchName)
}

// GetOrPrepareWorktreeAt gets an existing worktree or creates a new one at
// the resolved targetPath (legacy or XDG — the workspace layer computes it
// via ResolveNewWorktreePath). Idempotency, in order:
//
//  1. An existing worktree whose branch matches is returned wherever it
//     lives — legacy location preferred over XDG; never create a duplicate.
//  2. An existing directory at either candidate path (legacy join or the
//     requested target) is reused.
//  3. Stale worktree entries (registered but directory gone) at either
//     candidate path are cleaned up.
//  4. The target's parent directory is created before `git worktree add`
//     (callers like flow never run paths.EnsureDirs).
//
// Returns the worktree path, a boolean indicating if it was newly created,
// and an error.
func (m *WorktreeManager) GetOrPrepareWorktreeAt(ctx context.Context, basePath, targetPath, branchName string) (string, bool, error) {
	if targetPath == "" {
		return "", false, fmt.Errorf("worktree target path cannot be empty")
	}
	targetPath = filepath.Clean(targetPath)

	worktreeName := worktreeNameForTarget(basePath, targetPath)

	// If no branch name is provided, use the worktree name as the branch name
	if branchName == "" {
		branchName = worktreeName
	}

	// Both locations a worktree of this name could already occupy, legacy
	// first. When targetPath IS the legacy join the two coincide.
	candidates := []string{filepath.Join(basePath, paths.LegacyWorktreeDirName, worktreeName)}
	if candidates[0] != targetPath {
		candidates = append(candidates, targetPath)
	}

	// Need to find the git root for worktree operations
	gitRoot, err := GetGitRoot(basePath)
	if err != nil {
		return "", false, fmt.Errorf("get git root: %w", err)
	}

	// Check if worktree already exists
	worktrees, err := m.ListWorktrees(ctx, gitRoot)
	if err != nil {
		return "", false, fmt.Errorf("list worktrees: %w", err)
	}

	// Rule 1: the branch checked out anywhere wins, legacy candidate first.
	var branchMatch string
	for _, wt := range worktrees {
		if wt.Branch != branchName {
			continue
		}
		// Verify the directory actually exists
		if _, err := os.Stat(wt.Path); err != nil {
			continue
		}
		if wt.Path == candidates[0] {
			return wt.Path, false, nil
		}
		if branchMatch == "" {
			branchMatch = wt.Path
		}
	}
	if branchMatch != "" {
		return branchMatch, false, nil
	}

	for _, candidate := range candidates {
		for _, wt := range worktrees {
			if wt.Path != candidate {
				continue
			}
			if _, err := os.Stat(candidate); err == nil {
				return wt.Path, false, nil // Worktree exists and directory is present
			}
			// Rule 3: directory gone — remove the stale worktree entry.
			if err := m.RemoveWorktree(ctx, gitRoot, candidate); err != nil {
				// Log warning but continue
				fmt.Printf("Warning: failed to remove stale worktree: %v\n", err)
			}
		}
		// Rule 2: an unregistered directory at a candidate path is reused
		// rather than shadowed by a duplicate.
		if _, err := os.Stat(candidate); err == nil {
			return candidate, false, nil
		}
	}

	// Rule 4: ensure the target's parent directory exists.
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return "", false, fmt.Errorf("create worktrees base directory: %w", err)
	}

	// Create the worktree with a new branch
	if err := m.CreateWorktree(ctx, gitRoot, targetPath, branchName, true); err != nil {
		// If branch already exists, try to create worktree using existing branch
		if strings.Contains(err.Error(), "already exists") {
			if err := m.CreateWorktree(ctx, gitRoot, targetPath, branchName, false); err != nil {
				return "", false, fmt.Errorf("create worktree with existing branch: %w", err)
			}
		} else {
			return "", false, fmt.Errorf("create worktree: %w", err)
		}
	}

	return targetPath, true, nil
}

// worktreeNameForTarget derives the worktree name (possibly nested, e.g.
// "fix/deep") from a resolved target path: relative to the legacy base when
// the target is legacy-shaped, relative to the identifier dir when the
// target is under paths.WorktreesDir(). Falls back to the basename.
func worktreeNameForTarget(basePath, targetPath string) string {
	if rel, ok := relPathUnder(filepath.Join(basePath, paths.LegacyWorktreeDirName), targetPath); ok {
		return rel
	}
	if wtd := paths.WorktreesDir(); wtd != "" {
		if rel, ok := relPathUnder(wtd, targetPath); ok {
			parts := strings.Split(rel, string(filepath.Separator))
			if len(parts) >= 2 {
				return filepath.Join(parts[1:]...)
			}
		}
	}
	return filepath.Base(targetPath)
}

// relPathUnder returns path relative to base when path is strictly inside base.
func relPathUnder(base, path string) (string, bool) {
	rel, err := filepath.Rel(filepath.Clean(base), filepath.Clean(path))
	if err != nil || rel == "." || rel == ".." ||
		strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}
