package worktreeregistry_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grovetools/core/pkg/paths"
	"github.com/grovetools/core/pkg/worktreeregistry"
	"github.com/grovetools/core/util/pathutil"
)

// writeLegacyEntry plants a registry file in the PRE-tombstone shape: no
// schema_version, no status, no final_shas. The whole point of the tombstone
// schema is that files written by older grove builds keep behaving exactly as
// they did, so the guard has to be a hand-written legacy file, not a
// round-tripped current one.
func writeLegacyEntry(t *testing.T, absPath string, body map[string]any) string {
	t.Helper()
	id := pathutil.WorktreeID(absPath)
	dir := filepath.Join(paths.StateDir(), "worktrees")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	data, err := json.Marshal(body)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, id+".json"), data, 0o644))
	return id
}

func TestLegacyEntryParsesAndIsActive(t *testing.T) {
	setStateDir(t)
	wt := t.TempDir()

	id := writeLegacyEntry(t, wt, map[string]any{
		"abs_path":      wt,
		"owner":         "/owner",
		"repos":         []string{"core"},
		"plan":          "legacy-plan",
		"labels":        map[string]string{"kind": "legacy"},
		"session_state": map[string]any{"active_job": "05-finish-provenance"},
	})

	entry, err := worktreeregistry.Load(id)
	require.NoError(t, err)
	assert.Equal(t, 0, entry.SchemaVersion, "legacy files carry no schema version")
	assert.Equal(t, "", entry.Status)
	assert.Equal(t, worktreeregistry.StatusActive, entry.EffectiveStatus(),
		"an absent status must read as active")
	assert.False(t, entry.IsFinished())

	// Every default query path must still see it.
	all, err := worktreeregistry.ListAll()
	require.NoError(t, err)
	assert.Len(t, all, 1)

	plan, ok := worktreeregistry.PlanForPath(wt)
	assert.True(t, ok)
	assert.Equal(t, "legacy-plan", plan)

	resolved, err := worktreeregistry.Resolve(wt, nil)
	require.NoError(t, err)
	require.NotNil(t, resolved)
	assert.Equal(t, "legacy-plan", resolved.Plan)

	byRef, err := worktreeregistry.FindByRef("legacy-plan")
	require.NoError(t, err)
	require.NotNil(t, byRef)
	assert.Equal(t, wt, byRef.AbsPath)
}

func TestTombstoneFlipsStatusStripsSessionStateAndRecordsSHAs(t *testing.T) {
	setStateDir(t)
	wt := t.TempDir()

	require.NoError(t, worktreeregistry.Save(&worktreeregistry.Entry{
		AbsPath:      wt,
		Owner:        "/owner",
		Repos:        []string{"core", "flow"},
		Plan:         "hosted-git",
		Labels:       map[string]string{"wave": "local-only"},
		SessionState: map[string]any{"active_job": "05", "secret": "should not fossilize"},
	}))

	id := pathutil.WorktreeID(wt)
	finals := []worktreeregistry.RepoFinalState{
		{Repo: "core", Branch: "hosted-git", SHA: "aaaa1111", Source: worktreeregistry.SHASourceReceipt},
		{Repo: "flow", Branch: "hosted-git", SHA: "bbbb2222", Source: worktreeregistry.SHASourceBranchHead},
	}
	entry, err := worktreeregistry.Tombstone(id, finals)
	require.NoError(t, err)

	assert.Equal(t, worktreeregistry.StatusFinished, entry.Status)
	assert.True(t, entry.IsFinished())
	assert.False(t, entry.FinishedAt.IsZero())
	assert.Equal(t, worktreeregistry.EntrySchemaVersion, entry.SchemaVersion)
	assert.Nil(t, entry.SessionState, "SessionState must never fossilize in a tombstone")
	assert.Equal(t, finals, entry.FinalSHAs)

	// The binding itself is what tombstoning exists to keep.
	assert.Equal(t, "hosted-git", entry.Plan)
	assert.Equal(t, []string{"core", "flow"}, entry.Repos)
	assert.Equal(t, map[string]string{"wave": "local-only"}, entry.Labels)

	// And it survives a reload — nothing here is in-memory only.
	reloaded, err := worktreeregistry.Load(id)
	require.NoError(t, err)
	assert.True(t, reloaded.IsFinished())
	assert.Nil(t, reloaded.SessionState)
	assert.Equal(t, finals, reloaded.FinalSHAs)
}

