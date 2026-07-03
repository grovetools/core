package git

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/grovetools/core/command"
)

// WouldRebaseConflict predicts whether rebasing branchRef onto ontoRef would hit
// a merge conflict. It runs `git merge-tree --write-tree <ontoRef> <branchRef>`
// and reports true when the combined output mentions CONFLICT.
//
// This is a HEURISTIC, not a faithful rebase replay: merge-tree performs a
// single three-way merge of the two tips against their merge base, whereas a
// real rebase replays each commit in sequence. It can therefore both miss
// conflicts that only arise mid-replay and over-report conflicts a real rebase
// would resolve. Callers should treat the result as a best-effort prediction and
// still rely on the actual Rebase (plus AbortRebase) to surface the truth.
//
// branchRef defaults to "HEAD" when empty.
func WouldRebaseConflict(repoPath, ontoRef, branchRef string) (bool, error) {
	if branchRef == "" {
		branchRef = "HEAD"
	}

	cmdBuilder := command.NewSafeBuilder()
	cmd, err := cmdBuilder.Build(context.Background(), "git", "merge-tree", "--write-tree", ontoRef, branchRef)
	if err != nil {
		return false, fmt.Errorf("failed to build command: %w", err)
	}
	execCmd := cmd.Exec()
	execCmd.Dir = repoPath
	// merge-tree exits non-zero when the merge has conflicts, so we cannot treat
	// a non-zero exit as a hard error; scan the combined output instead.
	output, _ := execCmd.CombinedOutput()
	return strings.Contains(string(output), "CONFLICT"), nil
}

// TouchedFilesSinceMergeBase returns the files the current branch's commits
// changed relative to the merge-base with ontoRef — `git diff --name-only
// <ontoRef>...HEAD` (three-dot: only what the branch touched, not what ontoRef
// moved on to). Paths are sorted for deterministic output. A caller that passes
// an empty ontoRef has a bug, so this returns an error rather than silently
// diffing against the working tree.
func TouchedFilesSinceMergeBase(repoPath, ontoRef string) ([]string, error) {
	if ontoRef == "" {
		return nil, fmt.Errorf("TouchedFilesSinceMergeBase: empty ontoRef")
	}

	cmdBuilder := command.NewSafeBuilder()
	cmd, err := cmdBuilder.Build(context.Background(), "git", "diff", "--name-only", ontoRef+"...HEAD")
	if err != nil {
		return nil, fmt.Errorf("failed to build command: %w", err)
	}
	execCmd := cmd.Exec()
	execCmd.Dir = repoPath
	output, err := execCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to diff %s...HEAD: %w", ontoRef, err)
	}
	var files []string
	for _, line := range strings.Split(string(output), "\n") {
		if f := strings.TrimSpace(line); f != "" {
			files = append(files, f)
		}
	}
	sort.Strings(files)
	return files, nil
}

// MergeTreeConflictFiles returns the paths `git merge-tree --write-tree`
// predicts would conflict when merging branchRef into ontoRef — the file-level
// detail behind WouldRebaseConflict's boolean, under the same single
// three-way-merge heuristic (see that function's caveats). A clean merge (or a
// repo where the prediction cannot run) yields nil.
//
// It runs `git merge-tree --write-tree --name-only -z` (git ≥ 2.40): the NUL-
// separated output is the toplevel tree OID, then one token per conflicted
// path, then an empty token separating the informational section. merge-tree
// exits non-zero when the merge conflicts, so a non-zero exit is not treated as
// an error. branchRef defaults to "HEAD" when empty.
func MergeTreeConflictFiles(repoPath, ontoRef, branchRef string) ([]string, error) {
	if ontoRef == "" {
		return nil, fmt.Errorf("MergeTreeConflictFiles: empty ontoRef")
	}
	if branchRef == "" {
		branchRef = "HEAD"
	}

	cmdBuilder := command.NewSafeBuilder()
	cmd, err := cmdBuilder.Build(context.Background(), "git", "merge-tree", "--write-tree", "--name-only", "-z", ontoRef, branchRef)
	if err != nil {
		return nil, fmt.Errorf("failed to build command: %w", err)
	}
	execCmd := cmd.Exec()
	execCmd.Dir = repoPath
	// Stdout only (the informational section shares it); the conflict exit code
	// is expected, so the error is ignored and an unrunnable prediction simply
	// yields no paths.
	output, _ := execCmd.Output()

	tokens := strings.Split(string(output), "\x00")
	if len(tokens) < 2 {
		return nil, nil
	}
	var files []string
	seen := make(map[string]bool)
	// tokens[0] is the tree OID; conflicted paths follow until the empty token
	// that separates the informational section.
	for _, tok := range tokens[1:] {
		if tok == "" {
			break
		}
		if !seen[tok] {
			seen[tok] = true
			files = append(files, tok)
		}
	}
	sort.Strings(files)
	return files, nil
}

