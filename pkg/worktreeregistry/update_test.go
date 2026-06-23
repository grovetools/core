package worktreeregistry_test

import (
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grovetools/core/pkg/worktreeregistry"
	"github.com/grovetools/core/util/pathutil"
)

func TestUpdate_MutatesEntry(t *testing.T) {
	setStateDir(t)
	dir := t.TempDir()

	require.NoError(t, worktreeregistry.Save(&worktreeregistry.Entry{AbsPath: dir}))

	id := pathutil.WorktreeID(dir)
	err := worktreeregistry.Update(id, func(e *worktreeregistry.Entry) {
		if e.SessionState == nil {
			e.SessionState = map[string]interface{}{}
		}
		e.SessionState["review:core/main.go@abc"] = true
	})
	require.NoError(t, err)

	loaded, err := worktreeregistry.Load(id)
	require.NoError(t, err)
	assert.Equal(t, true, loaded.SessionState["review:core/main.go@abc"])
}

func TestUpdate_MissingEntryReturnsNotExist(t *testing.T) {
	setStateDir(t)

	id := pathutil.WorktreeID(t.TempDir())
	err := worktreeregistry.Update(id, func(*worktreeregistry.Entry) {})
	require.Error(t, err)
	assert.True(t, os.IsNotExist(err), "Update on a missing entry should return an os.IsNotExist error")
}

// TestUpdate_ConcurrentNoLostWrites is the core concurrency guarantee: 10
// goroutines each writing a distinct key into the SAME entry's SessionState
// must all survive. A bare Load→mutate→Save (no serialization) would lose
// writes because each goroutine reads a stale copy and Save persists the whole
// Entry (last-write-wins). Update's registryMu serializes the read-modify-write
// so every key lands.
func TestUpdate_ConcurrentNoLostWrites(t *testing.T) {
	setStateDir(t)
	dir := t.TempDir()

	require.NoError(t, worktreeregistry.Save(&worktreeregistry.Entry{
		AbsPath:      dir,
		SessionState: map[string]interface{}{},
	}))
	id := pathutil.WorktreeID(dir)

	const n = 10
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			err := worktreeregistry.Update(id, func(e *worktreeregistry.Entry) {
				if e.SessionState == nil {
					e.SessionState = map[string]interface{}{}
				}
				e.SessionState[fmt.Sprintf("review:repo/file-%d.go@hash", i)] = true
			})
			assert.NoError(t, err)
		}()
	}
	wg.Wait()

	loaded, err := worktreeregistry.Load(id)
	require.NoError(t, err)
	require.NotNil(t, loaded.SessionState)
	assert.Len(t, loaded.SessionState, n, "all concurrent writes should be preserved")
	for i := 0; i < n; i++ {
		key := fmt.Sprintf("review:repo/file-%d.go@hash", i)
		assert.Equal(t, true, loaded.SessionState[key], "missing key %s — write was lost", key)
	}
}
