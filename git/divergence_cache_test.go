package git

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetDivergenceCache clears the package-level divergence cache so tests
// observe cold-cache behavior deterministically.
func resetDivergenceCache() {
	divergenceCacheMu.Lock()
	defer divergenceCacheMu.Unlock()
	divergenceCache = make(map[string]divergenceEntry)
}

// commitFile writes content to name and commits it.
func commitFile(t *testing.T, dir, name, content, msg string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
	runGitCommand(t, dir, "add", name)
	runGitCommand(t, dir, "commit", "-m", msg)
}

// setupMainRepo creates a repo with an initial commit on a branch named
// "main", regardless of the host's init.defaultBranch.
func setupMainRepo(t *testing.T, dir string) {
	t.Helper()
	setupGitRepo(t, dir)
	commitFile(t, dir, "base.txt", "base\n", "initial commit")
	runGitCommand(t, dir, "branch", "-M", "main")
}

func TestResolveDivergenceSHAsNormalRepo(t *testing.T) {
	dir := t.TempDir()
	setupMainRepo(t, dir)

	gitDir, commonDir, err := resolveGitDirs(dir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, ".git"), gitDir)
	assert.Equal(t, gitDir, commonDir)

	headSHA, err := resolveHeadSHA(gitDir, commonDir)
	require.NoError(t, err)
	assert.Equal(t, revParse(t, dir, "HEAD"), headSHA)

	shas, ok := resolveDivergenceSHAs(dir, localMainRefCandidates)
	require.True(t, ok)
	assert.Equal(t, "refs/heads/main", shas.baseRef)
	assert.Equal(t, revParse(t, dir, "refs/heads/main"), shas.baseSHA)
	assert.Equal(t, revParse(t, dir, "HEAD"), shas.headSHA)

	// Packed refs: after pack-refs the loose file is gone but resolution must
	// still succeed via packed-refs.
	runGitCommand(t, dir, "pack-refs", "--all")
	_, err = os.Stat(filepath.Join(gitDir, "refs", "heads", "main"))
	require.True(t, os.IsNotExist(err), "expected loose main ref to be packed away")

	shas, ok = resolveDivergenceSHAs(dir, localMainRefCandidates)
	require.True(t, ok)
	assert.Equal(t, revParse(t, dir, "refs/heads/main"), shas.baseSHA)
	assert.Equal(t, revParse(t, dir, "HEAD"), shas.headSHA)

	// Detached HEAD: HEAD holds a raw SHA.
	runGitCommand(t, dir, "checkout", "--detach")
	headSHA, err = resolveHeadSHA(gitDir, commonDir)
	require.NoError(t, err)
	assert.Equal(t, revParse(t, dir, "HEAD"), headSHA)
}

func TestResolveDivergenceSHAsLinkedWorktree(t *testing.T) {
	root := t.TempDir()
	mainDir := filepath.Join(root, "main-repo")
	wtDir := filepath.Join(root, "wt")
	require.NoError(t, os.MkdirAll(mainDir, 0o755))
	setupMainRepo(t, mainDir)
	runGitCommand(t, mainDir, "worktree", "add", wtDir, "-b", "feature")
	commitFile(t, wtDir, "feature.txt", "feature\n", "feature commit")

	gitDir, commonDir, err := resolveGitDirs(wtDir)
	require.NoError(t, err)
	assert.NotEqual(t, gitDir, commonDir, "linked worktree must have a distinct common dir")

	shas, ok := resolveDivergenceSHAs(wtDir, localMainRefCandidates)
	require.True(t, ok)
	assert.Equal(t, revParse(t, wtDir, "HEAD"), shas.headSHA)
	assert.Equal(t, "refs/heads/main", shas.baseRef)
	assert.Equal(t, revParse(t, wtDir, "refs/heads/main"), shas.baseSHA)

	// The worktree HEAD must be the feature tip, not the main checkout's HEAD.
	assert.NotEqual(t, revParse(t, mainDir, "HEAD"), shas.headSHA)
}

