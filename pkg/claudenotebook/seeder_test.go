package claudenotebook_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grovetools/core/pkg/claudenotebook"
)

const settingsRel = ".claude/settings.local.json"

// readSettings reads and parses .claude/settings.local.json under worktree.
func readSettings(t *testing.T, worktree string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(worktree, settingsRel))
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))
	return m
}

// stringSliceAt returns the string array living at the nested object path.
func stringSliceAt(t *testing.T, root map[string]any, path ...string) []string {
	t.Helper()
	cur := root
	for _, key := range path[:len(path)-1] {
		child, ok := cur[key].(map[string]any)
		require.Truef(t, ok, "expected object at %q", key)
		cur = child
	}
	raw, ok := cur[path[len(path)-1]].([]any)
	require.Truef(t, ok, "expected array at %q", path[len(path)-1])
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		s, ok := item.(string)
		require.True(t, ok, "array element must be a string")
		out = append(out, s)
	}
	return out
}

// assertNoTmpLeak fails if a settings.local.json.tmp lingers in the worktree.
func assertNoTmpLeak(t *testing.T, worktree string) {
	t.Helper()
	_, err := os.Stat(filepath.Join(worktree, settingsRel+".tmp"))
	assert.True(t, os.IsNotExist(err), "no settings.local.json.tmp should be left behind")
}

const (
	addlDirsKey  = "additionalDirectories"
	allowWrite0  = "permissions"
	sandboxKey   = "sandbox"
	fsKey        = "filesystem"
	allowWriteLf = "allowWrite"
)

func additionalDirs(t *testing.T, root map[string]any) []string {
	return stringSliceAt(t, root, allowWrite0, addlDirsKey)
}

func allowWriteDirs(t *testing.T, root map[string]any) []string {
	return stringSliceAt(t, root, sandboxKey, fsKey, allowWriteLf)
}

// (a) Missing settings.local.json -> created, both keys present with the dirs.
func TestSeedNotebookDirs_MissingFileCreated(t *testing.T) {
	wt := t.TempDir()

	d1 := "/Users/dev/notebooks/grovetools/workspaces/core"
	d2 := "/Users/dev/notebooks/grovetools/workspaces/nb"
	require.NoError(t, claudenotebook.SeedNotebookDirs(wt, []string{d1, d2}))

	root := readSettings(t, wt)
	assert.ElementsMatch(t, []string{d1, d2}, additionalDirs(t, root))
	assert.ElementsMatch(t, []string{d1, d2}, allowWriteDirs(t, root))
	assertNoTmpLeak(t, wt)
}

// (b) Existing file with unrelated top-level keys + a user-added entry in each
// array -> preserved AND notebook dirs appended (non-destructive).
func TestSeedNotebookDirs_PreservesAndAppends(t *testing.T) {
	wt := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(wt, ".claude"), 0o755))

	userRead := "/Users/dev/manual-read-dir"
	userWrite := "/Users/dev/manual-write-dir"
	seed := map[string]any{
		"model":         "claude-opus-4-8", // unrelated top-level key
		"someUserField": "keep-me",
		"permissions": map[string]any{
			"additionalDirectories": []any{userRead},
			"allow":                 []any{"Bash"}, // unrelated nested field
		},
		"sandbox": map[string]any{
			"filesystem": map[string]any{
				"allowWrite": []any{userWrite},
			},
		},
	}
	data, err := json.MarshalIndent(seed, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(wt, settingsRel), data, 0o644))

	nb := "/Users/dev/notebooks/grovetools/workspaces/core"
	require.NoError(t, claudenotebook.SeedNotebookDirs(wt, []string{nb}))

	root := readSettings(t, wt)
	// Unrelated keys survive.
	assert.Equal(t, "claude-opus-4-8", root["model"])
	assert.Equal(t, "keep-me", root["someUserField"])
	// Unrelated nested field survives.
	perms := root["permissions"].(map[string]any)
	assert.Equal(t, []any{"Bash"}, perms["allow"])
	// User entries preserved AND notebook dir appended in both arrays.
	assert.ElementsMatch(t, []string{userRead, nb}, additionalDirs(t, root))
	assert.ElementsMatch(t, []string{userWrite, nb}, allowWriteDirs(t, root))
	assertNoTmpLeak(t, wt)
}