func TestTombstoneIsIdempotentAndKeepsFirstFinishedAt(t *testing.T) {
	setStateDir(t)
	wt := t.TempDir()
	require.NoError(t, worktreeregistry.Save(&worktreeregistry.Entry{AbsPath: wt, Plan: "p"}))
	id := pathutil.WorktreeID(wt)

	first, err := worktreeregistry.Tombstone(id, []worktreeregistry.RepoFinalState{{Repo: "core", SHA: "aaa"}})
	require.NoError(t, err)

	// A re-run with nothing new must not blank what the first run captured.
	second, err := worktreeregistry.Tombstone(id, nil)
	require.NoError(t, err)
	assert.Equal(t, first.FinishedAt, second.FinishedAt)
	assert.Equal(t, first.FinalSHAs, second.FinalSHAs)
}

func TestTombstoneMissingEntryReportsNotExist(t *testing.T) {
	setStateDir(t)
	_, err := worktreeregistry.Tombstone("no-such-entry-00000000", nil)
	require.Error(t, err)
	assert.True(t, os.IsNotExist(err), "callers distinguish 'nothing registered' via os.IsNotExist, got %v", err)
}

func TestFinishedEntriesAreExcludedFromDefaultQueries(t *testing.T) {
	setStateDir(t)
	active := t.TempDir()
	finished := t.TempDir()

	require.NoError(t, worktreeregistry.Save(&worktreeregistry.Entry{AbsPath: active, Plan: "active-plan"}))
	require.NoError(t, worktreeregistry.Save(&worktreeregistry.Entry{AbsPath: finished, Plan: "finished-plan"}))
	_, err := worktreeregistry.Tombstone(pathutil.WorktreeID(finished), nil)
	require.NoError(t, err)

	all, err := worktreeregistry.ListAll()
	require.NoError(t, err)
	require.Len(t, all, 1, "ListAll must return only active entries")
	assert.Equal(t, active, all[0].AbsPath)

	withHistory, err := worktreeregistry.ListAllIncludingFinished()
	require.NoError(t, err)
	assert.Len(t, withHistory, 2, "the opt-in must return history too")

	// Resolve
	resolved, err := worktreeregistry.Resolve(finished, nil)
	require.NoError(t, err)
	assert.Nil(t, resolved, "Resolve must treat a tombstone as an empty structural default")
	resolvedAny, err := worktreeregistry.ResolveIncludingFinished(finished, nil)
	require.NoError(t, err)
	require.NotNil(t, resolvedAny)
	assert.True(t, resolvedAny.IsFinished())

	// PlanForPath
	_, ok := worktreeregistry.PlanForPath(finished)
	assert.False(t, ok, "a finished worktree must not attribute its path to a plan")
	plan, ok := worktreeregistry.PlanForPath(active)
	assert.True(t, ok)
	assert.Equal(t, "active-plan", plan)

	// FindByRef, by plan name and by absolute path.
	_, err = worktreeregistry.FindByRef("finished-plan")
	assert.Error(t, err, "a finished plan is not a valid action target")
	_, err = worktreeregistry.FindByRef(finished)
	assert.Error(t, err)

	byName, err := worktreeregistry.FindByRefIncludingFinished("finished-plan")
	require.NoError(t, err)
	assert.Equal(t, finished, byName.AbsPath)
	byPath, err := worktreeregistry.FindByRefIncludingFinished(finished)
	require.NoError(t, err)
	assert.True(t, byPath.IsFinished())
}