func TestResolveDivergenceSHAsAnomalies(t *testing.T) {
	t.Run("non-repo directory", func(t *testing.T) {
		_, ok := resolveDivergenceSHAs(t.TempDir(), localMainRefCandidates)
		assert.False(t, ok)
	})

	t.Run("repo without main or master", func(t *testing.T) {
		dir := t.TempDir()
		setupGitRepo(t, dir)
		commitFile(t, dir, "a.txt", "a\n", "commit")
		runGitCommand(t, dir, "branch", "-M", "topic")
		_, ok := resolveDivergenceSHAs(dir, localMainRefCandidates)
		assert.False(t, ok)
	})

	t.Run("unborn HEAD", func(t *testing.T) {
		dir := t.TempDir()
		setupGitRepo(t, dir)
		_, ok := resolveDivergenceSHAs(dir, localMainRefCandidates)
		assert.False(t, ok)
	})
}

func TestGetCommitsDivergenceFromMainCache(t *testing.T) {
	resetDivergenceCache()
	dir := t.TempDir()
	setupMainRepo(t, dir)
	runGitCommand(t, dir, "checkout", "-b", "feature")
	commitFile(t, dir, "feature.txt", "feature\n", "feature commit")
	runGitCommand(t, dir, "checkout", "main")
	commitFile(t, dir, "main.txt", "main\n", "main commit")
	runGitCommand(t, dir, "checkout", "feature")

	cleanPath := filepath.Clean(dir)

	ahead, behind := GetCommitsDivergenceFromMain(dir, "feature")
	assert.Equal(t, 1, ahead)
	assert.Equal(t, 1, behind)

	// Prove the second call is served from the cache: tamper with the stored
	// counts and confirm the tampered values come back verbatim (a forked
	// recomputation would return 1/1 again).
	shas, ok := resolveDivergenceSHAs(cleanPath, localMainRefCandidates)
	require.True(t, ok)
	storeDivergence(cleanPath, shas, 42, 24)
	ahead, behind = GetCommitsDivergenceFromMain(dir, "feature")
	assert.Equal(t, 42, ahead)
	assert.Equal(t, 24, behind)

	// A new commit moves HEAD, invalidating the entry: values must refresh.
	commitFile(t, dir, "feature2.txt", "more\n", "second feature commit")
	ahead, behind = GetCommitsDivergenceFromMain(dir, "feature")
	assert.Equal(t, 2, ahead)
	assert.Equal(t, 1, behind)

	// And the refreshed values are stable on a repeat call.
	ahead, behind = GetCommitsDivergenceFromMain(dir, "feature")
	assert.Equal(t, 2, ahead)
	assert.Equal(t, 1, behind)
}

func TestStoreDivergenceIfCurrentRejectsMovedRefs(t *testing.T) {
	resetDivergenceCache()
	dir := t.TempDir()
	setupMainRepo(t, dir)
	runGitCommand(t, dir, "checkout", "-b", "feature")

	before, ok := resolveDivergenceSHAs(dir, localMainRefCandidates)
	require.True(t, ok)
	commitFile(t, dir, "feature.txt", "feature\n", "move head")

	assert.False(t, storeDivergenceIfCurrent(dir, localMainRefCandidates, before, 9, 7))
	_, _, cached := lookupDivergence(dir, before)
	assert.False(t, cached, "a result computed across a ref move must not be cached")
}

func TestMainShortcutWithContradictoryBranchDoesNotPoisonCache(t *testing.T) {
	resetDivergenceCache()
	dir := t.TempDir()
	setupMainRepo(t, dir)
	runGitCommand(t, dir, "checkout", "-b", "feature")
	commitFile(t, dir, "feature.txt", "feature\n", "feature commit")

	// An incorrect exported-API argument still retains the historical shortcut
	// result, but it must not poison the SHA-keyed cache.
	ahead, behind := GetCommitsDivergenceFromMain(dir, "main")
	assert.Equal(t, 0, ahead)
	assert.Equal(t, 0, behind)

	ahead, behind = GetCommitsDivergenceFromMain(dir, "feature")
	assert.Equal(t, 1, ahead)
	assert.Equal(t, 0, behind)
}

