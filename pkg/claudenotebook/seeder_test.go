package claudenotebook_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grovetools/core/pkg/claudenotebook"
	"github.com/grovetools/core/util/pathutil"
)

// worktreeEditRule builds the canonical worktree Edit rule the seeder is
// expected to emit (symlink/case-resolved, matching Claude's resolved cwd).
func worktreeEditRule(t *testing.T, wt string) string {
	t.Helper()
	canon, err := pathutil.CanonicalPath(wt)
	if err != nil {
		canon = wt
	}
	return "Edit(//" + strings.TrimPrefix(canon, "/") + "/**)"
}

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
	// Unrelated nested field survives (the auto-derived Edit rules are appended
	// additively alongside it).
	assert.Contains(t, allowRules(t, root), "Bash")
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

// allowRules returns the permissions.allow string array (or empty if absent).
func allowRules(t *testing.T, root map[string]any) []string {
	t.Helper()
	perms, ok := root["permissions"].(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := perms["allow"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// ============================================================================
// Edit() auto-derivation tests (Task 1)
// ============================================================================

// (1) Edit-rule derivation: one Edit(//<dir>/**) per notebook dir + one for the
// worktree, with the exact "//" absolute-anchor format.
func TestSeedSettings_EditRuleDerivation(t *testing.T) {
	wt := t.TempDir()

	require.NoError(t, claudenotebook.SeedSettings(wt, nil, []string{"/abs/nbA", "/abs/nbB"}))

	root := readSettings(t, wt)
	rules := allowRules(t, root)
	assert.Contains(t, rules, "Edit(//abs/nbA/**)")
	assert.Contains(t, rules, "Edit(//abs/nbB/**)")
	assert.Contains(t, rules, worktreeEditRule(t, wt))
	assertNoTmpLeak(t, wt)
}

// (1b) The worktree Edit rule is CANONICALIZED (symlinks resolved) so it matches
// the cwd Claude compares against. On macOS t.TempDir() is a /var/folders symlink
// to /private/var/folders; the emitted rule must use the resolved form, never the
// raw symlink form (which would silently miss).
func TestSeedSettings_WorktreeEditRuleCanonicalized(t *testing.T) {
	wt := t.TempDir()

	require.NoError(t, claudenotebook.SeedSettings(wt, nil, []string{"/abs/nbA"}))

	rules := allowRules(t, readSettings(t, wt))
	canon, err := pathutil.CanonicalPath(wt)
	require.NoError(t, err)
	assert.Contains(t, rules, "Edit(//"+strings.TrimPrefix(canon, "/")+"/**)")
	if canon != wt {
		// The raw (unresolved) form must NOT appear — that was the bug.
		assert.NotContains(t, rules, "Edit(//"+strings.TrimPrefix(wt, "/")+"/**)")
	}
}

// (2a) Edit rules ride the notebook-dir gate, NOT the settings gate:
// GROVE_SEED_CLAUDE_SETTINGS=off with dirs present still emits Edit rules.
func TestSeedSettings_EditRulesRideDirGate_SettingsGateOff(t *testing.T) {
	wt := t.TempDir()
	t.Setenv("GROVE_SEED_CLAUDE_SETTINGS", "off")

	require.NoError(t, claudenotebook.SeedSettings(wt, nil, []string{"/abs/nbA"}))

	root := readSettings(t, wt)
	rules := allowRules(t, root)
	assert.Contains(t, rules, "Edit(//abs/nbA/**)")
	assert.Contains(t, rules, worktreeEditRule(t, wt))
}

// (2b) With the notebook-dir gate off and a nil cfg, nothing is written (no
// Edit rules, no file). The dir gate is enforced at the SeedNotebookDirs entry
// point (the notebook-dir path), which the Edit rules ride.
func TestSeedSettings_EditRulesRideDirGate_DirGateOff(t *testing.T) {
	wt := t.TempDir()
	t.Setenv("GROVE_SEED_CLAUDE_NOTEBOOK_DIRS", "off")

	require.NoError(t, claudenotebook.SeedNotebookDirs(wt, []string{"/abs/nbA"}))

	_, err := os.Stat(filepath.Join(wt, settingsRel))
	assert.True(t, os.IsNotExist(err), "dir gate off + nil cfg must not create the file")
}

// (3) Re-seeding dedups Edit rules and preserves user-added allow rules.
func TestSeedSettings_EditRuleDedupNoClobber(t *testing.T) {
	wt := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(wt, ".claude"), 0o755))

	// Pre-seed one Edit rule (the one we'll re-derive) + a user rule.
	preExisting := "Edit(//abs/nbA/**)"
	userRule := "Bash(make:*)"
	seed := map[string]any{
		"permissions": map[string]any{
			"allow": []any{preExisting, userRule},
		},
	}
	data, err := json.MarshalIndent(seed, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(wt, settingsRel), data, 0o644))

	require.NoError(t, claudenotebook.SeedSettings(wt, nil, []string{"/abs/nbA"}))

	rules := allowRules(t, readSettings(t, wt))
	// No duplicate of the pre-existing Edit rule.
	count := 0
	for _, r := range rules {
		if r == preExisting {
			count++
		}
	}
	assert.Equal(t, 1, count, "pre-existing Edit rule must not be duplicated")
	// User rule preserved.
	assert.Contains(t, rules, userRule, "user-added rule must be preserved")
}

