package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/pkg/claudetrust"
	"github.com/grovetools/core/pkg/pitrust"
	"github.com/grovetools/core/pkg/worktreeregistry"
	"github.com/grovetools/core/util/pathutil"
)

// Prepare creates or gets a fully configured worktree.
func Prepare(ctx context.Context, opts PrepareOptions, setupHandlers ...func(worktreePath, gitRoot string) error) (string, error) {
	// Centralized safeguard: check if the git root is a notebook repo.
	if IsNotebookRepo(opts.GitRoot) {
		return "", fmt.Errorf("cannot create project worktree inside a notebook repository located at %s. Run this command from your project directory", opts.GitRoot)
	}

	// Reject anything that is not a pure relative name BEFORE it reaches
	// filepath.Join. An absolute path arriving here (a caller passing an
	// already-resolved worktree PATH where a name belongs) does not replace the
	// base — Join concatenates it — so MkdirAll below would otherwise
	// materialize a deep synthetic tree inside the container base.
	if err := ValidateWorktreeName(opts.WorktreeName); err != nil {
		return "", err
	}

	// Every worktree container is a synthetic directory that holds its 1..N repos
	// as subdirs, each checked out as its own linked git worktree by
	// SetupSubmodules. The container itself has NO top-level .git — it is never a
	// git worktree of the ecosystem superrepo, for anchored AND non-anchored
	// ecosystem worktrees alike. This is deliberate: a superrepo worktree tracks
	// submodule gitlinks, which forces submodule bumps and blocks clean per-repo
	// rebasing (the whole point of this container shape). The synthetic root
	// grove.toml with `workspaces = ["*"]` is what makes discovery classify the
	// container as an ecosystem worktree (see classifyWorkspaceRoot in
	// discover.go); the `.grove/workspace` marker's owner: key (written below)
	// records whether the owner is the ecosystem root (non-anchored) or a
	// sub-repo (anchored), which is how classification resolves the parent
	// ecosystem (see GetProjectByPath in lookup.go).
	base := WorktreeBase(opts.GitRoot, opts.UseXDGWorktrees)
	target := ResolveNewWorktreePath(opts.GitRoot, opts.WorktreeName, opts.UseXDGWorktrees)

	worktreePath := target
	var created bool
	// rollbackRoot is the SHALLOWEST directory this call creates for this
	// worktree. Removing worktreePath alone on failure leaks every intermediate
	// directory MkdirAll synthesized (branch-style names nest), and those
	// leftovers are then adopted as real worktrees by registry reconciliation
	// and discovery. It is always at or below base, so a concurrent Prepare's
	// sibling worktree is never in the blast radius.
	var rollbackRoot string
	if _, statErr := os.Stat(worktreePath); os.IsNotExist(statErr) {
		// The shared base is created separately and never rolled back.
		if err := os.MkdirAll(base, 0o755); err != nil {
			return "", fmt.Errorf("failed to create worktree base: %w", err)
		}
		rollbackRoot = shallowestMissing(base, worktreePath)
		if err := os.MkdirAll(worktreePath, 0o755); err != nil {
			return "", fmt.Errorf("failed to create worktree container: %w", err)
		}
		if err := os.WriteFile(filepath.Join(worktreePath, "grove.toml"), []byte("workspaces = [\"*\"]\n"), 0o644); err != nil { //nolint:gosec // synthetic container config is not sensitive
			return "", fmt.Errorf("failed to write synthetic grove.toml: %w", err)
		}
		created = true
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
			// Remove the poisoned, half-provisioned container before returning.
			// Without this, the empty/partial dir survives on disk; the NEXT
			// Prepare sees it via os.Stat (created==false), SKIPS SetupSubmodules
			// entirely, and returns a silently-incomplete worktree with no error —
			// the exact "second run succeeds with a broken container" bug. We only
			// created worktreePath in THIS call (created==true here), so removing
			// it is safe; any member linked-worktrees left inside are cleared by
			// the pre-add `git worktree prune` on the next attempt.
			//
			// Remove rollbackRoot, not worktreePath: for a nested (branch-style)
			// name they differ, and leaving the intermediate directories behind
			// is what turns a transient failure into a permanent phantom
			// worktree (reconcile adopts it, skills/settings sync provision it).
			_ = os.RemoveAll(rollbackRoot)
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
		//
		// Gate: grove only touches ~/.claude.json when this worktree's resolved
		// [claude] profile sets manageTrust=true (default off, opt-in). The gate
		// is per-worktree by design — seeding has the worktree's own cascade in
		// hand, so a per-project manageTrust legitimately enables its own trust.
		// This is resolved INDEPENDENTLY of ShouldSeed (a manageTrust-only block
		// is ShouldSeed()==false), and it also short-circuits the EPERM→daemon
		// delegation fallback below.
		if WorktreeManagesTrust(absWorktreePath, opts.SiblingWorkspaces) {
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
		}

		// Relocate the owner checkout's exec-trust decision onto this worktree's
		// member repos. The exec-provenance gate keys trust on (config file
		// PATH, digest), so without this every worktree re-asks about config
		// the user already reviewed in the owner checkout — ~N repos per
		// worktree, forever. Inheritance is granted ONLY where the worktree's
		// grove.toml carries byte-identical exec values (same digest) as the
		// owner's, so a branch that edited a hook inherits nothing and the gate
		// stays shut, exactly as the digest binding intends. See
		// core/config/exectrust_inherit.go.
		//
		// Gated on [security] inherit_worktree_trust, read from user-controlled
		// layers only (default on). Best-effort like the trust seeds above: a
		// failure here costs approval prompts, never the worktree.
		if config.InheritWorktreeTrustEnabled(absWorktreePath) {
			candidates := config.WorktreeInheritCandidates(ownerPath, absWorktreePath, opts.SiblingWorkspaces)
			if outcomes, inheritErr := config.InheritExecTrust(candidates, true); inheritErr != nil {
				fmt.Printf("Warning: failed to record inherited exec-trust: %v\n", inheritErr)
			} else if n := config.InheritGrantedCount(outcomes); n > 0 {
				logger.WithField("count", n).Debug("Inherited exec-trust from owner checkout")
			}
		}

		// Pre-seed pi (coding agent) project trust for the container path,
		// best-effort. Unlike Claude's per-exact-path trust, pi's lookup walks
		// up to the nearest decided ancestor (trust-manager.ts in the pi
		// source), so the container alone covers every member-repo subdir.
		// This is seeded alongside Claude but NOT behind the [claude]
		// manageTrust gate: pitrust gates itself on ~/.pi/agent existing (pi
		// actually installed) and GROVE_PRESEED_PI_TRUST. It matters most for
		// headless pi, which silently skips the trust prompt and treats an
		// undecided project as untrusted, never loading project .pi/
		// resources. No daemon fallback exists for pi (the trust/seed RPC is
		// claude-specific), so a sandbox-side EPERM only costs a warning.
		if piPath, piErr := pathutil.CanonicalPath(absWorktreePath); piErr == nil {
			if seedErr := pitrust.SeedTrust(piPath); seedErr != nil {
				fmt.Printf("Warning: failed to pre-seed pi trust: %v\n", seedErr)
			}
			if seedErr := pitrust.SeedTrustForConfigDir(".grove-agent", piPath); seedErr != nil {
				fmt.Printf("Warning: failed to pre-seed grove-agent trust: %v\n", seedErr)
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

// shallowestMissing returns the highest directory at or under base, on the path
// from base to target, that does not yet exist — i.e. the shallowest directory
// a subsequent MkdirAll(target) will create. When every level already exists it
// returns target, so callers can always remove the result unconditionally.
//
// base itself is never returned: it is the shared container base and may hold
// sibling worktrees created by other callers.
func shallowestMissing(base, target string) string {
	rel, err := filepath.Rel(base, target)
	if err != nil || rel == "." || rel == ".." ||
		strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		// target is not under base (shouldn't happen for a validated name) —
		// fall back to the leaf, which is always safe to remove.
		return target
	}
	cur := base
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		cur = filepath.Join(cur, part)
		if _, err := os.Stat(cur); os.IsNotExist(err) {
			return cur
		}
	}
	return target
}
