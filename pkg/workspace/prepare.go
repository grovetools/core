package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/grovetools/core/git"
)

// Prepare creates or gets a fully configured worktree.
func Prepare(ctx context.Context, opts PrepareOptions, setupHandlers ...func(worktreePath, gitRoot string) error) (string, error) {
	// Centralized safeguard: check if the git root is a notebook repo.
	if IsNotebookRepo(opts.GitRoot) {
		return "", fmt.Errorf("cannot create project worktree inside a notebook repository located at %s. Run this command from your project directory", opts.GitRoot)
	}

	if opts.WorktreeName == "" {
		return "", fmt.Errorf("worktree name cannot be empty")
	}

	wm := git.NewWorktreeManager()
	target := ResolveNewWorktreePath(opts.GitRoot, opts.WorktreeName, opts.UseXDGWorktrees)
	worktreePath, created, err := wm.GetOrPrepareWorktreeAt(ctx, opts.GitRoot, target, opts.BranchName)
	if err != nil {
		return "", fmt.Errorf("failed to prepare worktree: %w", err)
	}

	// Only run setup logic for newly created worktrees
	if created {
		// Discover all workspaces once and create a provider for efficient lookups
		logger := logrus.New()
		logger.SetOutput(os.Stderr)
		logger.SetLevel(logrus.WarnLevel)
		discoveryService := NewDiscoveryService(logger)

		discoveryResult, err := discoveryService.DiscoverAll()
		if err != nil {
			fmt.Printf("Warning: failed to discover workspaces for worktree '%s': %v\n", opts.WorktreeName, err)
		}

		// Create a provider from the discovery result
		var provider *Provider
		if discoveryResult != nil {
			provider = NewProvider(discoveryResult)
		}

		if err := SetupSubmodules(ctx, worktreePath, opts.GitRoot, opts.BranchName, opts.SiblingWorkspaces, provider, setupHandlers...); err != nil {
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
		_ = os.MkdirAll(groveDir, 0o755)
		markerPath := filepath.Join(groveDir, "workspace")

		// Determine if this is an ecosystem worktree
		isEcosystem := len(opts.SiblingWorkspaces) > 0

		// The ecosystem:/repos: keys below are a frozen persisted format —
		// keep them verbatim. owner: is an additive key recording the owning
		// repository root so deleted (zombie) worktrees stay owner-resolvable
		// after their .git file is gone (see WorktreeOwner).
		ownerPath := opts.GitRoot
		if abs, err := filepath.Abs(opts.GitRoot); err == nil {
			ownerPath = abs
		}
		markerContent := fmt.Sprintf("branch: %s\nplan: %s\ncreated_at: %s\nowner: %s\necosystem: %t\n",
			opts.BranchName, opts.PlanName, time.Now().UTC().Format(time.RFC3339), ownerPath, isEcosystem)

		// Add repos list for ecosystem worktrees
		if isEcosystem {
			markerContent += "repos:\n"
			for _, repo := range opts.SiblingWorkspaces {
				markerContent += fmt.Sprintf("  - %s\n", repo)
			}
		}

		_ = os.WriteFile(markerPath, []byte(markerContent), 0o644) //nolint:gosec // workspace marker is not sensitive
	}

	return worktreePath, nil
}
