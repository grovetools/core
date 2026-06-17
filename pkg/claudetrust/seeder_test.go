package claudetrust_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grovetools/core/pkg/claudetrust"
)

// readConfig reads and parses ~/.claude.json under the test HOME.
func readConfig(t *testing.T, home string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, ".claude.json"))
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))
	return m
}

// trusted reports whether projects[path].hasTrustDialogAccepted == true.
func trusted(t *testing.T, root map[string]any, path string) bool {
	t.Helper()
	projects, ok := root["projects"].(map[string]any)
	require.True(t, ok, "projects must be a map")
	entry, ok := projects[path].(map[string]any)
	if !ok {
		return false
	}
	return entry["hasTrustDialogAccepted"] == true
}

// assertNoTmpLeak fails if a .claude.json.tmp lingers in HOME.
func assertNoTmpLeak(t *testing.T, home string) {
	t.Helper()
	_, err := os.Stat(filepath.Join(home, ".claude.json.tmp"))
	assert.True(t, os.IsNotExist(err), "no .claude.json.tmp should be left behind")
}

// (a) Missing file -> created, every path trusted.
func TestSeedTrust_MissingFileCreated(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	p1 := "/Users/dev/proj/feat"
	p2 := "/Users/dev/proj/feat/svc-a"
	require.NoError(t, claudetrust.SeedTrust(p1, p2))

	root := readConfig(t, home)
	assert.True(t, trusted(t, root, p1), "container path trusted")
	assert.True(t, trusted(t, root, p2), "repo subdir trusted")
	assertNoTmpLeak(t, home)
}

// (b) Preserves an unrelated projects entry AND an unknown top-level key.
func TestSeedTrust_PreservesUnrelatedAndTopLevel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	seed := map[string]any{
		"numStartups":   7.0, // unknown top-level key
		"installMethod": "brew",
		"projects": map[string]any{
			"/Users/dev/other": map[string]any{
				"hasTrustDialogAccepted": true,
				"someUserField":          "keep-me",
			},
		},
	}
	data, err := json.MarshalIndent(seed, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(home, ".claude.json"), data, 0o600))

	target := "/Users/dev/proj/feat"
	require.NoError(t, claudetrust.SeedTrust(target))

	root := readConfig(t, home)
	// New target is trusted.
	assert.True(t, trusted(t, root, target))
	// Unknown top-level keys survive.
	assert.Equal(t, 7.0, root["numStartups"])
	assert.Equal(t, "brew", root["installMethod"])
	// Unrelated project entry and its fields survive untouched.
	projects := root["projects"].(map[string]any)
	other := projects["/Users/dev/other"].(map[string]any)
	assert.Equal(t, "keep-me", other["someUserField"])
	assert.Equal(t, true, other["hasTrustDialogAccepted"])
	assertNoTmpLeak(t, home)
}

// (c) Existing target entry: other fields preserved, flag flips to true.
func TestSeedTrust_ExistingEntryFlagFlips(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	target := "/Users/dev/proj/feat"
	seed := map[string]any{
		"projects": map[string]any{
			target: map[string]any{
				"hasTrustDialogAccepted": false,
				"history":                []any{"a", "b"},
				"allowedTools":           []any{"Bash"},
			},
		},
	}
	data, err := json.MarshalIndent(seed, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(home, ".claude.json"), data, 0o600))

	require.NoError(t, claudetrust.SeedTrust(target))

	root := readConfig(t, home)
	projects := root["projects"].(map[string]any)
	entry := projects[target].(map[string]any)
	assert.Equal(t, true, entry["hasTrustDialogAccepted"], "flag flipped true")
	assert.Equal(t, []any{"a", "b"}, entry["history"], "history preserved")
	assert.Equal(t, []any{"Bash"}, entry["allowedTools"], "allowedTools preserved")
	assertNoTmpLeak(t, home)
}

// (d) Malformed JSON -> error, file untouched, no panic.
func TestSeedTrust_MalformedNoOp(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	configPath := filepath.Join(home, ".claude.json")
	garbage := []byte("{ this is not valid json ]]")
	require.NoError(t, os.WriteFile(configPath, garbage, 0o600))

	err := claudetrust.SeedTrust("/Users/dev/proj/feat")
	require.Error(t, err, "malformed JSON must return an error")

	after, readErr := os.ReadFile(configPath)
	require.NoError(t, readErr)
	assert.Equal(t, garbage, after, "malformed file must be left byte-for-byte unchanged")
	assertNoTmpLeak(t, home)
}

// (e) Gate off -> file untouched / never created.
func TestSeedTrust_GateOff(t *testing.T) {
	for _, val := range []string{"0", "false", "off"} {
		t.Run(val, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("GROVE_PRESEED_CLAUDE_TRUST", val)

			require.NoError(t, claudetrust.SeedTrust("/Users/dev/proj/feat"))

			_, err := os.Stat(filepath.Join(home, ".claude.json"))
			assert.True(t, os.IsNotExist(err), "gate off must not create the file")
			assertNoTmpLeak(t, home)
		})
	}
}

// (f) No .claude.json.tmp left after a normal seed (explicit, beyond the
// per-case checks above).
func TestSeedTrust_NoTmpLeak(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	require.NoError(t, claudetrust.SeedTrust("/Users/dev/proj/feat"))
	assertNoTmpLeak(t, home)
}

// Preserves the file mode of an existing file across the atomic rewrite.
func TestSeedTrust_PreservesMode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	configPath := filepath.Join(home, ".claude.json")
	require.NoError(t, os.WriteFile(configPath, []byte(`{"projects":{}}`), 0o600))

	require.NoError(t, claudetrust.SeedTrust("/Users/dev/proj/feat"))

	info, err := os.Stat(configPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}