// ============================================================================
// allowGroveTools expansion tests (Task 2)
// ============================================================================

// (4) allowGroveTools=true with empty Allow and nil dirs expands into
// Bash(<tool>:*) rules AND writes the file (proves the widened hasConfig gate).
func TestSeedSettings_AllowGroveToolsExpansion(t *testing.T) {
	wt := t.TempDir()

	cfg := &claudenotebook.ClaudeConfig{AllowGroveTools: boolPtr(true)}
	require.NoError(t, claudenotebook.SeedSettings(wt, cfg, nil))

	// The file is written even though the only signal is the flag.
	_, statErr := os.Stat(filepath.Join(wt, settingsRel))
	require.NoError(t, statErr, "lone allowGroveTools=true must write the file")

	rules := allowRules(t, readSettings(t, wt))
	for _, want := range []string{"Bash(grove:*)", "Bash(flow:*)", "Bash(groved:*)", "Bash(aglogs:*)", "Bash(nb:*)"} {
		assert.Contains(t, rules, want)
	}
	assertNoTmpLeak(t, wt)
}

// (5) allowGroveTools nil or explicit false expands into nothing.
func TestSeedSettings_AllowGroveToolsOffNoExpansion(t *testing.T) {
	t.Run("nil flag", func(t *testing.T) {
		wt := t.TempDir()
		cfg := &claudenotebook.ClaudeConfig{
			Permissions: claudenotebook.ClaudePermissions{Allow: []string{"Bash(git:*)"}},
		}
		require.NoError(t, claudenotebook.SeedSettings(wt, cfg, nil))
		assert.NotContains(t, allowRules(t, readSettings(t, wt)), "Bash(grove:*)")
	})

	t.Run("false flag", func(t *testing.T) {
		wt := t.TempDir()
		cfg := &claudenotebook.ClaudeConfig{
			AllowGroveTools: boolPtr(false),
			Permissions:     claudenotebook.ClaudePermissions{Allow: []string{"Bash(git:*)"}},
		}
		require.NoError(t, claudenotebook.SeedSettings(wt, cfg, nil))
		assert.NotContains(t, allowRules(t, readSettings(t, wt)), "Bash(grove:*)")
	})
}

// ============================================================================
// SeedSettings tests — ClaudeConfig support
// ============================================================================

// Helper to create a bool pointer.
func boolPtr(b bool) *bool { return &b }

// boolAt returns the boolean value at a nested path.
func boolAt(t *testing.T, root map[string]any, path ...string) bool {
	t.Helper()
	cur := root
	for _, key := range path[:len(path)-1] {
		child, ok := cur[key].(map[string]any)
		require.Truef(t, ok, "expected object at %q", key)
		cur = child
	}
	val, ok := cur[path[len(path)-1]].(bool)
	require.Truef(t, ok, "expected bool at %q", path[len(path)-1])
	return val
}