// (c) Duplicate/empty input dirs -> deduped, sorted, empties dropped.
func TestSeedNotebookDirs_DedupSortDropEmpty(t *testing.T) {
	wt := t.TempDir()

	b := "/Users/dev/notebooks/b"
	a := "/Users/dev/notebooks/a"
	require.NoError(t, claudenotebook.SeedNotebookDirs(wt, []string{b, a, b, "", a, ""}))

	root := readSettings(t, wt)
	// Sorted + deduped + no empties.
	assert.Equal(t, []string{a, b}, additionalDirs(t, root))
	assert.Equal(t, []string{a, b}, allowWriteDirs(t, root))
}

// (d) Malformed JSON -> returns error, file left byte-for-byte unchanged.
func TestSeedNotebookDirs_MalformedNoOp(t *testing.T) {
	wt := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(wt, ".claude"), 0o755))
	settingsPath := filepath.Join(wt, settingsRel)
	garbage := []byte("{ not valid json ]]")
	require.NoError(t, os.WriteFile(settingsPath, garbage, 0o644))

	err := claudenotebook.SeedNotebookDirs(wt, []string{"/Users/dev/notebooks/core"})
	require.Error(t, err, "malformed JSON must return an error")

	after, readErr := os.ReadFile(settingsPath)
	require.NoError(t, readErr)
	assert.Equal(t, garbage, after, "malformed file must be left byte-for-byte unchanged")
	assertNoTmpLeak(t, wt)
}

// (e) Gate off (GROVE_SEED_CLAUDE_NOTEBOOK_DIRS in {0,false,off}) -> no-op, file
// not created.
func TestSeedNotebookDirs_GateOff(t *testing.T) {
	for _, val := range []string{"0", "false", "off"} {
		t.Run(val, func(t *testing.T) {
			wt := t.TempDir()
			t.Setenv("GROVE_SEED_CLAUDE_NOTEBOOK_DIRS", val)

			require.NoError(t, claudenotebook.SeedNotebookDirs(wt, []string{"/Users/dev/notebooks/core"}))

			_, err := os.Stat(filepath.Join(wt, settingsRel))
			assert.True(t, os.IsNotExist(err), "gate off must not create the file")
			assertNoTmpLeak(t, wt)
		})
	}
}

// (f) Both permissions.additionalDirectories AND sandbox.filesystem.allowWrite
// are written. (Covered above; asserted explicitly here on a fresh file.)
func TestSeedNotebookDirs_BothKeysWritten(t *testing.T) {
	wt := t.TempDir()
	nb := "/Users/dev/notebooks/grovetools/workspaces/core"
	require.NoError(t, claudenotebook.SeedNotebookDirs(wt, []string{nb}))

	root := readSettings(t, wt)
	assert.Contains(t, additionalDirs(t, root), nb, "permissions.additionalDirectories written")
	assert.Contains(t, allowWriteDirs(t, root), nb, "sandbox.filesystem.allowWrite written")
}

// (g) No settings.local.json.tmp left behind after a normal seed (explicit).
func TestSeedNotebookDirs_NoTmpLeak(t *testing.T) {
	wt := t.TempDir()
	require.NoError(t, claudenotebook.SeedNotebookDirs(wt, []string{"/Users/dev/notebooks/core"}))
	assertNoTmpLeak(t, wt)
}

// (h) Existing file mode preserved across the atomic rewrite.
func TestSeedNotebookDirs_PreservesMode(t *testing.T) {
	wt := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(wt, ".claude"), 0o755))
	settingsPath := filepath.Join(wt, settingsRel)
	require.NoError(t, os.WriteFile(settingsPath, []byte(`{}`), 0o600))

	require.NoError(t, claudenotebook.SeedNotebookDirs(wt, []string{"/Users/dev/notebooks/core"}))

	info, err := os.Stat(settingsPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "file mode preserved across rewrite")
}

// Re-seeding the same dirs is idempotent: no duplicate entries accumulate.
func TestSeedNotebookDirs_Idempotent(t *testing.T) {
	wt := t.TempDir()
	nb := "/Users/dev/notebooks/grovetools/workspaces/core"
	require.NoError(t, claudenotebook.SeedNotebookDirs(wt, []string{nb}))
	require.NoError(t, claudenotebook.SeedNotebookDirs(wt, []string{nb}))

	root := readSettings(t, wt)
	assert.Equal(t, []string{nb}, additionalDirs(t, root), "no duplicate on re-seed")
	assert.Equal(t, []string{nb}, allowWriteDirs(t, root), "no duplicate on re-seed")
}
