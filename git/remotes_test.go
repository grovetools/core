package git

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bareRemote creates a bare repo to stand in for a hosted remote.
func bareRemote(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGitCommand(t, dir, "init", "--bare")
	return dir
}

// revListCount is the GROUND TRUTH the model is checked against: the raw
// `git rev-list --count <a>..<b>` git itself reports, run outside the model.
func revListCount(t *testing.T, dir, from, to string) int {
	t.Helper()
	cmd := exec.Command("git", "rev-list", "--count", from+".."+to)
	cmd.Dir = dir
	out, err := cmd.Output()
	require.NoError(t, err)
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	require.NoError(t, err)
	return n
}

// --- ListRemotes across 0, 1 and 2 remotes -----------------------------------

func TestListRemotes_NoRemotes(t *testing.T) {
	dir := setupRebaseRepo(t)

	remotes, err := ListRemotes(dir)
	require.NoError(t, err, "a repo with no remotes is a normal state, not an error")
	assert.Empty(t, remotes, "local-only repo has no remotes")
}

func TestListRemotes_OneRemote(t *testing.T) {
	dir := setupRebaseRepo(t)
	origin := bareRemote(t)
	runGitCommand(t, dir, "remote", "add", "origin", origin)

	remotes, err := ListRemotes(dir)
	require.NoError(t, err)
	require.Len(t, remotes, 1)
	assert.Equal(t, "origin", remotes[0].Name)
	assert.Equal(t, origin, remotes[0].URL, "the fetch URL is reported once, not once per fetch/push line")
}

func TestListRemotes_TwoRemotes(t *testing.T) {
	dir := setupRebaseRepo(t)
	origin := bareRemote(t)
	forge := bareRemote(t)
	// Added out of alphabetical order: the model sorts by name.
	runGitCommand(t, dir, "remote", "add", "origin", origin)
	runGitCommand(t, dir, "remote", "add", "forge", forge)

	remotes, err := ListRemotes(dir)
	require.NoError(t, err)
	require.Len(t, remotes, 2)
	assert.Equal(t, "forge", remotes[0].Name)
	assert.Equal(t, forge, remotes[0].URL)
	assert.Equal(t, "origin", remotes[1].Name)
	assert.Equal(t, origin, remotes[1].URL)
}

func TestParseRemotesV(t *testing.T) {
	out := "forge\tssh://git@forge.example.com/g/core.git (fetch)\n" +
		"forge\tssh://git@forge.example.com/g/core.git (push)\n" +
		"origin\tgit@github.com:u/core.git (fetch)\n" +
		"origin\tgit@github.com:u/other.git (push)\n"
	remotes := parseRemotesV(out)
	require.Len(t, remotes, 2, "a differing push URL is not a second remote")
	assert.Equal(t, Remote{Name: "forge", URL: "ssh://git@forge.example.com/g/core.git"}, remotes[0])
	assert.Equal(t, Remote{Name: "origin", URL: "git@github.com:u/core.git"}, remotes[1],
		"the FETCH url is what tracking refs come from, so it is what is reported")

	assert.Empty(t, parseRemotesV(""), "no remotes yields an empty, non-nil slice")
}

// --- RemoteBranchExists ------------------------------------------------------

func TestRemoteBranchExists(t *testing.T) {
	dir := setupRebaseRepo(t)
	origin := bareRemote(t)
	forge := bareRemote(t)
	runGitCommand(t, dir, "remote", "add", "origin", origin)
	runGitCommand(t, dir, "remote", "add", "forge", forge)

	// Published to origin only.
	runGitCommand(t, dir, "push", "origin", "main")
	runGitCommand(t, dir, "fetch", "origin")

	assert.True(t, RemoteBranchExists(dir, "origin", "main"))
	assert.False(t, RemoteBranchExists(dir, "forge", "main"),
		"a branch not published to the forge remote must read as absent, not as unknown-true")
	assert.False(t, RemoteBranchExists(dir, "origin", "nope"))
	assert.False(t, RemoteBranchExists(dir, "nosuchremote", "main"))

	// Detached-HEAD spellings are never remote-tracked.
	for _, b := range []string{"", "HEAD", "(detached)"} {
		assert.False(t, RemoteBranchExists(dir, "origin", b), "detached spelling %q", b)
	}

	// Flag-shaped names are refused rather than handed to git.
	assert.False(t, RemoteBranchExists(dir, "--upload-pack=touch /tmp/x", "main"))
	assert.False(t, RemoteBranchExists(dir, "origin", "../../etc/passwd"))
}

