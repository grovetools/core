package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mattsolo1/grove-core/git"
	"github.com/sirupsen/logrus"
)

// Prepare creates or gets a fully configured worktree.
func Prepare(ctx context.Context, opts PrepareOptions, setupHandlers ...func(worktreePath, gitRoot string) error) (string, error) {
	wm := git.NewWorktreeManager()
	worktreePath, err := wm.GetOrPrepareWorktree(ctx, opts.GitRoot, opts.WorktreeName, opts.BranchName)
	if err != nil {
		return "", fmt.Errorf("failed to prepare worktree: %w", err)
	}

	// Instantiate a discovery service to find local workspaces for submodule setup
	// We use a null logger as we don't need discovery output here.
	logger := logrus.New()
	logger.SetOutput(os.Stderr)
	logger.SetLevel(logrus.WarnLevel)
	discoveryService := NewDiscoveryService(logger)

	if err := SetupSubmodules(ctx, worktreePath, opts.BranchName, opts.Repos, discoveryService); err != nil {
		fmt.Printf("Warning: failed to setup submodules for worktree '%s': %v\n", opts.WorktreeName, err)
	}

	// Run any provided post-setup handlers
	for _, handler := range setupHandlers {
		if err := handler(worktreePath, opts.GitRoot); err != nil {
			fmt.Printf("Warning: setup handler failed for worktree '%s': %v\n", opts.WorktreeName, err)
		}
	}

	// Create a generic workspace marker file
	groveDir := filepath.Join(worktreePath, ".grove")
	os.MkdirAll(groveDir, 0755)
	markerPath := filepath.Join(groveDir, "workspace")

	// Determine if this is an ecosystem worktree
	isEcosystem := len(opts.Repos) > 0

	markerContent := fmt.Sprintf("branch: %s\nplan: %s\ncreated_at: %s\necosystem: %t\n",
		opts.BranchName, opts.PlanName, time.Now().UTC().Format(time.RFC3339), isEcosystem)

	// Add repos list for ecosystem worktrees
	if isEcosystem {
		markerContent += "repos:\n"
		for _, repo := range opts.Repos {
			markerContent += fmt.Sprintf("  - %s\n", repo)
		}
	}

	os.WriteFile(markerPath, []byte(markerContent), 0644)

	return worktreePath, nil
}
