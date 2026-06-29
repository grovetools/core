package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeAndCommit writes content to filename in dir and commits it.
func writeAndCommit(t *testing.T, dir, filename, content, message string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644))
	runGitCommand(t, dir, "add", filename)
	runGitCommand(t, dir, "commit", "-m", message)
}

// setupRebaseRepo creates a repo on a "main" branch with one baseline commit and
// returns its path.
func setupRebaseRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	setupGitRepo(t, dir)
	// Pin the default branch name so the tests don't depend on the host's
	// init.defaultBranch (main vs master).
	runGitCommand(t, dir, "checkout", "-b", "main")
	writeAndCommit(t, dir, "shared.txt", "base\n", "baseline")
	return dir
}

func TestWouldRebaseConflict_Conflict(t *testing.T) {
	dir := setupRebaseRepo(t)

	// Feature edits the shared file...
	runGitCommand(t, dir, "checkout", "-b", "feature")
	writeAndCommit(t, dir, "shared.txt", "feature change\n", "feature edit")

	// ...and main edits the same line, so a rebase would conflict.
	runGitCommand(t, dir, "checkout", "main")
	writeAndCommit(t, dir, "shared.txt", "main change\n", "main edit")

	conflict, err := WouldRebaseConflict(dir, "main", "feature")
	require.NoError(t, err)
	assert.True(t, conflict, "overlapping edits should be predicted as a conflict")
}

func TestWouldRebaseConflict_Clean(t *testing.T) {
	dir := setupRebaseRepo(t)

	// Feature touches a different file than main, so no conflict is predicted.
	runGitCommand(t, dir, "checkout", "-b", "feature")
	writeAndCommit(t, dir, "feature.txt", "feature only\n", "feature file")

	runGitCommand(t, dir, "checkout", "main")
	writeAndCommit(t, dir, "main.txt", "main only\n", "main file")

	conflict, err := WouldRebaseConflict(dir, "main", "feature")
	require.NoError(t, err)
	assert.False(t, conflict, "disjoint edits should not be predicted as a conflict")
}

func TestListLocalBranches(t *testing.T) {
	dir := setupRebaseRepo(t) // starts on "main"
	runGitCommand(t, dir, "checkout", "-b", "feature")
	runGitCommand(t, dir, "checkout", "-b", "wip/experiment")

	branches, err := ListLocalBranches(dir)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"main", "feature", "wip/experiment"}, branches,
		"should enumerate every local branch by short name")
}

func TestRebase_Success(t *testing.T) {
	dir := setupRebaseRepo(t)

	runGitCommand(t, dir, "checkout", "-b", "feature")
	writeAndCommit(t, dir, "feature.txt", "feature only\n", "feature file")

	runGitCommand(t, dir, "checkout", "main")
	writeAndCommit(t, dir, "main.txt", "main only\n", "main file")

	runGitCommand(t, dir, "checkout", "feature")
	require.NoError(t, Rebase(dir, "main"))

	// After a clean rebase the feature branch is ahead of main and not behind.
	ahead, behind, err := GetCommitsDivergence(dir, "main", "HEAD")
	require.NoError(t, err)
	assert.Equal(t, 1, ahead)
	assert.Equal(t, 0, behind)
}

func TestRebase_ConflictThenAbort(t *testing.T) {
	dir := setupRebaseRepo(t)

	runGitCommand(t, dir, "checkout", "-b", "feature")
	writeAndCommit(t, dir, "shared.txt", "feature change\n", "feature edit")

	runGitCommand(t, dir, "checkout", "main")
	writeAndCommit(t, dir, "shared.txt", "main change\n", "main edit")

	runGitCommand(t, dir, "checkout", "feature")
	err := Rebase(dir, "main")
	require.Error(t, err, "conflicting rebase should fail")

	// The repo is mid-rebase; AbortRebase must restore a clean state.
	require.NoError(t, AbortRebase(dir))

	status, err := GetStatus(dir)
	require.NoError(t, err)
	assert.Equal(t, "feature", status.Branch)
	assert.False(t, status.IsDirty, "tree should be clean after abort")
}

func TestGetCommitsDivergence(t *testing.T) {
	dir := setupRebaseRepo(t)

	runGitCommand(t, dir, "checkout", "-b", "feature")
	writeAndCommit(t, dir, "a.txt", "a\n", "a")
	writeAndCommit(t, dir, "b.txt", "b\n", "b")

	runGitCommand(t, dir, "checkout", "main")
	writeAndCommit(t, dir, "c.txt", "c\n", "c")

	ahead, behind, err := GetCommitsDivergence(dir, "main", "feature")
	require.NoError(t, err)
	assert.Equal(t, 2, ahead, "feature has 2 commits main lacks")
	assert.Equal(t, 1, behind, "main has 1 commit feature lacks")
}

func TestDeleteRemoteBranch(t *testing.T) {
	// A bare repo stands in for the GitHub remote.
	remote := t.TempDir()
	runGitCommand(t, remote, "init", "--bare")

	dir := setupRebaseRepo(t)
	runGitCommand(t, dir, "remote", "add", "origin", remote)

	// Push a feature branch so origin/feature exists.
	runGitCommand(t, dir, "checkout", "-b", "feature")
	writeAndCommit(t, dir, "feature.txt", "feature\n", "feature work")
	runGitCommand(t, dir, "push", "origin", "feature")
	runGitCommand(t, dir, "fetch", "origin")
	require.True(t, HasRemoteBranch(dir, "feature"), "branch should exist on origin after push")

	// Delete it from the remote and confirm it is gone.
	require.NoError(t, DeleteRemoteBranch(dir, "feature"))
	runGitCommand(t, dir, "fetch", "origin", "--prune")
	assert.False(t, HasRemoteBranch(dir, "feature"), "branch should be gone from origin after delete")
}