// ListLocalBranches returns the repo's local branch names (refs/heads), in git's
// default ordering. It runs `git for-each-ref --format='%(refname:short)'
// refs/heads`, the canonical way to enumerate local branches without parsing the
// porcelain `git branch` output. Blank lines are dropped. It is the shared helper
// behind the Rebase page's ref picker, which unions the branches across the
// in-scope repos.
func ListLocalBranches(repoPath string) ([]string, error) {
	cmdBuilder := command.NewSafeBuilder()
	cmd, err := cmdBuilder.Build(context.Background(), "git", "for-each-ref", "--format=%(refname:short)", "refs/heads")
	if err != nil {
		return nil, fmt.Errorf("failed to build command: %w", err)
	}
	execCmd := cmd.Exec()
	execCmd.Dir = repoPath
	output, err := execCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list local branches: %w", err)
	}
	var branches []string
	for _, line := range strings.Split(string(output), "\n") {
		if name := strings.TrimSpace(line); name != "" {
			branches = append(branches, name)
		}
	}
	return branches, nil
}

// Rebase runs `git rebase <ontoRef>` in repoPath, replaying the current branch
// onto ontoRef. On failure (including conflicts) it returns an error carrying
// git's output; the repo is left mid-rebase, so callers that want a clean tree
// should follow a failure with AbortRebase.
func Rebase(repoPath, ontoRef string) error {
	cmdBuilder := command.NewSafeBuilder()
	cmd, err := cmdBuilder.Build(context.Background(), "git", "rebase", ontoRef)
	if err != nil {
		return fmt.Errorf("failed to build command: %w", err)
	}
	execCmd := cmd.Exec()
	execCmd.Dir = repoPath
	if output, err := execCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git rebase %s failed: %s", ontoRef, strings.TrimSpace(string(output)))
	}
	return nil
}

// DeleteRemoteBranch deletes branch from the origin remote by running
// `git push origin --delete <branch>` in repoPath. It is DESTRUCTIVE and
// OUTWARD-FACING — it removes the branch on the remote (e.g. GitHub), not just
// the local tracking ref — so callers MUST confirm intent before invoking. On
// failure it returns an error carrying git's output.
func DeleteRemoteBranch(repoPath, branch string) error {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return fmt.Errorf("cannot delete remote branch: empty branch name")
	}
	cmdBuilder := command.NewSafeBuilder()
	cmd, err := cmdBuilder.Build(context.Background(), "git", "push", "origin", "--delete", branch)
	if err != nil {
		return fmt.Errorf("failed to build command: %w", err)
	}
	execCmd := cmd.Exec()
	execCmd.Dir = repoPath
	if output, err := execCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git push origin --delete %s failed: %s", branch, strings.TrimSpace(string(output)))
	}
	return nil
}

// MainCheckoutPath resolves the path to a repo's MAIN (primary) checkout from
// any of its linked worktrees. It runs `git worktree list --porcelain` in
// repoPath and returns the first non-bare worktree — git always lists the main
// worktree first — which is the working copy where the local default branch
// (main/master) lives. When repoPath itself IS the main checkout it returns
// repoPath's own entry.
//
// This is the inverse of workspace.FindWorktreePath (which goes main → linked
// worktree): the Rebase page rows are linked worktrees, so landing needs the
// main checkout to fast-forward its default branch.
func MainCheckoutPath(repoPath string) (string, error) {
	cmdBuilder := command.NewSafeBuilder()
	cmd, err := cmdBuilder.Build(context.Background(), "git", "worktree", "list", "--porcelain")
	if err != nil {
		return "", fmt.Errorf("failed to build command: %w", err)
	}
	execCmd := cmd.Exec()
	execCmd.Dir = repoPath
	output, err := execCmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to list worktrees: %w", err)
	}
	if path := firstNonBareWorktree(string(output)); path != "" {
		return path, nil
	}
	return "", fmt.Errorf("could not resolve main checkout for %s", repoPath)
}