func TestDivergenceCacheSizeBackstop(t *testing.T) {
	resetDivergenceCache()
	for i := 0; i < maxDivergenceCacheEntries; i++ {
		storeDivergence(filepath.Join("repo", strconv.Itoa(i)), divergenceSHAs{headSHA: "a"}, i, 0)
	}
	storeDivergence("final", divergenceSHAs{headSHA: "b"}, 1, 2)

	divergenceCacheMu.Lock()
	defer divergenceCacheMu.Unlock()
	assert.Len(t, divergenceCache, 1)
	assert.Contains(t, divergenceCache, "final")
}

func TestGetCommitsDivergenceFromRemoteMainCache(t *testing.T) {
	resetDivergenceCache()
	root := t.TempDir()
	originDir := filepath.Join(root, "origin")
	cloneDir := filepath.Join(root, "clone")
	require.NoError(t, os.MkdirAll(originDir, 0o755))
	setupMainRepo(t, originDir)
	runGitCommand(t, root, "clone", originDir, cloneDir)
	runGitCommand(t, cloneDir, "config", "user.email", "test@example.com")
	runGitCommand(t, cloneDir, "config", "user.name", "Test User")

	commitFile(t, cloneDir, "local.txt", "local\n", "local commit")

	cleanPath := filepath.Clean(cloneDir)

	ahead, behind := GetCommitsDivergenceFromRemoteMain(cloneDir, "main")
	assert.Equal(t, 1, ahead)
	assert.Equal(t, 0, behind)

	// Cache-hit proof via tampering, as in the local-main test.
	shas, ok := resolveDivergenceSHAs(cleanPath, remoteMainRefCandidates)
	require.True(t, ok)
	assert.Equal(t, "refs/remotes/origin/main", shas.baseRef)
	storeDivergence(cleanPath, shas, 7, 3)
	ahead, behind = GetCommitsDivergenceFromRemoteMain(cloneDir, "main")
	assert.Equal(t, 7, ahead)
	assert.Equal(t, 3, behind)

	// HEAD moves; the entry is stale and must be recomputed.
	commitFile(t, cloneDir, "local2.txt", "more\n", "second local commit")
	ahead, behind = GetCommitsDivergenceFromRemoteMain(cloneDir, "main")
	assert.Equal(t, 2, ahead)
	assert.Equal(t, 0, behind)
}

// TestDivergenceCacheStatsCountHitsAndMisses pins the counters the daemon
// publishes as git.divergence_cache.* on /api/system/stats. A collapsing hit
// rate is the earliest signal that git forks are about to dominate CPU again.
func TestDivergenceCacheStatsCountHitsAndMisses(t *testing.T) {
	h0, m0, _ := DivergenceCacheStats()

	shas := divergenceSHAs{headSHA: "aaa", baseRef: "refs/heads/main", baseSHA: "bbb"}
	path := t.TempDir()

	if _, _, ok := lookupDivergence(path, shas); ok {
		t.Fatal("unexpected hit on a cold cache")
	}
	storeDivergence(path, shas, 3, 4)
	if _, _, ok := lookupDivergence(path, shas); !ok {
		t.Fatal("expected a hit after storing")
	}
	// A moved endpoint invalidates: counted as a miss, not a hit.
	if _, _, ok := lookupDivergence(path, divergenceSHAs{headSHA: "ccc", baseRef: "refs/heads/main", baseSHA: "bbb"}); ok {
		t.Fatal("stale SHAs must not hit")
	}

	h1, m1, _ := DivergenceCacheStats()
	if h1-h0 != 1 {
		t.Errorf("hits delta = %d, want 1", h1-h0)
	}
	if m1-m0 != 2 {
		t.Errorf("misses delta = %d, want 2", m1-m0)
	}
}