// optionalBoolAt returns the boolean value at a nested path, or nil if the key
// doesn't exist.
func optionalBoolAt(root map[string]any, path ...string) *bool {
	cur := root
	for _, key := range path[:len(path)-1] {
		child, ok := cur[key].(map[string]any)
		if !ok {
			return nil
		}
		cur = child
	}
	val, ok := cur[path[len(path)-1]].(bool)
	if !ok {
		return nil
	}
	return &val
}

// TestSeedSettings_WithClaudeConfig tests the full ClaudeConfig seeding.
func TestSeedSettings_WithClaudeConfig(t *testing.T) {
	wt := t.TempDir()

	cfg := &claudenotebook.ClaudeConfig{
		Permissions: claudenotebook.ClaudePermissions{
			Allow: []string{"Bash(git:*)", "Read(*.md)"},
		},
		Sandbox: claudenotebook.ClaudeSandbox{
			Enabled:                  boolPtr(true),
			FailIfUnavailable:        boolPtr(false),
			AutoAllowBashIfSandboxed: boolPtr(true),
			Filesystem: claudenotebook.ClaudeSandboxFilesystem{
				AllowWrite: []string{"/tmp/project"},
			},
			Network: claudenotebook.ClaudeSandboxNetwork{
				AllowedDomains: []string{"api.github.com", "registry.npmjs.org"},
			},
		},
	}
	notebookDirs := []string{"/Users/dev/notebooks/core"}

	require.NoError(t, claudenotebook.SeedSettings(wt, cfg, notebookDirs))

	root := readSettings(t, wt)

	// Check permissions.allow — config rules present (alongside the auto-derived
	// Edit rules from the notebook dir + worktree, which ride the dir gate).
	allow := stringSliceAt(t, root, "permissions", "allow")
	assert.Contains(t, allow, "Bash(git:*)")
	assert.Contains(t, allow, "Read(*.md)")
	assert.Contains(t, allow, "Edit(//Users/dev/notebooks/core/**)")

	// Check permissions.additionalDirectories (from notebook dirs)
	assert.ElementsMatch(t, []string{"/Users/dev/notebooks/core"}, additionalDirs(t, root))

	// Check sandbox booleans
	assert.True(t, boolAt(t, root, "sandbox", "enabled"))
	assert.False(t, boolAt(t, root, "sandbox", "failIfUnavailable"))
	assert.True(t, boolAt(t, root, "sandbox", "autoAllowBashIfSandboxed"))

	// Check sandbox.filesystem.allowWrite (notebook dirs + config dirs)
	allowWrite := allowWriteDirs(t, root)
	assert.Contains(t, allowWrite, "/Users/dev/notebooks/core")
	assert.Contains(t, allowWrite, "/tmp/project")

	// Check sandbox.network.allowedDomains
	domains := stringSliceAt(t, root, "sandbox", "network", "allowedDomains")
	assert.ElementsMatch(t, []string{"api.github.com", "registry.npmjs.org"}, domains)

	assertNoTmpLeak(t, wt)
}

// TestSeedSettings_MergeBoolNilVsFalseVsTrue tests that nil booleans are not
// written, false is written explicitly, and true is written.
func TestSeedSettings_MergeBoolNilVsFalseVsTrue(t *testing.T) {
	tests := []struct {
		name         string
		enabled      *bool
		expectExists bool
		expectValue  bool
	}{
		{
			name:         "nil - should not write key",
			enabled:      nil,
			expectExists: false,
		},
		{
			name:         "false - should write false",
			enabled:      boolPtr(false),
			expectExists: true,
			expectValue:  false,
		},
		{
			name:         "true - should write true",
			enabled:      boolPtr(true),
			expectExists: true,
			expectValue:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wt := t.TempDir()

			cfg := &claudenotebook.ClaudeConfig{
				Sandbox: claudenotebook.ClaudeSandbox{
					Enabled: tc.enabled,
				},
			}
			// Need at least one non-empty field to trigger writing.
			cfg.Permissions.Allow = []string{"Bash"}

			require.NoError(t, claudenotebook.SeedSettings(wt, cfg, nil))

			root := readSettings(t, wt)
			val := optionalBoolAt(root, "sandbox", "enabled")

			if tc.expectExists {
				require.NotNil(t, val, "expected sandbox.enabled to be written")
				assert.Equal(t, tc.expectValue, *val)
			} else {
				assert.Nil(t, val, "expected sandbox.enabled to NOT be written when nil")
			}
		})
	}
}