// TestHasRemoteBranch_OriginOnlyBehaviorPreserved pins the origin-only
// compatibility promise: the pre-multi-remote entry point still answers exactly
// as the default-remote spelling of the new model.
func TestHasRemoteBranch_OriginOnlyBehaviorPreserved(t *testing.T) {
	dir := setupRebaseRepo(t)
	origin := bareRemote(t)
	runGitCommand(t, dir, "remote", "add", "origin", origin)
	runGitCommand(t, dir, "push", "origin", "main")
	runGitCommand(t, dir, "fetch", "origin")

	assert.True(t, HasRemoteBranch(dir, "main"))
	assert.Equal(t, RemoteBranchExists(dir, DefaultRemoteName, "main"), HasRemoteBranch(dir, "main"))

	runGitCommand(t, dir, "checkout", "-b", "unpushed")
	assert.False(t, HasRemoteBranch(dir, "unpushed"))
	assert.Equal(t, RemoteBranchExists(dir, DefaultRemoteName, "unpushed"), HasRemoteBranch(dir, "unpushed"))
}

// --- divergence vs rev-list ground truth --------------------------------------

// TestGetRemoteBranchState_MatchesRevListGroundTruth builds a repo whose HEAD is
// both ahead of and behind two DIFFERENT remotes by different amounts, then
// checks every count against raw `git rev-list --count`.
func TestGetRemoteBranchState_MatchesRevListGroundTruth(t *testing.T) {
	dir := setupRebaseRepo(t)
	origin := bareRemote(t)
	forge := bareRemote(t)
	runGitCommand(t, dir, "remote", "add", "origin", origin)
	runGitCommand(t, dir, "remote", "add", "forge", forge)

	// Publish the baseline to origin, then one more commit to the forge, so the
	// two remotes sit at different points in the same history.
	runGitCommand(t, dir, "push", "origin", "main")
	writeAndCommit(t, dir, "one.txt", "1\n", "one")
	runGitCommand(t, dir, "push", "forge", "main")
	// Two more local commits: HEAD is now 3 ahead of origin/main, 2 ahead of
	// forge/main, and behind neither.
	writeAndCommit(t, dir, "two.txt", "2\n", "two")
	writeAndCommit(t, dir, "three.txt", "3\n", "three")
	runGitCommand(t, dir, "fetch", "origin")
	runGitCommand(t, dir, "fetch", "forge")

	for _, remote := range []string{"origin", "forge"} {
		ref := RemoteTrackingRef(remote, "main")
		st := GetRemoteBranchState(dir, remote, "main", "HEAD")
		require.True(t, st.Exists, "%s/main should be tracked", remote)
		assert.Equal(t, revListCount(t, dir, ref, "HEAD"), st.Ahead, "%s ahead", remote)
		assert.Equal(t, revListCount(t, dir, "HEAD", ref), st.Behind, "%s behind", remote)
	}

	assert.Equal(t, 3, GetRemoteBranchState(dir, "origin", "main", "HEAD").Ahead)
	assert.Equal(t, 2, GetRemoteBranchState(dir, "forge", "main", "HEAD").Ahead)

	// Now move origin ahead of the local branch from a second clone, so behind
	// is non-zero too — still without ever fetching implicitly.
	other := t.TempDir()
	runGitCommand(t, other, "clone", origin, ".")
	writeAndCommit(t, other, "remote-side.txt", "r\n", "remote side")
	runGitCommand(t, other, "push", "origin", "HEAD:main")

	before := GetRemoteBranchState(dir, "origin", "main", "HEAD")
	assert.Equal(t, 0, before.Behind,
		"the model must NOT fetch: the stale tracking ref still knows nothing of the new commit")

	runGitCommand(t, dir, "fetch", "origin")
	after := GetRemoteBranchState(dir, "origin", "main", "HEAD")
	ref := RemoteTrackingRef("origin", "main")
	assert.Equal(t, revListCount(t, dir, ref, "HEAD"), after.Ahead)
	assert.Equal(t, revListCount(t, dir, "HEAD", ref), after.Behind)
	assert.Positive(t, after.Behind, "after an explicit fetch the divergence is visible")
}

func TestGetRemoteBranchState_MissingTrackingRef(t *testing.T) {
	dir := setupRebaseRepo(t)
	runGitCommand(t, dir, "remote", "add", "forge", bareRemote(t))

	st := GetRemoteBranchState(dir, "forge", "main", "HEAD")
	assert.False(t, st.Exists, "never published to the forge remote")
	assert.Equal(t, 0, st.Ahead)
	assert.Equal(t, 0, st.Behind)
	assert.True(t, st.Freshness.RefUpdatedAt.IsZero(), "no tracking ref ever moved")
	assert.False(t, st.Freshness.Known(),
		"nothing has been fetched in this checkout either, so the absence cannot be dated at all")

	// Once SOMETHING has been fetched here, the absence becomes a dated
	// observation rather than an undatable one — which is the whole difference
	// between "not there" and "not there as of a fetch three days ago".
	runGitCommand(t, dir, "fetch", "forge")
	dated := GetRemoteBranchState(dir, "forge", "main", "HEAD")
	assert.False(t, dated.Exists, "still not published")
	assert.False(t, dated.Freshness.LastFetchAt.IsZero(),
		"an absent tracking ref must still carry the last-fetch bound")
}