// The registry in the wild holds all three shapes at once: entries written by
// this build, tombstones, and files written before either existed. Every
// default query must return exactly the active ones.
func TestMixedActiveFinishedAndLegacyRegistry(t *testing.T) {
	setStateDir(t)

	active := t.TempDir()
	finished := t.TempDir()
	legacy := t.TempDir()

	require.NoError(t, worktreeregistry.Save(&worktreeregistry.Entry{AbsPath: active, Plan: "active-plan"}))
	require.NoError(t, worktreeregistry.Save(&worktreeregistry.Entry{AbsPath: finished, Plan: "finished-plan"}))
	_, err := worktreeregistry.Tombstone(pathutil.WorktreeID(finished), nil)
	require.NoError(t, err)
	writeLegacyEntry(t, legacy, map[string]any{"abs_path": legacy, "plan": "legacy-plan"})

	all, err := worktreeregistry.ListAll()
	require.NoError(t, err)
	paths := map[string]bool{}
	for _, e := range all {
		paths[e.AbsPath] = true
	}
	assert.True(t, paths[active], "active entry missing from ListAll")
	assert.True(t, paths[legacy], "legacy (unversioned) entry must still be treated as active")
	assert.False(t, paths[finished], "tombstone must be excluded by default")
	assert.Len(t, all, 2)

	for _, ref := range []string{"active-plan", "legacy-plan"} {
		entry, err := worktreeregistry.FindByRef(ref)
		require.NoErrorf(t, err, "FindByRef(%q)", ref)
		assert.Equal(t, ref, entry.Plan)
	}
	_, err = worktreeregistry.FindByRef("finished-plan")
	assert.Error(t, err)

	// Reconcile over the mix: containers all still exist, so nothing is
	// pruned, and the tombstone stays a tombstone.
	require.NoError(t, worktreeregistry.Reconcile(""))
	after, err := worktreeregistry.ListAllIncludingFinished()
	require.NoError(t, err)
	assert.Len(t, after, 3, "reconcile must not drop any of the three shapes")
	tomb, err := worktreeregistry.Load(pathutil.WorktreeID(finished))
	require.NoError(t, err)
	assert.True(t, tomb.IsFinished())
}

func TestReconcileKeepsTombstonesAndStillPrunesActiveGhosts(t *testing.T) {
	setStateDir(t)

	// A finished worktree whose container is GONE — the normal end state
	// after finish retires it. Reconcile must not treat that as drift.
	gone := filepath.Join(t.TempDir(), "retired-worktree")
	require.NoError(t, os.MkdirAll(gone, 0o755))
	require.NoError(t, worktreeregistry.Save(&worktreeregistry.Entry{AbsPath: gone, Plan: "retired"}))
	_, err := worktreeregistry.Tombstone(pathutil.WorktreeID(gone), []worktreeregistry.RepoFinalState{{Repo: "core", SHA: "abc"}})
	require.NoError(t, err)
	require.NoError(t, os.RemoveAll(gone))

	// An ACTIVE entry whose container is gone is still drift, and is pruned.
	ghost := filepath.Join(t.TempDir(), "ghost-worktree")
	require.NoError(t, os.MkdirAll(ghost, 0o755))
	require.NoError(t, worktreeregistry.Save(&worktreeregistry.Entry{AbsPath: ghost, Plan: "ghost"}))
	require.NoError(t, os.RemoveAll(ghost))

	require.NoError(t, worktreeregistry.Reconcile(""))

	tomb, err := worktreeregistry.Load(pathutil.WorktreeID(gone))
	require.NoError(t, err, "Reconcile must not prune a tombstone whose worktree is gone by design")
	assert.True(t, tomb.IsFinished())
	assert.Equal(t, "retired", tomb.Plan)

	_, err = worktreeregistry.Load(pathutil.WorktreeID(ghost))
	assert.True(t, os.IsNotExist(err), "an active entry with no container on disk is still pruned")
}

