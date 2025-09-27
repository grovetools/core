package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mattsolo1/grove-core/git"
)

// Prepare creates or gets a fully configured worktree.
func Prepare(ctx context.Context, opts PrepareOptions) (string, error) {
	wm := git.NewWorktreeManager()
	worktreePath, err := wm.GetOrPrepareWorktree(ctx, opts.GitRoot, opts.WorktreeName, opts.BranchName)
	if err != nil {
		return "", fmt.Errorf("failed to prepare worktree: %w", err)
	}

	if err := SetupSubmodules(ctx, worktreePath, opts.BranchName, opts.Repos); err != nil {
		fmt.Printf("Warning: failed to setup submodules for worktree '%s': %v\n", opts.WorktreeName, err)
	}

	if err := SetupGoWorkspaceForWorktree(worktreePath, opts.GitRoot); err != nil {
		fmt.Printf("Warning: failed to setup Go workspace in worktree: %v\n", err)
	}

	// Create a generic workspace marker file
	groveDir := filepath.Join(worktreePath, ".grove")
	os.MkdirAll(groveDir, 0755)
	markerPath := filepath.Join(groveDir, "workspace")
	markerContent := fmt.Sprintf("branch: %s\nplan: %s\ncreated_at: %s\n",
		opts.BranchName, opts.PlanName, time.Now().UTC().Format(time.RFC3339))
	os.WriteFile(markerPath, []byte(markerContent), 0644)

	return worktreePath, nil
}