// TestSeedSettings_ArrayUnionDedupe tests that arrays from config are unioned
// with existing values and deduped.
func TestSeedSettings_ArrayUnionDedupe(t *testing.T) {
	wt := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(wt, ".claude"), 0o755))

	// Pre-populate with existing values.
	seed := map[string]any{
		"permissions": map[string]any{
			"allow": []any{"ExistingRule"},
		},
		"sandbox": map[string]any{
			"network": map[string]any{
				"allowedDomains": []any{"existing.com"},
			},
		},
	}
	data, err := json.MarshalIndent(seed, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(wt, settingsRel), data, 0o644))

	// Seed with overlapping and new values.
	cfg := &claudenotebook.ClaudeConfig{
		Permissions: claudenotebook.ClaudePermissions{
			Allow: []string{"ExistingRule", "NewRule"}, // ExistingRule is a duplicate
		},
		Sandbox: claudenotebook.ClaudeSandbox{
			Network: claudenotebook.ClaudeSandboxNetwork{
				AllowedDomains: []string{"existing.com", "new.com"}, // existing.com is a duplicate
			},
		},
	}

	require.NoError(t, claudenotebook.SeedSettings(wt, cfg, nil))

	root := readSettings(t, wt)

	// Check permissions.allow - should have both, no duplicates.
	allowRules := stringSliceAt(t, root, "permissions", "allow")
	assert.ElementsMatch(t, []string{"ExistingRule", "NewRule"}, allowRules)

	// Check sandbox.network.allowedDomains - should have both, no duplicates.
	domains := stringSliceAt(t, root, "sandbox", "network", "allowedDomains")
	assert.ElementsMatch(t, []string{"existing.com", "new.com"}, domains)
}

// TestSeedSettings_GateOff tests that GROVE_SEED_CLAUDE_SETTINGS=off skips
// ClaudeConfig seeding but still allows notebook dirs.
func TestSeedSettings_GateOff(t *testing.T) {
	for _, val := range []string{"0", "false", "off"} {
		t.Run(val, func(t *testing.T) {
			wt := t.TempDir()
			t.Setenv("GROVE_SEED_CLAUDE_SETTINGS", val)

			cfg := &claudenotebook.ClaudeConfig{
				Permissions: claudenotebook.ClaudePermissions{
					Allow: []string{"ShouldNotAppear"},
				},
				Sandbox: claudenotebook.ClaudeSandbox{
					Enabled: boolPtr(true),
				},
			}
			notebookDirs := []string{"/Users/dev/notebooks/core"}

			require.NoError(t, claudenotebook.SeedSettings(wt, cfg, notebookDirs))

			root := readSettings(t, wt)

			// Notebook dirs should still be seeded.
			assert.Contains(t, additionalDirs(t, root), "/Users/dev/notebooks/core")
			assert.Contains(t, allowWriteDirs(t, root), "/Users/dev/notebooks/core")

			// ClaudeConfig allow rules should NOT be seeded when the settings
			// gate is off. (The auto-derived Edit rules DO ride the dir gate, so
			// permissions.allow may exist — but the config's "ShouldNotAppear"
			// rule must be absent.)
			assert.NotContains(t, allowRules(t, root), "ShouldNotAppear",
				"config permissions.allow should not be written when gate is off")

			sandbox, ok := root["sandbox"].(map[string]any)
			require.True(t, ok)
			_, hasEnabled := sandbox["enabled"]
			assert.False(t, hasEnabled, "sandbox.enabled should not be written when gate is off")

			assertNoTmpLeak(t, wt)
		})
	}
}

// TestSeedSettings_EmptyConfigNoOp tests that an empty config with no dirs is
// a no-op (no file created).
func TestSeedSettings_EmptyConfigNoOp(t *testing.T) {
	wt := t.TempDir()

	// Empty config and no notebook dirs.
	require.NoError(t, claudenotebook.SeedSettings(wt, nil, nil))

	_, err := os.Stat(filepath.Join(wt, settingsRel))
	assert.True(t, os.IsNotExist(err), "no file should be created with empty config and no dirs")
}

