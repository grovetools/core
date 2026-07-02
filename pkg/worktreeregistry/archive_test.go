package worktreeregistry_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grovetools/core/pkg/worktreeregistry"
	"github.com/grovetools/core/util/pathutil"
)

func TestArchive_RoundTrip(t *testing.T) {
	setStateDir(t)
	oldDir := t.TempDir()
	newDir := t.TempDir()

	require.NoError(t, worktreeregistry.Save(&worktreeregistry.Entry{
		AbsPath: oldDir,
		Owner:   "/some/owner",
		Repos:   []string{"core", "flow"},
		Plan:    "my-plan",
	}))

	require.NoError(t, worktreeregistry.Archive(oldDir, newDir))

	// Old ID file is gone.
	_, err := worktreeregistry.Load(pathutil.WorktreeID(oldDir))
	assert.True(t, os.IsNotExist(err), "old registry entry should be deleted")

	// New entry is keyed by the archive path and carries the archive metadata
	// plus the original entry's history.
	archived, err := worktreeregistry.Load(pathutil.WorktreeID(newDir))
	require.NoError(t, err)
	assert.Equal(t, newDir, archived.AbsPath)
	assert.Equal(t, oldDir, archived.OriginalPath)
	assert.False(t, archived.ArchivedAt.IsZero(), "ArchivedAt should be set")
	assert.True(t, archived.IsArchived())
	assert.Equal(t, "my-plan", archived.Plan)
	assert.Equal(t, []string{"core", "flow"}, archived.Repos)
	assert.Equal(t, "/some/owner", archived.Owner)
}

func TestArchive_SynthesizesMissingEntry(t *testing.T) {
	setStateDir(t)
	oldDir := t.TempDir()
	newDir := t.TempDir()

	// No registry entry exists for oldDir.
	require.NoError(t, worktreeregistry.Archive(oldDir, newDir))

	archived, err := worktreeregistry.Load(pathutil.WorktreeID(newDir))
	require.NoError(t, err)
	assert.Equal(t, newDir, archived.AbsPath)
	assert.Equal(t, oldDir, archived.OriginalPath)
	assert.True(t, archived.IsArchived())
}

func TestArchive_RejectsEmptyPaths(t *testing.T) {
	setStateDir(t)
	assert.Error(t, worktreeregistry.Archive("", t.TempDir()))
	assert.Error(t, worktreeregistry.Archive(t.TempDir(), ""))
}

func TestReconcile_LeavesArchivedEntryAlone(t *testing.T) {
	setStateDir(t)
	xdgBase := t.TempDir()

	// Archive a worktree into a real on-disk archive location (a sibling of
	// xdgBase, mirroring paths.WorktreeArchiveDir() vs paths.WorktreesDir()).
	oldDir := t.TempDir()
	archiveDir := t.TempDir()
	require.NoError(t, worktreeregistry.Save(&worktreeregistry.Entry{
		AbsPath:        oldDir,
		Plan:           "archived-plan",
		Repos:          []string{"core"},
		AnchorOverride: "gone-repo", // stale — must NOT be healed for archived entries
	}))
	require.NoError(t, worktreeregistry.Archive(oldDir, archiveDir))
	// The live location is gone after the (simulated) move.
	require.NoError(t, os.RemoveAll(oldDir))

	require.NoError(t, worktreeregistry.Reconcile(xdgBase))

	// The archived entry survives step-1 stat-prune (AbsPath points at the
	// archive location, which exists) and anchor-heal left it untouched.
	archived, err := worktreeregistry.Load(pathutil.WorktreeID(archiveDir))
	require.NoError(t, err, "archived entry should survive Reconcile")
	assert.True(t, archived.IsArchived())
	assert.Equal(t, "gone-repo", archived.AnchorOverride, "anchor-heal must skip archived entries")
}

func TestFindByRef_SkipsArchivedEntries(t *testing.T) {
	t.Run("live entry wins over archived same-plan entry", func(t *testing.T) {
		setStateDir(t)
		oldDir := t.TempDir()
		archiveDir := t.TempDir()
		require.NoError(t, worktreeregistry.Save(&worktreeregistry.Entry{AbsPath: oldDir, Plan: "shared-plan"}))
		require.NoError(t, worktreeregistry.Archive(oldDir, archiveDir))

		liveDir := t.TempDir()
		require.NoError(t, worktreeregistry.Save(&worktreeregistry.Entry{AbsPath: liveDir, Plan: "shared-plan"}))

		entry, err := worktreeregistry.FindByRef("shared-plan")
		require.NoError(t, err, "archived entry must not make the lookup ambiguous")
		assert.Equal(t, liveDir, entry.AbsPath)
	})

	t.Run("only archived entries means no match", func(t *testing.T) {
		setStateDir(t)
		oldDir := t.TempDir()
		archiveDir := t.TempDir()
		require.NoError(t, worktreeregistry.Save(&worktreeregistry.Entry{AbsPath: oldDir, Plan: "gone-plan"}))
		require.NoError(t, worktreeregistry.Archive(oldDir, archiveDir))

		_, err := worktreeregistry.FindByRef("gone-plan")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no worktree found")
	})
}