func TestReconcileDoesNotReadoptATombstonedContainer(t *testing.T) {
	setStateDir(t)
	base := t.TempDir()
	container := filepath.Join(base, "owner-1234", "my-plan")
	require.NoError(t, os.MkdirAll(container, 0o755))
	// A creation marker, so the adopt step considers it a real worktree.
	require.NoError(t, os.WriteFile(filepath.Join(container, "grove.toml"), []byte(""), 0o644))

	require.NoError(t, worktreeregistry.Save(&worktreeregistry.Entry{AbsPath: container, Plan: "my-plan"}))
	_, err := worktreeregistry.Tombstone(pathutil.WorktreeID(container), []worktreeregistry.RepoFinalState{{Repo: "core", SHA: "abc"}})
	require.NoError(t, err)

	// The container survives (finish archived the plan but kept the dir);
	// adoption would overwrite the tombstone with a bare structural entry.
	require.NoError(t, worktreeregistry.Reconcile(base))

	entry, err := worktreeregistry.Load(pathutil.WorktreeID(container))
	require.NoError(t, err)
	assert.True(t, entry.IsFinished(), "adoption must not resurrect a tombstoned container as active")
	assert.Equal(t, "my-plan", entry.Plan)
	require.Len(t, entry.FinalSHAs, 1)
}

func TestDeleteUnlessFinishedKeepsTombstonesOnly(t *testing.T) {
	setStateDir(t)

	activePath := t.TempDir()
	finishedPath := t.TempDir()
	require.NoError(t, worktreeregistry.Save(&worktreeregistry.Entry{AbsPath: activePath}))
	require.NoError(t, worktreeregistry.Save(&worktreeregistry.Entry{AbsPath: finishedPath}))
	_, err := worktreeregistry.Tombstone(pathutil.WorktreeID(finishedPath), nil)
	require.NoError(t, err)

	kept, err := worktreeregistry.DeleteUnlessFinished(pathutil.WorktreeID(finishedPath))
	require.NoError(t, err)
	assert.True(t, kept)
	_, err = worktreeregistry.Load(pathutil.WorktreeID(finishedPath))
	assert.NoError(t, err, "the tombstone must survive the teardown that follows it")

	kept, err = worktreeregistry.DeleteUnlessFinished(pathutil.WorktreeID(activePath))
	require.NoError(t, err)
	assert.False(t, kept)
	_, err = worktreeregistry.Load(pathutil.WorktreeID(activePath))
	assert.True(t, os.IsNotExist(err), "an active entry is deleted exactly as before")

	// Absent entry stays idempotent.
	kept, err = worktreeregistry.DeleteUnlessFinished("nope-00000000")
	require.NoError(t, err)
	assert.False(t, kept)
}

func TestArchivePreservesTombstone(t *testing.T) {
	setStateDir(t)
	live := t.TempDir()
	archived := t.TempDir()

	require.NoError(t, worktreeregistry.Save(&worktreeregistry.Entry{AbsPath: live, Plan: "p", Repos: []string{"core"}}))
	_, err := worktreeregistry.Tombstone(pathutil.WorktreeID(live),
		[]worktreeregistry.RepoFinalState{{Repo: "core", SHA: "deadbeef", Source: worktreeregistry.SHASourceReceipt}})
	require.NoError(t, err)

	// Finish archives the container AFTER tombstoning it; the re-key must
	// carry the record across rather than reset it.
	require.NoError(t, worktreeregistry.Archive(live, archived))

	entry, err := worktreeregistry.Load(pathutil.WorktreeID(archived))
	require.NoError(t, err)
	assert.True(t, entry.IsFinished())
	assert.True(t, entry.IsArchived())
	require.Len(t, entry.FinalSHAs, 1)
	assert.Equal(t, "deadbeef", entry.FinalSHAs[0].SHA)
}