// TestSeedSettings_NilConfigWithDirs tests that passing nil config with dirs
// works (delegates to notebook-only seeding).
func TestSeedSettings_NilConfigWithDirs(t *testing.T) {
	wt := t.TempDir()

	require.NoError(t, claudenotebook.SeedSettings(wt, nil, []string{"/Users/dev/notebooks/core"}))

	root := readSettings(t, wt)
	assert.Contains(t, additionalDirs(t, root), "/Users/dev/notebooks/core")
	assert.Contains(t, allowWriteDirs(t, root), "/Users/dev/notebooks/core")
}

// TestSeedSettings_MalformedJSONNoOp tests that malformed JSON returns an error
// and leaves the file unchanged (same behavior as SeedNotebookDirs).
func TestSeedSettings_MalformedJSONNoOp(t *testing.T) {
	wt := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(wt, ".claude"), 0o755))
	settingsPath := filepath.Join(wt, settingsRel)
	garbage := []byte("{ not valid json ]]")
	require.NoError(t, os.WriteFile(settingsPath, garbage, 0o644))

	cfg := &claudenotebook.ClaudeConfig{
		Permissions: claudenotebook.ClaudePermissions{
			Allow: []string{"Bash"},
		},
	}
	err := claudenotebook.SeedSettings(wt, cfg, nil)
	require.Error(t, err, "malformed JSON must return an error")

	after, readErr := os.ReadFile(settingsPath)
	require.NoError(t, readErr)
	assert.Equal(t, garbage, after, "malformed file must be left byte-for-byte unchanged")
	assertNoTmpLeak(t, wt)
}

// TestSeedSettings_AtomicWrite tests that the atomic write behavior is preserved.
func TestSeedSettings_AtomicWrite(t *testing.T) {
	wt := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(wt, ".claude"), 0o755))
	settingsPath := filepath.Join(wt, settingsRel)
	require.NoError(t, os.WriteFile(settingsPath, []byte(`{}`), 0o600))

	cfg := &claudenotebook.ClaudeConfig{
		Permissions: claudenotebook.ClaudePermissions{
			Allow: []string{"Bash"},
		},
	}
	require.NoError(t, claudenotebook.SeedSettings(wt, cfg, nil))

	info, err := os.Stat(settingsPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "file mode preserved across rewrite")
	assertNoTmpLeak(t, wt)
}

// TestSeedSettings_BoolOverwritesExisting tests that explicitly set booleans
// overwrite existing values in the file (grove.toml wins).
func TestSeedSettings_BoolOverwritesExisting(t *testing.T) {
	wt := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(wt, ".claude"), 0o755))

	// Pre-populate with existing boolean values.
	seed := map[string]any{
		"sandbox": map[string]any{
			"enabled":           false, // Will be overwritten to true
			"failIfUnavailable": true,  // Will be overwritten to false
		},
	}
	data, err := json.MarshalIndent(seed, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(wt, settingsRel), data, 0o644))

	cfg := &claudenotebook.ClaudeConfig{
		Sandbox: claudenotebook.ClaudeSandbox{
			Enabled:           boolPtr(true),
			FailIfUnavailable: boolPtr(false),
		},
	}

	require.NoError(t, claudenotebook.SeedSettings(wt, cfg, nil))

	root := readSettings(t, wt)
	assert.True(t, boolAt(t, root, "sandbox", "enabled"), "enabled should be overwritten to true")
	assert.False(t, boolAt(t, root, "sandbox", "failIfUnavailable"), "failIfUnavailable should be overwritten to false")
}

// optionalStringAt returns the string value at a nested path, or nil if the key
// doesn't exist (or isn't a string).
func optionalStringAt(root map[string]any, path ...string) *string {
	cur := root
	for _, key := range path[:len(path)-1] {
		child, ok := cur[key].(map[string]any)
		if !ok {
			return nil
		}
		cur = child
	}
	val, ok := cur[path[len(path)-1]].(string)
	if !ok {
		return nil
	}
	return &val
}

