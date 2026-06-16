package worktreeregistry_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grovetools/core/pkg/worktreeregistry"
	"github.com/grovetools/core/util/pathutil"
)

// setStateDir overrides GROVE_HOME so StateDir() resolves to a temp directory
// for the duration of the test.
func setStateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("GROVE_HOME", dir)
	return dir
}

func TestWorktreeID_MatchesDirIdentifierRecipe(t *testing.T) {
	tmp := t.TempDir()
	id := pathutil.WorktreeID(tmp)
	assert.NotEmpty(t, id)
	// ID must be "<basename>-<8hexchars>".
	assert.Regexp(t, `^[a-z0-9_-]+-[0-9a-f]{8}$`, id, "ID should be <sanitized basename>-<8 hex chars>")
}

func TestWorktreeID_StableAcrossCalls(t *testing.T) {
	tmp := t.TempDir()
	id1 := pathutil.WorktreeID(tmp)
	id2 := pathutil.WorktreeID(tmp)
	assert.Equal(t, id1, id2)
}

func TestSaveAndLoad_RoundTrip(t *testing.T) {
	setStateDir(t)
	dir := t.TempDir()

	entry := &worktreeregistry.Entry{
		AbsPath:   dir,
		Owner:     "/some/owner",
		Repos:     []string{"core", "flow"},
		Plan:      "my-plan",
		CreatedAt: time.Now().UTC().Truncate(time.Second),
		SessionState: map[string]interface{}{
			"key": "value",
		},
	}

	require.NoError(t, worktreeregistry.Save(entry))

	id := pathutil.WorktreeID(dir)
	loaded, err := worktreeregistry.Load(id)
	require.NoError(t, err)
	require.NotNil(t, loaded)

	assert.Equal(t, entry.AbsPath, loaded.AbsPath)
	assert.Equal(t, entry.Owner, loaded.Owner)
	assert.Equal(t, entry.Repos, loaded.Repos)
	assert.Equal(t, entry.Plan, loaded.Plan)
	assert.Equal(t, entry.SessionState["key"], loaded.SessionState["key"])
}

func TestAtomicSave_NoConcurrentPartialFiles(t *testing.T) {
	groveHome := setStateDir(t)

	var wg sync.WaitGroup
	dirs := make([]string, 10)
	for i := range dirs {
		dirs[i] = t.TempDir()
	}

	for _, dir := range dirs {
		dir := dir
		wg.Add(1)
		go func() {
			defer wg.Done()
			entry := &worktreeregistry.Entry{
				AbsPath: dir,
				Owner:   "/owner",
			}
			_ = worktreeregistry.Save(entry)
		}()
	}
	wg.Wait()

	// Verify no .tmp files leaked. StateDir() = GROVE_HOME/state/grove,
	// so registryDir = GROVE_HOME/state/grove/worktrees.
	registryDir := filepath.Join(groveHome, "state", "grove", "worktrees")
	entries, err := os.ReadDir(registryDir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), ".tmp", "no tmp files should remain after concurrent saves")
	}
}

func TestDelete_Idempotent(t *testing.T) {
	setStateDir(t)
	dir := t.TempDir()

	entry := &worktreeregistry.Entry{AbsPath: dir}
	require.NoError(t, worktreeregistry.Save(entry))

	id := pathutil.WorktreeID(dir)
	require.NoError(t, worktreeregistry.Delete(id))
	require.NoError(t, worktreeregistry.Delete(id), "second delete should be a no-op")
}

func TestListAll(t *testing.T) {
	setStateDir(t)
	dirs := make([]string, 3)
	for i := range dirs {
		dirs[i] = t.TempDir()
		require.NoError(t, worktreeregistry.Save(&worktreeregistry.Entry{AbsPath: dirs[i]}))
	}

	all, err := worktreeregistry.ListAll()
	require.NoError(t, err)
	assert.Len(t, all, 3)
}

func TestReconcile_PrunesStaleEntry(t *testing.T) {
	setStateDir(t)
	dir := t.TempDir()

	entry := &worktreeregistry.Entry{AbsPath: dir}
	require.NoError(t, worktreeregistry.Save(entry))

	// Remove the directory — simulates a manually deleted worktree.
	require.NoError(t, os.RemoveAll(dir))

	require.NoError(t, worktreeregistry.Reconcile(""))

	id := pathutil.WorktreeID(dir)
	_, err := worktreeregistry.Load(id)
	assert.True(t, os.IsNotExist(err), "stale entry should be pruned after Reconcile")
}

func TestReconcile_AdoptsUnregisteredLiveDir(t *testing.T) {
	setStateDir(t)
	xdgBase := t.TempDir()

	// Simulate an XDG worktree: <xdgBase>/<identifier>/<name>
	identifierDir := filepath.Join(xdgBase, "grovetools-abc12345")
	require.NoError(t, os.MkdirAll(identifierDir, 0o755))
	wtDir := filepath.Join(identifierDir, "my-branch")
	require.NoError(t, os.MkdirAll(wtDir, 0o755))

	require.NoError(t, worktreeregistry.Reconcile(xdgBase))

	id := pathutil.WorktreeID(wtDir)
	loaded, err := worktreeregistry.Load(id)
	require.NoError(t, err)
	assert.Equal(t, wtDir, loaded.AbsPath)
}

func TestResolve_ReturnsNilForMissingEntry(t *testing.T) {
	setStateDir(t)
	dir := t.TempDir()

	resolved, err := worktreeregistry.Resolve(dir, nil)
	assert.NoError(t, err)
	assert.Nil(t, resolved)
}

func TestResolve_HealsStaleAnchorOverride(t *testing.T) {
	setStateDir(t)
	dir := t.TempDir()

	entry := &worktreeregistry.Entry{
		AbsPath:        dir,
		Owner:          "/owner",
		Repos:          []string{"core", "flow"},
		AnchorOverride: "daemon", // no longer in Repos
	}
	require.NoError(t, worktreeregistry.Save(entry))

	resolved, err := worktreeregistry.Resolve(dir, []string{"core", "flow"})
	require.NoError(t, err)
	require.NotNil(t, resolved)
	assert.Empty(t, resolved.AnchorOverride, "stale AnchorOverride should be cleared")
}