func TestDeleteRemoteBranch_EmptyBranch(t *testing.T) {
	dir := setupRebaseRepo(t)
	assert.Error(t, DeleteRemoteBranch(dir, ""), "empty branch name must be refused")
	assert.Error(t, DeleteRemoteBranch(dir, "   "), "blank branch name must be refused")
}

func TestFirstNonBareWorktree(t *testing.T) {
	// Main first, then a linked worktree — the main path wins.
	out := "worktree /repos/foo\nHEAD abc\nbranch refs/heads/main\n\n" +
		"worktree /wt/foo-feature\nHEAD def\nbranch refs/heads/feature\n"
	assert.Equal(t, "/repos/foo", firstNonBareWorktree(out))

	// A leading bare entry (no checkout) is skipped in favor of the first
	// real working copy.
	bareFirst := "worktree /repos/bare\nbare\n\n" +
		"worktree /repos/foo\nHEAD abc\nbranch refs/heads/main\n"
	assert.Equal(t, "/repos/foo", firstNonBareWorktree(bareFirst))

	assert.Equal(t, "", firstNonBareWorktree(""), "no worktrees yields empty path")
}

// setupMainAndWorktree builds a main checkout on "main" with a baseline commit
// plus a linked worktree on branch "feature", and returns both paths. The
// worktree lives outside the main checkout (git refuses nested worktrees).
func setupMainAndWorktree(t *testing.T) (mainPath, wtPath, feature string) {
	t.Helper()
	mainPath = setupRebaseRepo(t) // on "main", one baseline commit
	wtPath = filepath.Join(t.TempDir(), "feature-wt")
	runGitCommand(t, mainPath, "worktree", "add", "-b", "feature", wtPath)
	return mainPath, wtPath, "feature"
}

func TestMainCheckoutPath_FromWorktree(t *testing.T) {
	mainPath, wtPath, _ := setupMainAndWorktree(t)

	// Resolving from the linked worktree must point back to the main checkout.
	got, err := MainCheckoutPath(wtPath)
	require.NoError(t, err)
	// macOS /tmp is a symlink to /private/tmp; compare resolved paths.
	wantResolved, _ := filepath.EvalSymlinks(mainPath)
	gotResolved, _ := filepath.EvalSymlinks(got)
	assert.Equal(t, wantResolved, gotResolved)
}

func TestAdvanceMainToBranch_FastForward(t *testing.T) {
	mainPath, wtPath, feature := setupMainAndWorktree(t)

	// The feature branch adds a commit on top of main (already caught up), so
	// main can fast-forward to it.
	writeAndCommit(t, wtPath, "feature.txt", "feature\n", "feature work")
	featureHead := headCommit(t, wtPath)

	require.NoError(t, AdvanceMainToBranch(mainPath, feature, "main", wtPath))

	// main now points at the feature tip...
	assert.Equal(t, featureHead, revParse(t, mainPath, "main"), "main fast-forwarded to feature")
	// ...and the worktree was resynced to main (still the same commit, clean).
	status, err := GetStatus(wtPath)
	require.NoError(t, err)
	assert.False(t, status.IsDirty, "worktree clean after resync")
	assert.Equal(t, featureHead, headCommit(t, wtPath))
}

func TestAdvanceMainToBranch_RefusesDivergedBranch(t *testing.T) {
	mainPath, wtPath, feature := setupMainAndWorktree(t)

	// feature adds a commit...
	writeAndCommit(t, wtPath, "feature.txt", "feature\n", "feature work")
	// ...and main diverges with its own commit, so an ff-only merge is impossible.
	writeAndCommit(t, mainPath, "main.txt", "main\n", "main work")
	mainHead := revParse(t, mainPath, "main")

	err := AdvanceMainToBranch(mainPath, feature, "main", wtPath)
	require.Error(t, err, "diverged branch must not fast-forward main")
	// main must be left untouched — never moved backward or force-merged.
	assert.Equal(t, mainHead, revParse(t, mainPath, "main"), "main unchanged after refused land")
}

func TestAdvanceMainToBranch_Validation(t *testing.T) {
	mainPath := setupRebaseRepo(t)
	assert.Error(t, AdvanceMainToBranch("", "feature", "main", ""), "empty main path refused")
	assert.Error(t, AdvanceMainToBranch(mainPath, "", "main", ""), "empty branch refused")
	assert.Error(t, AdvanceMainToBranch(mainPath, "HEAD", "main", ""), "detached HEAD refused")
	assert.Error(t, AdvanceMainToBranch(mainPath, "feature", "", ""), "empty default branch refused")
}

// headCommit returns the resolved HEAD commit of dir.
func headCommit(t *testing.T, dir string) string {
	t.Helper()
	return revParse(t, dir, "HEAD")
}

// revParse resolves ref to a commit hash in dir.
func revParse(t *testing.T, dir, ref string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", ref)
	cmd.Dir = dir
	out, err := cmd.Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}