func TestGetRemoteBranchState_LocalRefDefaultsToHEAD(t *testing.T) {
	dir := setupRebaseRepo(t)
	runGitCommand(t, dir, "remote", "add", "origin", bareRemote(t))
	runGitCommand(t, dir, "push", "origin", "main")
	runGitCommand(t, dir, "fetch", "origin")
	writeAndCommit(t, dir, "extra.txt", "e\n", "extra")

	assert.Equal(t,
		GetRemoteBranchState(dir, "origin", "main", "HEAD"),
		GetRemoteBranchState(dir, "origin", "main", ""),
		`an empty local ref means HEAD`)
}

// --- RemoteBranchDistance (the origin-compat probe) ---------------------------

func TestRemoteBranchDistance(t *testing.T) {
	dir := setupRebaseRepo(t)
	origin := bareRemote(t)
	runGitCommand(t, dir, "remote", "add", "origin", origin)
	runGitCommand(t, dir, "push", "origin", "main")
	runGitCommand(t, dir, "fetch", "origin")

	exists, behind := RemoteBranchDistance(dir, DefaultRemoteName, "main")
	assert.True(t, exists)
	assert.Equal(t, 0, behind)

	// Advance origin from a second clone and fetch: HEAD now trails it.
	other := t.TempDir()
	runGitCommand(t, other, "clone", origin, ".")
	writeAndCommit(t, other, "remote-side.txt", "r\n", "remote side")
	runGitCommand(t, other, "push", "origin", "HEAD:main")
	runGitCommand(t, dir, "fetch", "origin")

	exists, behind = RemoteBranchDistance(dir, DefaultRemoteName, "main")
	assert.True(t, exists)
	assert.Equal(t, revListCount(t, dir, "HEAD", RemoteTrackingRef("origin", "main")), behind)
	assert.Equal(t, 1, behind)

	exists, behind = RemoteBranchDistance(dir, DefaultRemoteName, "never-pushed")
	assert.False(t, exists)
	assert.Equal(t, 0, behind)
}

// --- ListRemoteBranchStates ---------------------------------------------------

func TestListRemoteBranchStates(t *testing.T) {
	dir := setupRebaseRepo(t)

	states, err := ListRemoteBranchStates(dir, "main", "HEAD")
	require.NoError(t, err)
	assert.Empty(t, states, "a repo with no remotes renders cleanly as no rows")

	runGitCommand(t, dir, "remote", "add", "origin", bareRemote(t))
	runGitCommand(t, dir, "remote", "add", "forge", bareRemote(t))
	runGitCommand(t, dir, "push", "origin", "main")
	runGitCommand(t, dir, "fetch", "origin")

	states, err = ListRemoteBranchStates(dir, "main", "HEAD")
	require.NoError(t, err)
	require.Len(t, states, 2)
	assert.Equal(t, "forge", states[0].Remote)
	assert.False(t, states[0].Exists, "not published to the forge remote")
	assert.Equal(t, "origin", states[1].Remote)
	assert.True(t, states[1].Exists)
}

// --- freshness ----------------------------------------------------------------

func TestTrackingFreshness_ReportedNotAssumed(t *testing.T) {
	dir := setupRebaseRepo(t)
	runGitCommand(t, dir, "remote", "add", "origin", bareRemote(t))
	runGitCommand(t, dir, "push", "origin", "main")

	// Before any fetch from this checkout there is no FETCH_HEAD at all.
	assert.True(t, lastFetchAt(dir).IsZero(), "no fetch has run in this checkout yet")

	start := time.Now().Add(-2 * time.Minute)
	runGitCommand(t, dir, "fetch", "origin")

	st := GetRemoteBranchState(dir, "origin", "main", "HEAD")
	require.True(t, st.Exists)
	require.True(t, st.Freshness.Known(), "a tracked ref must carry at least one dating signal")
	assert.False(t, st.Freshness.LastFetchAt.IsZero(), "FETCH_HEAD exists after an explicit fetch")
	assert.True(t, st.Freshness.LastFetchAt.After(start), "last-fetch time is this run's fetch")
	if !st.Freshness.RefUpdatedAt.IsZero() {
		assert.True(t, st.Freshness.RefUpdatedAt.After(start),
			"the tracking ref moved during this test")
	}
}