// TestSeedSettings_DefaultModeWritten confirms permissions.defaultMode is
// written into settings.local.json when set on the config.
func TestSeedSettings_DefaultModeWritten(t *testing.T) {
	wt := t.TempDir()

	cfg := &claudenotebook.ClaudeConfig{
		Permissions: claudenotebook.ClaudePermissions{
			DefaultMode: "bypassPermissions",
		},
	}
	require.NoError(t, claudenotebook.SeedSettings(wt, cfg, nil))

	root := readSettings(t, wt)
	got := optionalStringAt(root, "permissions", "defaultMode")
	require.NotNil(t, got, "expected permissions.defaultMode to be written")
	assert.Equal(t, "bypassPermissions", *got)
	assertNoTmpLeak(t, wt)
}

// TestSeedSettings_DefaultModeAbsentWhenUnset confirms the key is NOT written
// when DefaultMode is empty (so we never introduce an empty value).
func TestSeedSettings_DefaultModeAbsentWhenUnset(t *testing.T) {
	wt := t.TempDir()

	// A config with other content (so the file is written) but no defaultMode.
	cfg := &claudenotebook.ClaudeConfig{
		Permissions: claudenotebook.ClaudePermissions{
			Allow: []string{"Bash(git:*)"},
		},
	}
	require.NoError(t, claudenotebook.SeedSettings(wt, cfg, nil))

	root := readSettings(t, wt)
	assert.Nil(t, optionalStringAt(root, "permissions", "defaultMode"),
		"permissions.defaultMode must be absent when unset")
}

// TestSeedSettings_DefaultModeNoClobberWhenUnset confirms a pre-existing user
// defaultMode is preserved (not overwritten with empty) when the config leaves
// DefaultMode unset.
func TestSeedSettings_DefaultModeNoClobberWhenUnset(t *testing.T) {
	wt := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(wt, ".claude"), 0o755))

	seed := map[string]any{
		"permissions": map[string]any{
			"defaultMode": "acceptEdits", // user-set value
		},
	}
	data, err := json.MarshalIndent(seed, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(wt, settingsRel), data, 0o644))

	// Config has content (forces a write) but no defaultMode.
	cfg := &claudenotebook.ClaudeConfig{
		Permissions: claudenotebook.ClaudePermissions{Allow: []string{"Bash(git:*)"}},
	}
	require.NoError(t, claudenotebook.SeedSettings(wt, cfg, nil))

	root := readSettings(t, wt)
	got := optionalStringAt(root, "permissions", "defaultMode")
	require.NotNil(t, got, "pre-existing defaultMode must be preserved")
	assert.Equal(t, "acceptEdits", *got, "unset config must not clobber user's defaultMode")
}

// TestSeedSettings_DefaultModeOverwritesExisting confirms an explicit config
// defaultMode wins over an existing value (grove.toml wins, like the booleans).
func TestSeedSettings_DefaultModeOverwritesExisting(t *testing.T) {
	wt := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(wt, ".claude"), 0o755))

	seed := map[string]any{
		"permissions": map[string]any{"defaultMode": "plan"},
	}
	data, err := json.MarshalIndent(seed, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(wt, settingsRel), data, 0o644))

	cfg := &claudenotebook.ClaudeConfig{
		Permissions: claudenotebook.ClaudePermissions{DefaultMode: "bypassPermissions"},
	}
	require.NoError(t, claudenotebook.SeedSettings(wt, cfg, nil))

	root := readSettings(t, wt)
	got := optionalStringAt(root, "permissions", "defaultMode")
	require.NotNil(t, got)
	assert.Equal(t, "bypassPermissions", *got, "explicit config defaultMode should win")
}

// TestSeedSettings_DefaultModeGateOff confirms defaultMode is NOT written when
// the settings gate is off (it rides the same gate as the other config fields).
func TestSeedSettings_DefaultModeGateOff(t *testing.T) {
	wt := t.TempDir()
	t.Setenv("GROVE_SEED_CLAUDE_SETTINGS", "off")

	cfg := &claudenotebook.ClaudeConfig{
		Permissions: claudenotebook.ClaudePermissions{DefaultMode: "bypassPermissions"},
	}
	require.NoError(t, claudenotebook.SeedSettings(wt, cfg, nil))

	_, err := os.Stat(filepath.Join(wt, settingsRel))
	assert.True(t, os.IsNotExist(err), "gate off + lone defaultMode must not write the file")
}

