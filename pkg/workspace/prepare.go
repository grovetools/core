package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/grovetools/core/git"
	"github.com/grovetools/core/pkg/claudetrust"
	"github.com/grovetools/core/pkg/worktreeregistry"
	"github.com/grovetools/core/util/pathutil"
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

	// Determine whether the source root is an ecosystem. This forks how the
	// container is created: an ecosystem worktree's container IS a git worktree
	// of the ecosystem root, whereas a standalone repo's container is a plain
	// synthetic directory holding the repo as a subdir (the child's git worktree
	// is created INTO it by SetupSubmodules). The synthetic root grove.toml with
	// `workspaces = ["*"]` is what makes discovery classify the container as an
	// ecosystem worktree (see classifyWorkspaceRoot in discover.go).
	isEcosystem := false
	if node, _ := GetProjectByPath(opts.GitRoot); node != nil {
		isEcosystem = node.IsEcosystem()
	}

	target := ResolveNewWorktreePath(opts.GitRoot, opts.WorktreeName, opts.UseXDGWorktrees)

	var worktreePath string
	var created bool
	if isEcosystem {
		wm := git.NewWorktreeManager()
		var err error
		worktreePath, created, err = wm.GetOrPrepareWorktreeAt(ctx, opts.GitRoot, target, opts.BranchName)
		if err != nil {
			return "", fmt.Errorf("failed to prepare worktree: %w", err)
		}
	} else {
		worktreePath = target
		if _, statErr := os.Stat(worktreePath); os.IsNotExist(statErr) {
			if err := os.MkdirAll(worktreePath, 0o755); err != nil {
				return "", fmt.Errorf("failed to create worktree container: %w", err)
			}
			if err := os.WriteFile(filepath.Join(worktreePath, "grove.toml"), []byte("workspaces = [\"*\"]\n"), 0o644); err != nil { //nolint:gosec // synthetic container config is not sensitive
				return "", fmt.Errorf("failed to write synthetic grove.toml: %w", err)
			}
			created = true
		}
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
			// Propagate hard: an explicitly-requested sibling repo that can't be
			// set up means the resulting worktree would be silently incomplete
			// (a non-ecosystem or missing-repo worktree). Fail loudly so the
			// caller (flow plan init) exits non-zero rather than producing a
			// half-wired worktree.
			return "", fmt.Errorf("failed to setup submodules for worktree '%s': %w", opts.WorktreeName, err)
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

		// Every worktree is now a unified container holding 1..N repos as
		// subdirs, so the marker always records the repos: list and is always
		// ecosystem: true. opts.SiblingWorkspaces is non-empty here (the caller
		// seeds it with the standalone repo's own name when no siblings are
		// requested) and is already resolved (no __ALL__ sentinel).
		//
		// The ecosystem:/repos: keys below are a frozen persisted format —
		// keep them verbatim. owner: is an additive key recording the owning
		// repository root so deleted (zombie) worktrees stay owner-resolvable
		// after their .git file is gone (see WorktreeOwner).
		ownerPath := opts.GitRoot
		if abs, err := filepath.Abs(opts.GitRoot); err == nil {
			ownerPath = abs
		}
		markerContent := fmt.Sprintf("branch: %s\nplan: %s\ncreated_at: %s\nowner: %s\necosystem: true\n",
			opts.BranchName, opts.PlanName, time.Now().UTC().Format(time.RFC3339), ownerPath)

		markerContent += "repos:\n"
		for _, repo := range opts.SiblingWorkspaces {
			markerContent += fmt.Sprintf("  - %s\n", repo)
		}

		_ = os.WriteFile(markerPath, []byte(markerContent), 0o644) //nolint:gosec // workspace marker is not sensitive

		// Upsert registry entry for this new worktree.
		absWorktreePath := worktreePath
		if abs, absErr := filepath.Abs(worktreePath); absErr == nil {
			absWorktreePath = abs
		}
		regEntry := &worktreeregistry.Entry{
			AbsPath:   absWorktreePath,
			Owner:     ownerPath,
			Repos:     opts.SiblingWorkspaces,
			Plan:      opts.PlanName,
			CreatedAt: time.Now().UTC(),
		}
		if saveErr := worktreeregistry.Save(regEntry); saveErr != nil {
			fmt.Printf("Warning: failed to write registry entry for worktree '%s': %v\n", opts.WorktreeName, saveErr)
		}

		// Pre-seed Claude Code folder-trust so agents launched inside this
		// worktree don't stall at the interactive trust prompt. Trust is
		// per-exact-path, and flow scopes an agent's cwd to either the
		// container or a <worktree>/<repo> subdir, so seed both. Every key MUST
		// be canonicalized with pathutil.CanonicalPath: flow runs each cwd
		// through CanonicalPath before handing it to Claude (macOS case +
		// symlinks), so an un-canonicalized key would silently miss. The dirs
		// exist on disk here (SetupSubmodules already ran), so canonicalization
		// resolves real case/symlinks. Never abort worktree creation on failure.
		trustPaths := make([]string, 0, 1+len(opts.SiblingWorkspaces))
		trustPaths = append(trustPaths, absWorktreePath)
		for _, repo := range opts.SiblingWorkspaces {
			trustPaths = append(trustPaths, filepath.Join(absWorktreePath, repo))
		}
		canonicalPaths := make([]string, 0, len(trustPaths))
		for _, p := range trustPaths {
			canonical, canonErr := pathutil.CanonicalPath(p)
			if canonErr != nil {
				fmt.Printf("Warning: failed to canonicalize path for Claude trust pre-seed (%s): %v\n", p, canonErr)
				continue
			}
			canonicalPaths = append(canonicalPaths, canonical)
		}
		if seedErr := claudetrust.SeedTrust(canonicalPaths...); seedErr != nil {
			// ~/.claude.json sits OUTSIDE the OS sandbox's writable boundary
			// (roughly working-dir + temp), so when Prepare runs sandbox-side the
			// write is rejected with EPERM. Delegate the privileged write to the
			// unsandboxed daemon, which re-derives the trust path set from the
			// registry entry saved above (never from caller-supplied paths). The
			// registry Save already ran, so the daemon can resolve absWorktreePath.
			if claudetrust.IsPermissionDenied(seedErr) && opts.TrustSeedFallback != nil {
				if rpcErr := opts.TrustSeedFallback(ctx, absWorktreePath); rpcErr != nil {
					fmt.Printf("Warning: failed to pre-seed Claude trust via daemon: %v\n", rpcErr)
				}
			} else {
				fmt.Printf("Warning: failed to pre-seed Claude trust: %v\n", seedErr)
			}
		}

		// Seed the worktree's .claude/settings.local.json with the union of
		// every member repo's paired-notebook directory, so flow agents can
		// READ (no prompt) and WRITE (under /sandbox) the out-of-tree notebooks
		// where briefings/plans/concepts/.artifacts live. opts.SiblingWorkspaces
		// is the linked member-repo set; the provider (discovered above) carries
		// the anchored-worktree NotebookName mapping. Best-effort: never abort
		// worktree creation on failure.
		if seedErr := SeedNotebookDirsForWorktree(worktreePath, opts.SiblingWorkspaces, provider); seedErr != nil {
			fmt.Printf("Warning: failed to seed notebook dirs into worktree settings: %v\n", seedErr)
		}

		// Seed the [claude] grove.toml profile (permissions.allow + sandbox
		// settings) into the same .claude/settings.local.json, unioning every
		// member repo's [claude] block. Best-effort: never abort worktree
		// creation on failure.
		if seedErr := SeedClaudeSettingsForWorktree(worktreePath, opts.SiblingWorkspaces, provider); seedErr != nil {
			fmt.Printf("Warning: failed to seed claude settings into worktree settings: %v\n", seedErr)
		}
	}

	return worktreePath, nil
}