// firstNonBareWorktree extracts the path of the first non-bare worktree from
// `git worktree list --porcelain` output. Entries are separated by blank lines;
// each begins with a "worktree <path>" line and a bare worktree carries a
// standalone "bare" line. The main worktree is always listed first.
func firstNonBareWorktree(output string) string {
	var path string
	bare := false
	flush := func() string {
		if path != "" && !bare {
			return path
		}
		path, bare = "", false
		return ""
	}
	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			if p := flush(); p != "" {
				return p
			}
			continue
		}
		switch {
		case strings.HasPrefix(line, "worktree "):
			path = strings.TrimSpace(strings.TrimPrefix(line, "worktree "))
		case line == "bare":
			bare = true
		}
	}
	return flush()
}

// AdvanceMainToBranch lands a caught-up worktree branch into the repo's default
// branch. In the MAIN checkout it checks out defaultBranch and fast-forwards it
// to branch (`merge --ff-only`); when worktreePath is non-empty it then resyncs
// that worktree's working copy to the advanced default (`reset --hard`). This
// mirrors flow's planutil.RebaseAndMergeRepo without importing flow.
//
// The ff-only merge is the safety gate: it SUCCEEDS only when branch already
// contains defaultBranch (i.e. branch was rebased/caught up onto it first), so a
// branch that has diverged is refused with a clear error rather than creating a
// merge commit or moving main backward. Callers MUST confirm intent first — this
// mutates the shared default branch.
func AdvanceMainToBranch(mainRepoPath, branch, defaultBranch, worktreePath string) error {
	branch = strings.TrimSpace(branch)
	defaultBranch = strings.TrimSpace(defaultBranch)
	if mainRepoPath == "" {
		return fmt.Errorf("cannot advance main: empty main checkout path")
	}
	if branch == "" || branch == "HEAD" {
		return fmt.Errorf("cannot advance main: invalid branch %q", branch)
	}
	if defaultBranch == "" {
		return fmt.Errorf("cannot advance main: empty default branch")
	}

	cmdBuilder := command.NewSafeBuilder()
	run := func(dir string, args ...string) error {
		cmd, err := cmdBuilder.Build(context.Background(), "git", args...)
		if err != nil {
			return fmt.Errorf("failed to build command: %w", err)
		}
		execCmd := cmd.Exec()
		execCmd.Dir = dir
		if output, err := execCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %s failed: %s", strings.Join(args, " "), strings.TrimSpace(string(output)))
		}
		return nil
	}

	if err := run(mainRepoPath, "checkout", defaultBranch); err != nil {
		return err
	}
	if err := run(mainRepoPath, "merge", "--ff-only", branch); err != nil {
		return fmt.Errorf("fast-forward merge of %s into %s failed (is the branch caught up?): %w", branch, defaultBranch, err)
	}
	if worktreePath != "" {
		if err := run(worktreePath, "reset", "--hard", defaultBranch); err != nil {
			return fmt.Errorf("advanced %s but failed to resync worktree: %w", defaultBranch, err)
		}
	}
	return nil
}

// AbortRebase runs `git rebase --abort` in repoPath, restoring the branch to its
// pre-rebase state. It is the rollback for a Rebase that failed partway through.
func AbortRebase(repoPath string) error {
	cmdBuilder := command.NewSafeBuilder()
	cmd, err := cmdBuilder.Build(context.Background(), "git", "rebase", "--abort")
	if err != nil {
		return fmt.Errorf("failed to build command: %w", err)
	}
	execCmd := cmd.Exec()
	execCmd.Dir = repoPath
	if output, err := execCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git rebase --abort failed: %s", strings.TrimSpace(string(output)))
	}
	return nil
}