// ============================================================================
// ClaudeConfig.Merge tests
// ============================================================================

// TestClaudeConfig_Merge tests the Merge function for ClaudeConfig.
func TestClaudeConfig_Merge(t *testing.T) {
	t.Run("arrays are unioned", func(t *testing.T) {
		root := claudenotebook.ClaudeConfig{
			Permissions: claudenotebook.ClaudePermissions{
				Allow: []string{"RuleA"},
			},
			Sandbox: claudenotebook.ClaudeSandbox{
				Network: claudenotebook.ClaudeSandboxNetwork{
					AllowedDomains: []string{"a.com"},
				},
			},
		}
		member := claudenotebook.ClaudeConfig{
			Permissions: claudenotebook.ClaudePermissions{
				Allow: []string{"RuleB", "RuleA"}, // RuleA is duplicate
			},
			Sandbox: claudenotebook.ClaudeSandbox{
				Network: claudenotebook.ClaudeSandboxNetwork{
					AllowedDomains: []string{"b.com"},
				},
			},
		}

		root.Merge(&member)

		assert.ElementsMatch(t, []string{"RuleA", "RuleB"}, root.Permissions.Allow)
		assert.ElementsMatch(t, []string{"a.com", "b.com"}, root.Sandbox.Network.AllowedDomains)
	})

	t.Run("root booleans win", func(t *testing.T) {
		root := claudenotebook.ClaudeConfig{
			Sandbox: claudenotebook.ClaudeSandbox{
				Enabled: boolPtr(true),
			},
		}
		member := claudenotebook.ClaudeConfig{
			Sandbox: claudenotebook.ClaudeSandbox{
				Enabled: boolPtr(false), // Should NOT override root
			},
		}

		root.Merge(&member)

		require.NotNil(t, root.Sandbox.Enabled)
		assert.True(t, *root.Sandbox.Enabled, "root boolean should win")
	})

	t.Run("member fills nil root booleans", func(t *testing.T) {
		root := claudenotebook.ClaudeConfig{
			Sandbox: claudenotebook.ClaudeSandbox{
				Enabled: nil, // Not set
			},
		}
		member := claudenotebook.ClaudeConfig{
			Sandbox: claudenotebook.ClaudeSandbox{
				Enabled: boolPtr(true), // Should fill the gap
			},
		}

		root.Merge(&member)

		require.NotNil(t, root.Sandbox.Enabled)
		assert.True(t, *root.Sandbox.Enabled, "member should fill nil root boolean")
	})

	t.Run("merge nil does nothing", func(t *testing.T) {
		root := claudenotebook.ClaudeConfig{
			Permissions: claudenotebook.ClaudePermissions{
				Allow: []string{"Rule"},
			},
		}

		root.Merge(nil)

		assert.Equal(t, []string{"Rule"}, root.Permissions.Allow)
	})
}

// TestClaudeConfig_IsEmpty tests the IsEmpty function.
func TestClaudeConfig_IsEmpty(t *testing.T) {
	t.Run("empty config", func(t *testing.T) {
		cfg := claudenotebook.ClaudeConfig{}
		assert.True(t, cfg.IsEmpty())
	})

	t.Run("non-empty with allow", func(t *testing.T) {
		cfg := claudenotebook.ClaudeConfig{
			Permissions: claudenotebook.ClaudePermissions{
				Allow: []string{"Rule"},
			},
		}
		assert.False(t, cfg.IsEmpty())
	})

	t.Run("non-empty with boolean", func(t *testing.T) {
		cfg := claudenotebook.ClaudeConfig{
			Sandbox: claudenotebook.ClaudeSandbox{
				Enabled: boolPtr(false),
			},
		}
		assert.False(t, cfg.IsEmpty())
	})

	t.Run("non-empty with domains", func(t *testing.T) {
		cfg := claudenotebook.ClaudeConfig{
			Sandbox: claudenotebook.ClaudeSandbox{
				Network: claudenotebook.ClaudeSandboxNetwork{
					AllowedDomains: []string{"a.com"},
				},
			},
		}
		assert.False(t, cfg.IsEmpty())
	})
}
