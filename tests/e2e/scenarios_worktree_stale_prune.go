package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/grovetools/tend/pkg/fs"
	"github.com/grovetools/tend/pkg/git"
	"github.com/grovetools/tend/pkg/harness"

	"github.com/grovetools/core/pkg/workspace"
)

// runGit runs a git subcommand in dir and returns combined output, failing the
// scenario with context on error.
func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git %s in %s: %w: %s", strings.Join(args, " "), dir, err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// pruneDryRun returns the `git worktree prune --dry-run -v` output for dir; a
// non-empty result means a stale/dangling worktree registration exists.
func pruneDryRun(dir string) (string, error) {
	out, err := runGit(dir, "worktree", "prune", "--dry-run", "-v")
	return strings.TrimSpace(out), err
}

// WorktreeStalePruneScenario is the tend regression for the inbox bug
// "worktree create silently yields incomplete ecosystem on stale git-worktree
// state". A member repo carries a stale/dangling git-worktree registration left
// by an `rm -rf` cleanup that never ran `git worktree prune` (git reports
// "gitdir file points to non-existent location"). Before the fix,
// SetupSubmodules' `git worktree add` for that member failed, the failure was
// swallowed, and the member was silently dropped — yielding an incomplete,
// non-hermetic container with NO error.
//
// This scenario seeds exactly that stale state, runs worktree creation, and
// asserts the resulting container is COMPLETE (the pre-create prune cleared the
// stale entry). A second step asserts that a member which genuinely cannot be
// added (branch already live in another worktree) surfaces a LOUD error naming
// the repo — never a silent incomplete container.
func WorktreeStalePruneScenario() *harness.Scenario {
	var ecoDir, memberDir string

	return &harness.Scenario{
		Name:        "worktree-create-stale-prune",
		Description: "Pre-create prune clears stale member registrations so the container is complete; a genuine add failure fails loud naming the repo.",
		Tags:        []string{"core", "workspace", "worktree", "regression"},
		Steps: []harness.Step{
			{
				Name: "Setup ecosystem-of-repos with a member carrying a stale worktree registration",
				Func: func(ctx *harness.Context) error {
					homeDir := ctx.HomeDir()
					workDir := filepath.Join(homeDir, "work")
					if err := fs.CreateDir(workDir); err != nil {
						return err
					}

					// Ecosystem root holds member repos as direct-child dirs (the
					// real `flow plan init --worktree` ecosystem-of-repos case).
					ecoDir = filepath.Join(workDir, "eco")
					if err := fs.WriteString(filepath.Join(ecoDir, "grove.toml"), "workspaces = [\"*\"]\n"); err != nil {
						return err
					}
					ecoRepo, err := git.SetupTestRepo(ecoDir)
					if err != nil {
						return err
					}
					if err := ecoRepo.AddCommit("ecosystem root"); err != nil {
						return err
					}

					// Member repo as a direct child of the ecosystem root.
					memberDir = filepath.Join(ecoDir, "member")
					if err := fs.WriteString(filepath.Join(memberDir, "README.md"), "member\n"); err != nil {
						return err
					}
					memberRepo, err := git.SetupTestRepo(memberDir)
					if err != nil {
						return err
					}
					if err := memberRepo.AddCommit("member initial"); err != nil {
						return err
					}

					// SEED a stale/dangling worktree registration in the member,
					// exactly as a prior `rm -rf` cleanup leaves it: create a
					// worktree, then remove its directory WITHOUT prune/remove.
					staleWtDir := filepath.Join(homeDir, "stale-member-wt")
					if _, err := runGit(memberDir, "worktree", "add", "-b", "leftover-branch", staleWtDir); err != nil {
						return err
					}
					if err := os.RemoveAll(staleWtDir); err != nil {
						return err
					}
					stale, err := pruneDryRun(memberDir)
					if err != nil {
						return err
					}
					if stale == "" {
						return fmt.Errorf("precondition failed: expected a stale worktree registration in member after rm -rf, got none")
					}
					ctx.ShowCommandOutput("git -C member worktree prune --dry-run -v", stale, "")
					return nil
				},
			},
			{
				Name: "SetupSubmodules prunes the stale entry and yields a COMPLETE container",
				Func: func(ctx *harness.Context) error {
					container := filepath.Join(ctx.HomeDir(), "container")
					if _, err := runGit(ecoDir, "worktree", "add", "-b", "feature-branch", container); err != nil {
						return err
					}

					provider := newProviderForLocalWorkspaces(map[string]string{"member": memberDir})

					err := workspace.SetupSubmodules(context.Background(), container, ecoDir, "feature-branch", []string{"member"}, provider)
					if err := ctx.Check("SetupSubmodules succeeds with stale member registration", err); err != nil {
						return err
					}

					// Container must be COMPLETE: the member's linked worktree exists.
					if err := fs.AssertExists(filepath.Join(container, "member", ".git")); err != nil {
						return fmt.Errorf("container is incomplete: member worktree missing (non-hermetic): %w", err)
					}

					// The stale entry must be cleared by the pre-create prune.
					stale, err := pruneDryRun(memberDir)
					if err != nil {
						return err
					}
					if stale != "" {
						return fmt.Errorf("stale registration not cleared by pre-create prune: %s", stale)
					}
					return nil
				},
			},
			{
				Name: "A member that cannot be added fails LOUD and names the repo",
				Func: func(ctx *harness.Context) error {
					// New ecosystem + member; pin the member's target branch to a
					// live worktree so `git worktree add -B feature-branch` for the
					// member genuinely fails (branch already in use). Prune cannot
					// clear a live worktree.
					eco2 := filepath.Join(ctx.HomeDir(), "work", "eco2")
					if err := fs.WriteString(filepath.Join(eco2, "grove.toml"), "workspaces = [\"*\"]\n"); err != nil {
						return err
					}
					eco2Repo, err := git.SetupTestRepo(eco2)
					if err != nil {
						return err
					}
					if err := eco2Repo.AddCommit("eco2 root"); err != nil {
						return err
					}

					member2 := filepath.Join(eco2, "member")
					if err := fs.WriteString(filepath.Join(member2, "README.md"), "member2\n"); err != nil {
						return err
					}
					member2Repo, err := git.SetupTestRepo(member2)
					if err != nil {
						return err
					}
					if err := member2Repo.AddCommit("member2 initial"); err != nil {
						return err
					}

					liveWt := filepath.Join(ctx.HomeDir(), "live-feature-branch")
					if _, err := runGit(member2, "worktree", "add", "-b", "feature-branch", liveWt); err != nil {
						return err
					}

					container2 := filepath.Join(ctx.HomeDir(), "container2")
					if _, err := runGit(eco2, "worktree", "add", "-b", "feature-branch", container2); err != nil {
						return err
					}

					provider := newProviderForLocalWorkspaces(map[string]string{"member": member2})
					err = workspace.SetupSubmodules(context.Background(), container2, eco2, "feature-branch", []string{"member"}, provider)
					if err == nil {
						return fmt.Errorf("expected a loud error for an un-addable member, got nil (silent incomplete container)")
					}
					if !strings.Contains(err.Error(), "member") {
						return fmt.Errorf("error must name the dropped repo, got: %v", err)
					}
					if !strings.Contains(err.Error(), "incomplete") {
						return fmt.Errorf("error must flag the container as incomplete, got: %v", err)
					}
					ctx.AddAssertion("un-addable member surfaces a loud, named error", nil)
					return nil
				},
			},
		},
	}
}

// newProviderForLocalWorkspaces builds a workspace.Provider whose local
// workspaces are the given name->path map, mirroring discovery output for the
// fixture so SetupSubmodules can resolve members without a real disk scan.
func newProviderForLocalWorkspaces(workspaces map[string]string) *workspace.Provider {
	var projects []workspace.Project
	for name, path := range workspaces {
		projects = append(projects, workspace.Project{
			Name: name,
			Path: path,
			Workspaces: []workspace.DiscoveredWorkspace{
				{
					Name:              "main",
					Path:              path,
					Type:              workspace.WorkspaceTypePrimary,
					ParentProjectPath: path,
				},
			},
		})
	}
	return workspace.NewProvider(&workspace.DiscoveryResult{Projects: projects})
}
