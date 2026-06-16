package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/grovetools/tend/pkg/fs"
	"github.com/grovetools/tend/pkg/git"
	"github.com/grovetools/tend/pkg/harness"

	"github.com/grovetools/core/pkg/workspace"
)

// headCommit returns the full HEAD commit SHA of the git repo at dir.
func headCommit(dir string) (string, error) {
	out, err := runGit(dir, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// AnchoredMemberResolutionScenario is the tend regression for the
// `flow plan init <name> --worktree --anchor <subrepo>` Frankenstein-container
// bug. When an anchored container is built (GitRoot = the anchor sub-repo), each
// member repo's SOURCE checkout was resolved via name-based workspace discovery
// (LocalWorkspaces), which returns an ARBITRARY checkout per repo when several
// exist on disk (the normal case: the main ecosystem checkout PLUS other
// ecosystem worktrees / leftover worktrees). The new container then snapshotted
// a MIX of commits from unrelated branches/efforts and did not build.
//
// The contract: a new ecosystem worktree must snapshot the CURRENT STATE OF THE
// ECOSYSTEM — each member at the commit the ECOSYSTEM ROOT currently has it at.
// `--anchor` changes only base-dir placement, NEVER which commit a member starts
// from.
//
// This scenario reproduces the TRIGGERING CONDITION (two checkouts of the same
// member repo at DIFFERENT commits) and asserts every member in the new
// anchored container is at the ECOSYSTEM ROOT's commit, not the other checkout's.
// It FAILS before the canonical-member-resolution fix and PASSES after.
func AnchoredMemberResolutionScenario() *harness.Scenario {
	var (
		ecoDir         string
		anchorDir      string
		memberDir      string // canonical member checkout under the ecosystem root
		rogueMemberDir string // a SECOND checkout of the same member, different commit
		ecoMemberSHA   string // the ecosystem root's member commit (expected)
		rogueMemberSHA string // the other checkout's member commit (must NOT be used)
	)

	return &harness.Scenario{
		Name:        "anchored-member-canonical-resolution",
		Description: "An anchored container (gitRoot = sub-repo) snapshots each member at the ECOSYSTEM ROOT's commit, never an arbitrary second checkout that exists on disk.",
		Tags:        []string{"core", "workspace", "worktree", "anchor", "regression"},
		Steps: []harness.Step{
			{
				Name: "Setup ecosystem + anchor sub-repo + member, plus a SECOND member checkout at a different commit",
				Func: func(ctx *harness.Context) error {
					homeDir := ctx.HomeDir()
					workDir := filepath.Join(homeDir, "work")
					if err := fs.CreateDir(workDir); err != nil {
						return err
					}

					// Ecosystem root holds the anchor sub-repo and the member repo
					// as direct-child dirs (the real ecosystem-of-repos case).
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

					// Anchor sub-repo (the --anchor target): a direct child of the
					// ecosystem root. This becomes gitRoot for the anchored create.
					anchorDir = filepath.Join(ecoDir, "anchor")
					if err := fs.WriteString(filepath.Join(anchorDir, "README.md"), "anchor\n"); err != nil {
						return err
					}
					anchorRepo, err := git.SetupTestRepo(anchorDir)
					if err != nil {
						return err
					}
					if err := anchorRepo.AddCommit("anchor initial"); err != nil {
						return err
					}

					// Member repo (the canonical checkout) as a direct child of the
					// ecosystem root, at the EXPECTED commit.
					memberDir = filepath.Join(ecoDir, "member")
					if err := fs.WriteString(filepath.Join(memberDir, "VERSION"), "ecosystem-canonical\n"); err != nil {
						return err
					}
					memberRepo, err := git.SetupTestRepo(memberDir)
					if err != nil {
						return err
					}
					if err := memberRepo.AddCommit("member canonical (ecosystem-root commit)"); err != nil {
						return err
					}
					ecoMemberSHA, err = headCommit(memberDir)
					if err != nil {
						return err
					}

					// A SECOND, ROGUE checkout of the SAME member repo elsewhere on
					// disk, advanced to a DIFFERENT commit. This is the triggering
					// condition: name-based discovery (LocalWorkspaces) can return
					// THIS arbitrary checkout instead of the canonical one. We clone
					// from the canonical member so it is the same logical repo, then
					// add a divergent commit.
					rogueMemberDir = filepath.Join(homeDir, "rogue", "member")
					if err := fs.CreateDir(filepath.Dir(rogueMemberDir)); err != nil {
						return err
					}
					if _, err := runGit(filepath.Dir(rogueMemberDir), "clone", memberDir, rogueMemberDir); err != nil {
						return err
					}
					if _, err := runGit(rogueMemberDir, "config", "user.email", "test@example.com"); err != nil {
						return err
					}
					if _, err := runGit(rogueMemberDir, "config", "user.name", "Test User"); err != nil {
						return err
					}
					if err := fs.WriteString(filepath.Join(rogueMemberDir, "VERSION"), "ROGUE-feature-commit\n"); err != nil {
						return err
					}
					if _, err := runGit(rogueMemberDir, "add", "."); err != nil {
						return err
					}
					if _, err := runGit(rogueMemberDir, "commit", "-m", "ROGUE divergent commit (must NOT be snapshotted)"); err != nil {
						return err
					}
					rogueMemberSHA, err = headCommit(rogueMemberDir)
					if err != nil {
						return err
					}

					if ecoMemberSHA == rogueMemberSHA {
						return fmt.Errorf("precondition failed: canonical and rogue member commits are identical (%s); the two-checkout condition was not established", ecoMemberSHA)
					}
					ctx.ShowCommandOutput("member commits",
						fmt.Sprintf("ecosystem-root member @ %s\nrogue checkout member @ %s", ecoMemberSHA, rogueMemberSHA), "")
					return nil
				},
			},
			{
				Name: "Anchored SetupSubmodules (gitRoot = anchor sub-repo) snapshots member at the ECOSYSTEM ROOT's commit",
				Func: func(ctx *harness.Context) error {
					// Anchored container: gitRoot is the ANCHOR SUB-REPO, exactly as
					// `flow plan init --anchor anchor` passes it. The container dir
					// is a plain synthetic dir (workspaces=["*"]) — not a worktree of
					// the anchor — matching the non-ecosystem standalone-root path in
					// Prepare for an anchored create.
					container := filepath.Join(ctx.HomeDir(), "anchored-container")
					if err := fs.WriteString(filepath.Join(container, "grove.toml"), "workspaces = [\"*\"]\n"); err != nil {
						return err
					}

					// Provider mirrors discovery having resolved the member NAME to the
					// ROGUE checkout — the exact pre-fix failure mode. In the field,
					// multiple checkouts of `member` exist on disk and the
					// collision-prone LocalWorkspaces() name map keeps an ARBITRARY one;
					// here we pin the `member` node to the rogue checkout so the pre-fix
					// path (filepath.Join(gitRoot,"member") miss → localWorkspaces
					// ["member"]) deterministically picks the WRONG commit. The post-fix
					// path resolves the member against the CANONICAL ecosystem root
					// (<eco>/member), ignoring this rogue entry, and snapshots the right
					// commit.
					provider := newProviderForLocalWorkspaces(map[string]string{
						"anchor": anchorDir,
						"member": rogueMemberDir,
					})

					// gitRoot = anchorDir (the SUB-REPO), repos = both members.
					err := workspace.SetupSubmodules(context.Background(), container, anchorDir, "feature-branch",
						[]string{"anchor", "member"}, provider)
					if err := ctx.Check("anchored SetupSubmodules succeeds", err); err != nil {
						return err
					}

					// The member's linked worktree must exist (complete container).
					memberWtGit := filepath.Join(container, "member", ".git")
					if err := fs.AssertExists(memberWtGit); err != nil {
						return fmt.Errorf("container is incomplete: member worktree missing: %w", err)
					}

					// CRUX: the snapshotted member must be at the ECOSYSTEM ROOT's
					// commit, NOT the rogue checkout's commit. Before the fix the
					// member's source resolved to an arbitrary checkout, so the
					// container could carry rogueMemberSHA.
					gotSHA, err := headCommit(filepath.Join(container, "member"))
					if err != nil {
						return err
					}
					ctx.ShowCommandOutput("snapshotted member commit",
						fmt.Sprintf("got    %s\nwant   %s (ecosystem root)\nNOT    %s (rogue checkout)", gotSHA, ecoMemberSHA, rogueMemberSHA), "")

					if gotSHA == rogueMemberSHA {
						return fmt.Errorf("BUG REPRODUCED: anchored container snapshotted member at the ROGUE checkout commit %s; want the ECOSYSTEM ROOT commit %s", rogueMemberSHA, ecoMemberSHA)
					}
					if gotSHA != ecoMemberSHA {
						return fmt.Errorf("anchored container member @ %s; want the ECOSYSTEM ROOT commit %s", gotSHA, ecoMemberSHA)
					}

					// The anchor itself must be at the anchor sub-repo's commit.
					anchorWtGit := filepath.Join(container, "anchor", ".git")
					if err := fs.AssertExists(anchorWtGit); err != nil {
						return fmt.Errorf("container is incomplete: anchor worktree missing: %w", err)
					}
					anchorSHA, err := headCommit(anchorDir)
					if err != nil {
						return err
					}
					gotAnchorSHA, err := headCommit(filepath.Join(container, "anchor"))
					if err != nil {
						return err
					}
					if gotAnchorSHA != anchorSHA {
						return fmt.Errorf("anchored container anchor @ %s; want canonical anchor commit %s", gotAnchorSHA, anchorSHA)
					}

					ctx.AddAssertion("anchored container snapshots every member at the ecosystem root's commit", nil)
					return nil
				},
			},
		},
	}
}
