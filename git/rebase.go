package git

import (
	"context"
	"fmt"
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