func TestParseReflogUnixStamp(t *testing.T) {
	line := "e85580e refs/remotes/origin/main@{1785679794}: fetch origin: fast-forward\n"
	assert.Equal(t, time.Unix(1785679794, 0), parseReflogUnixStamp(line))

	for _, bad := range []string{"", "no selector here\n", "abc refs/remotes/origin/main@{nope}: x\n", "abc @{123"} {
		assert.True(t, parseReflogUnixStamp(bad).IsZero(), "unparseable %q must read as unknown, not as now", bad)
	}
}

// --- FetchRemote --------------------------------------------------------------

// TestFetchRemote_ExplicitOnly fetches from a LOCAL bare repo (no network), and
// pins the contract that a fetch is what makes stale counts fresh — nothing in
// the read path does it implicitly.
func TestFetchRemote_ExplicitOnly(t *testing.T) {
	dir := setupRebaseRepo(t)
	origin := bareRemote(t)
	runGitCommand(t, dir, "remote", "add", "origin", origin)
	runGitCommand(t, dir, "push", "origin", "main")
	runGitCommand(t, dir, "fetch", "origin")

	other := t.TempDir()
	runGitCommand(t, other, "clone", origin, ".")
	writeAndCommit(t, other, "remote-side.txt", "r\n", "remote side")
	runGitCommand(t, other, "push", "origin", "HEAD:main")

	// Every read path leaves the stale tracking ref alone.
	require.Equal(t, 0, GetRemoteBranchState(dir, "origin", "main", "HEAD").Behind)

	require.NoError(t, FetchRemote(context.Background(), dir, "origin"))
	assert.Equal(t, 1, GetRemoteBranchState(dir, "origin", "main", "HEAD").Behind,
		"only the explicit fetch moves the tracking ref")
}

func TestFetchRemote_RefusesBadNames(t *testing.T) {
	dir := setupRebaseRepo(t)
	assert.Error(t, FetchRemote(context.Background(), dir, ""), "empty remote refused")
	assert.Error(t, FetchRemote(context.Background(), dir, "   "), "blank remote refused")
	assert.Error(t, FetchRemote(context.Background(), dir, "--upload-pack=x"), "flag-shaped remote refused")
}

// TestFetchRemoteBranch_FetchesOnlyThatBranch pins the narrowing that makes the
// PRs page's fetch action cheap: asking for one branch lands THAT branch's
// tracking ref and leaves the repo's other branches untouched.
func TestFetchRemoteBranch_FetchesOnlyThatBranch(t *testing.T) {
	dir := setupRebaseRepo(t)
	origin := bareRemote(t)
	runGitCommand(t, dir, "remote", "add", "origin", origin)
	runGitCommand(t, dir, "push", "origin", "main")
	runGitCommand(t, dir, "fetch", "origin")

	// A second checkout publishes two new branches; only one is asked for.
	other := t.TempDir()
	runGitCommand(t, other, "clone", origin, ".")
	runGitCommand(t, other, "checkout", "-q", "-b", "feature/wanted")
	writeAndCommit(t, other, "wanted.txt", "w\n", "wanted")
	runGitCommand(t, other, "push", "origin", "feature/wanted")
	runGitCommand(t, other, "checkout", "-q", "-b", "feature/unwanted")
	writeAndCommit(t, other, "unwanted.txt", "u\n", "unwanted")
	runGitCommand(t, other, "push", "origin", "feature/unwanted")

	require.False(t, RemoteBranchExists(dir, "origin", "feature/wanted"),
		"nothing may have fetched on its own")

	require.NoError(t, FetchRemoteBranch(context.Background(), dir, "origin", "feature/wanted"))
	assert.True(t, RemoteBranchExists(dir, "origin", "feature/wanted"),
		"the requested branch's tracking ref must land where RemoteTrackingRef says to look")
	assert.False(t, RemoteBranchExists(dir, "origin", "feature/unwanted"),
		"a single-ref fetch must not drag in every other branch")
}

// TestFetchRemoteBranch_RefusesBadNames proves neither argument can smuggle a
// flag or a traversal into the argv.
func TestFetchRemoteBranch_RefusesBadNames(t *testing.T) {
	dir := setupRebaseRepo(t)
	ctx := context.Background()
	assert.Error(t, FetchRemoteBranch(ctx, dir, "", "main"), "empty remote refused")
	assert.Error(t, FetchRemoteBranch(ctx, dir, "origin", ""), "empty branch refused")
	assert.Error(t, FetchRemoteBranch(ctx, dir, "origin", "   "), "blank branch refused")
	assert.Error(t, FetchRemoteBranch(ctx, dir, "--upload-pack=x", "main"), "flag-shaped remote refused")
	assert.Error(t, FetchRemoteBranch(ctx, dir, "origin", "--upload-pack=x"), "flag-shaped branch refused")
	assert.Error(t, FetchRemoteBranch(ctx, dir, "origin", "a/../../b"), "traversal branch refused")
}
