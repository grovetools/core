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
	"github.com/grovetools/core/pkg/paths"
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

// worktreeAllowWrite returns the canonicalized worktree root the seeder is
// expected to auto-add to sandbox.filesystem.allowWrite (the write-side
// complement to the notebook-dir merge). Canonicalization mirrors the seeder
// (and worktreeEditRule), so on macOS a /var/folders tmp dir resolves to its
// /private/var/folders form.
func worktreeAllowWrite(t *testing.T, wt string) string {
	t.Helper()
	canon, err := pathutil.CanonicalPath(wt)
	if err != nil {
		canon = wt
	}
	return canon
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
	// allowWrite carries the notebook dirs PLUS the auto-added worktree root.
	assert.ElementsMatch(t, []string{d1, d2, worktreeAllowWrite(t, wt)}, allowWriteDirs(t, root))
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
	// User write entry + notebook dir + auto-added worktree root all coexist.
	assert.ElementsMatch(t, []string{userWrite, nb, worktreeAllowWrite(t, wt)}, allowWriteDirs(t, root))
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
	// allowWrite is the sorted notebook dirs plus the auto-added worktree root.
	assert.ElementsMatch(t, []string{a, b, worktreeAllowWrite(t, wt)}, allowWriteDirs(t, root))
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
	// The auto-added worktree root is also idempotent (no duplicate on re-seed).
	assert.ElementsMatch(t, []string{nb, worktreeAllowWrite(t, wt)}, allowWriteDirs(t, root), "no duplicate on re-seed")
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

	require.NoError(t, claudenotebook.SeedSettings(wt, nil, nil, []string{"/abs/nbA", "/abs/nbB"}))

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

	require.NoError(t, claudenotebook.SeedSettings(wt, nil, nil, []string{"/abs/nbA"}))

	rules := allowRules(t, readSettings(t, wt))
	canon, err := pathutil.CanonicalPath(wt)
	require.NoError(t, err)
	assert.Contains(t, rules, "Edit(//"+strings.TrimPrefix(canon, "/")+"/**)")
	if canon != wt {
		// The raw (unresolved) form must NOT appear — that was the bug.
		assert.NotContains(t, rules, "Edit(//"+strings.TrimPrefix(wt, "/")+"/**)")
	}
}

// Worktree-root allowWrite (the write-side complement to notebook-dir seeding).
// ============================================================================

// (wr-a) The canonicalized worktree root is auto-added to
// sandbox.filesystem.allowWrite so a sandboxed agent can Bash-write its own repo
// tree. The raw (unresolved) form must NOT appear — it would silently miss the
// /private/var/... cwd Claude resolves on macOS.
func TestSeedSettings_WorktreeRootInAllowWrite(t *testing.T) {
	wt := t.TempDir()

	require.NoError(t, claudenotebook.SeedSettings(wt, nil, nil, []string{"/abs/nbA"}))

	allow := allowWriteDirs(t, readSettings(t, wt))
	canon, err := pathutil.CanonicalPath(wt)
	require.NoError(t, err)
	assert.Contains(t, allow, canon, "canonical worktree root must be in allowWrite")
	if canon != wt {
		assert.NotContains(t, allow, wt, "raw (unresolved) worktree path must NOT appear")
	}
}

// (wr-a') The worktree root is added even on a config-only seed with NO notebook
// dirs: it fires whenever settings are seeded for a worktree, not only when the
// notebook-dir block runs.
func TestSeedSettings_WorktreeRootInAllowWrite_NoNotebookDirs(t *testing.T) {
	wt := t.TempDir()
	cfg := &claudenotebook.ClaudeConfig{Sandbox: claudenotebook.ClaudeSandbox{Enabled: boolPtr(true)}}

	require.NoError(t, claudenotebook.SeedSettings(wt, nil, cfg, nil))

	assert.Contains(t, allowWriteDirs(t, readSettings(t, wt)), worktreeAllowWrite(t, wt),
		"worktree root must be added even with no notebook dirs")
}

// (wr-b) With protectConfig on, the worktree-root allowWrite COEXISTS with the
// grove-owned denyWrite for the worktree's grove.toml. denyWrite takes
// precedence over allowWrite (schema), so adding the root (which CONTAINS
// grove.toml) does NOT un-protect it — the deny entry is still present.
func TestSeedSettings_WorktreeRootAllowWriteCoexistsWithProtectDenyWrite(t *testing.T) {
	wt := t.TempDir()
	cfg := &claudenotebook.ClaudeConfig{
		ProtectConfig: boolPtr(true),
		Sandbox:       claudenotebook.ClaudeSandbox{Enabled: boolPtr(true)},
	}
	require.NoError(t, claudenotebook.SeedSettings(wt, nil, cfg, nil))

	root := readSettings(t, wt)
	canonWt := worktreeAllowWrite(t, wt)
	// The worktree root is in allowWrite ...
	assert.Contains(t, allowWriteDirs(t, root), canonWt, "worktree root present in allowWrite")
	// ... AND the grove-owned denyWrite for its grove.toml is still present, even
	// though grove.toml lives inside the worktree-root subtree. denyWrite wins.
	assert.Contains(t, denyWriteAt(t, root), filepath.Join(canonWt, "grove.toml"),
		"grove.toml denyWrite must coexist with the worktree-root allowWrite")
}

// (wr-c) Re-seeding does not duplicate the worktree-root allowWrite entry.
func TestSeedSettings_WorktreeRootAllowWriteIdempotent(t *testing.T) {
	wt := t.TempDir()
	require.NoError(t, claudenotebook.SeedSettings(wt, nil, nil, []string{"/abs/nbA"}))
	require.NoError(t, claudenotebook.SeedSettings(wt, nil, nil, []string{"/abs/nbA"}))

	allow := allowWriteDirs(t, readSettings(t, wt))
	canonWt := worktreeAllowWrite(t, wt)
	n := 0
	for _, p := range allow {
		if p == canonWt {
			n++
		}
	}
	assert.Equal(t, 1, n, "worktree root must appear exactly once after re-seed")
}

// (wr-d) A pre-existing user allowWrite entry survives the worktree-root add.
func TestSeedSettings_WorktreeRootAllowWritePreservesUserEntries(t *testing.T) {
	wt := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(wt, ".claude"), 0o755))
	userWrite := "/Users/dev/manual-write-dir"
	seed := map[string]any{
		"sandbox": map[string]any{"filesystem": map[string]any{"allowWrite": []any{userWrite}}},
	}
	data, err := json.MarshalIndent(seed, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(wt, settingsRel), data, 0o644))

	require.NoError(t, claudenotebook.SeedSettings(wt, nil, nil, []string{"/abs/nbA"}))

	allow := allowWriteDirs(t, readSettings(t, wt))
	assert.Contains(t, allow, userWrite, "pre-existing user allowWrite entry survives")
	assert.Contains(t, allow, worktreeAllowWrite(t, wt), "worktree root added alongside")
}

// (2a) Edit rules ride the notebook-dir gate, NOT the settings gate:
// GROVE_SEED_CLAUDE_SETTINGS=off with dirs present still emits Edit rules.
func TestSeedSettings_EditRulesRideDirGate_SettingsGateOff(t *testing.T) {
	wt := t.TempDir()
	t.Setenv("GROVE_SEED_CLAUDE_SETTINGS", "off")

	require.NoError(t, claudenotebook.SeedSettings(wt, nil, nil, []string{"/abs/nbA"}))

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

	require.NoError(t, claudenotebook.SeedSettings(wt, nil, nil, []string{"/abs/nbA"}))

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
	require.NoError(t, claudenotebook.SeedSettings(wt, nil, cfg, nil))

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
		require.NoError(t, claudenotebook.SeedSettings(wt, nil, cfg, nil))
		assert.NotContains(t, allowRules(t, readSettings(t, wt)), "Bash(grove:*)")
	})

	t.Run("false flag", func(t *testing.T) {
		wt := t.TempDir()
		cfg := &claudenotebook.ClaudeConfig{
			AllowGroveTools: boolPtr(false),
			Permissions:     claudenotebook.ClaudePermissions{Allow: []string{"Bash(git:*)"}},
		}
		require.NoError(t, claudenotebook.SeedSettings(wt, nil, cfg, nil))
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

	require.NoError(t, claudenotebook.SeedSettings(wt, nil, cfg, notebookDirs))

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

// TestSeedSettings_SocketKnobsWritten asserts the three sandbox.network socket /
// local-bind knobs are seeded: allowUnixSockets unions with any pre-existing
// array, while allowAllUnixSockets and allowLocalBinding are written (and
// OVERWRITE an existing value, like the other sandbox bools).
func TestSeedSettings_SocketKnobsWritten(t *testing.T) {
	wt := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(wt, ".claude"), 0o755))

	// Pre-seed an existing settings file with an allowUnixSockets entry (to be
	// unioned) and allowLocalBinding=false (to be overwritten to true).
	seed := map[string]any{
		"sandbox": map[string]any{
			"network": map[string]any{
				"allowUnixSockets":  []any{"/run/existing.sock"},
				"allowLocalBinding": false,
			},
		},
	}
	data, err := json.MarshalIndent(seed, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(wt, settingsRel), data, 0o644))

	cfg := &claudenotebook.ClaudeConfig{
		Sandbox: claudenotebook.ClaudeSandbox{
			Network: claudenotebook.ClaudeSandboxNetwork{
				AllowUnixSockets:    []string{"/run/existing.sock", "/run/tuimux.sock"},
				AllowAllUnixSockets: boolPtr(true),
				AllowLocalBinding:   boolPtr(true),
			},
		},
	}

	require.NoError(t, claudenotebook.SeedSettings(wt, nil, cfg, nil))

	root := readSettings(t, wt)

	// allowUnixSockets unions (no duplicate of the pre-existing entry).
	sockets := stringSliceAt(t, root, "sandbox", "network", "allowUnixSockets")
	assert.ElementsMatch(t, []string{"/run/existing.sock", "/run/tuimux.sock"}, sockets)

	// allowAllUnixSockets written true; allowLocalBinding overwritten false->true.
	assert.True(t, boolAt(t, root, "sandbox", "network", "allowAllUnixSockets"))
	assert.True(t, boolAt(t, root, "sandbox", "network", "allowLocalBinding"))

	assertNoTmpLeak(t, wt)
}

// TestSeedSettings_SocketKnobsAbsentNoOp confirms a config that sets no socket
// knobs leaves the three keys unwritten (the empty/gate-off no-op holds).
func TestSeedSettings_SocketKnobsAbsentNoOp(t *testing.T) {
	wt := t.TempDir()

	// A non-empty config (forces a write) that carries no socket knobs.
	cfg := &claudenotebook.ClaudeConfig{
		Sandbox: claudenotebook.ClaudeSandbox{Enabled: boolPtr(true)},
	}
	require.NoError(t, claudenotebook.SeedSettings(wt, nil, cfg, nil))

	root := readSettings(t, wt)
	assert.Nil(t, optionalBoolAt(root, "sandbox", "network", "allowAllUnixSockets"))
	assert.Nil(t, optionalBoolAt(root, "sandbox", "network", "allowLocalBinding"))
	// allowUnixSockets must be absent too.
	if sb, ok := root["sandbox"].(map[string]any); ok {
		if net, ok := sb["network"].(map[string]any); ok {
			_, present := net["allowUnixSockets"]
			assert.False(t, present, "allowUnixSockets should be absent")
		}
	}
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

			require.NoError(t, claudenotebook.SeedSettings(wt, nil, cfg, nil))

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

	require.NoError(t, claudenotebook.SeedSettings(wt, nil, cfg, nil))

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

			require.NoError(t, claudenotebook.SeedSettings(wt, nil, cfg, notebookDirs))

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
	require.NoError(t, claudenotebook.SeedSettings(wt, nil, nil, nil))

	_, err := os.Stat(filepath.Join(wt, settingsRel))
	assert.True(t, os.IsNotExist(err), "no file should be created with empty config and no dirs")
}

// TestSeedSettings_NilConfigWithDirs tests that passing nil config with dirs
// works (delegates to notebook-only seeding).
func TestSeedSettings_NilConfigWithDirs(t *testing.T) {
	wt := t.TempDir()

	require.NoError(t, claudenotebook.SeedSettings(wt, nil, nil, []string{"/Users/dev/notebooks/core"}))

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
	err := claudenotebook.SeedSettings(wt, nil, cfg, nil)
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
	require.NoError(t, claudenotebook.SeedSettings(wt, nil, cfg, nil))

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

	require.NoError(t, claudenotebook.SeedSettings(wt, nil, cfg, nil))

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
	require.NoError(t, claudenotebook.SeedSettings(wt, nil, cfg, nil))

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
	require.NoError(t, claudenotebook.SeedSettings(wt, nil, cfg, nil))

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
	require.NoError(t, claudenotebook.SeedSettings(wt, nil, cfg, nil))

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
	require.NoError(t, claudenotebook.SeedSettings(wt, nil, cfg, nil))

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
	require.NoError(t, claudenotebook.SeedSettings(wt, nil, cfg, nil))

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

// ============================================================================
// Config self-protection (protectConfig) tests
// ============================================================================

// protectionExpectation mirrors the seeder's protectedConfigPaths/denyRulesForPath
// so tests can assert the exact entries without importing unexported helpers.
func protectionExpectation(t *testing.T, wt string, repos []string) (denyWrite []string, denyRules []string) {
	t.Helper()
	type pp struct {
		path  string
		isDir bool
	}
	var pths []pp
	if cfgDir := paths.ConfigDir(); cfgDir != "" {
		canon, cerr := pathutil.CanonicalPath(cfgDir)
		if cerr != nil {
			canon = cfgDir
		}
		pths = append(pths, pp{canon, true})
	}
	canonWt := wt
	if canon, err := pathutil.CanonicalPath(wt); err == nil {
		canonWt = canon
	}
	names := []string{"grove.toml", "grove.yml", "grove.yaml"}
	for _, n := range names {
		pths = append(pths, pp{filepath.Join(canonWt, n), false})
	}
	for _, r := range repos {
		for _, n := range names {
			pths = append(pths, pp{filepath.Join(canonWt, r, n), false})
		}
	}
	for _, p := range pths {
		denyWrite = append(denyWrite, p.path)
		anchored := "//" + strings.TrimPrefix(filepath.ToSlash(p.path), "/")
		if p.isDir {
			anchored += "/**"
		}
		denyRules = append(denyRules,
			"Edit("+anchored+")",
			"Write("+anchored+")",
			"MultiEdit("+anchored+")",
		)
	}
	return denyWrite, denyRules
}

func permDeny(t *testing.T, root map[string]any) []string {
	return stringSliceAt(t, root, "permissions", "deny")
}

func denyWriteAt(t *testing.T, root map[string]any) []string {
	return stringSliceAt(t, root, "sandbox", "filesystem", "denyWrite")
}

// TestSeedSettings_ProtectConfig_WritesBothLayers: protectConfig=true emits the
// sandbox denyWrite paths AND the permissions.deny Edit/Write/MultiEdit rules
// for the worktree + member-repo config files and the global config dir.
func TestSeedSettings_ProtectConfig_WritesBothLayers(t *testing.T) {
	wt := t.TempDir()
	repos := []string{"svc-a", "svc-b"}
	cfg := &claudenotebook.ClaudeConfig{
		ProtectConfig: boolPtr(true),
		Sandbox:       claudenotebook.ClaudeSandbox{Enabled: boolPtr(true)},
	}
	require.NoError(t, claudenotebook.SeedSettings(wt, repos, cfg, nil))

	root := readSettings(t, wt)
	wantDenyWrite, wantRules := protectionExpectation(t, wt, repos)
	gotDenyWrite := denyWriteAt(t, root)
	gotDeny := permDeny(t, root)
	for _, p := range wantDenyWrite {
		assert.Contains(t, gotDenyWrite, p, "denyWrite must contain protected path")
	}
	for _, r := range wantRules {
		assert.Contains(t, gotDeny, r, "permissions.deny must contain protection rule")
	}
	// The global config dir must use the /** subtree glob; the file paths must NOT.
	var sawDirGlob, sawFileExact bool
	for _, r := range gotDeny {
		if strings.HasSuffix(r, "/.config/grove/**)") {
			sawDirGlob = true
		}
		if strings.HasSuffix(r, "/grove.toml)") {
			sawFileExact = true
		}
	}
	assert.True(t, sawDirGlob, "global config dir rule must use /** subtree glob")
	assert.True(t, sawFileExact, "grove.toml file rule must match the file exactly (no glob)")
	assertNoTmpLeak(t, wt)
}

// TestSeedSettings_ProtectConfig_NeverDeniesToolInvocation: the protection rules
// target file PATHS only — never a Bash(<tool>:*) invocation.
func TestSeedSettings_ProtectConfig_NeverDeniesToolInvocation(t *testing.T) {
	wt := t.TempDir()
	cfg := &claudenotebook.ClaudeConfig{ProtectConfig: boolPtr(true), Sandbox: claudenotebook.ClaudeSandbox{Enabled: boolPtr(true)}}
	require.NoError(t, claudenotebook.SeedSettings(wt, []string{"core"}, cfg, nil))

	for _, r := range permDeny(t, readSettings(t, wt)) {
		assert.NotContains(t, r, "Bash(", "protection must never deny a Bash tool invocation")
	}
}

// TestSeedSettings_ProtectConfig_OnlySignalStillSeeds: a config whose ONLY
// signal is protectConfig=true (IsEmpty()==true) must still write protection.
// This is the regression guard for the ShouldSeed gate; without it the upstream
// IsEmpty short-circuits would drop the config before the seeder runs.
func TestSeedSettings_ProtectConfig_OnlySignalStillSeeds(t *testing.T) {
	wt := t.TempDir()
	cfg := &claudenotebook.ClaudeConfig{ProtectConfig: boolPtr(true)} // nothing else set
	require.True(t, cfg.IsEmpty(), "precondition: protectConfig-only config is IsEmpty")
	require.True(t, cfg.ShouldSeed(), "ShouldSeed must be true for a protectConfig-only config")

	require.NoError(t, claudenotebook.SeedSettings(wt, nil, cfg, nil))
	root := readSettings(t, wt)
	assert.NotEmpty(t, denyWriteAt(t, root), "denyWrite must be written for protectConfig-only config")
	assert.NotEmpty(t, permDeny(t, root), "permissions.deny must be written for protectConfig-only config")
}

// TestSeedSettings_ProtectConfig_NoClobberUserDeny: a pre-existing user deny rule
// and user denyWrite path survive the protection seed, and the user's own
// configured Deny/DenyWrite arrays are unioned in.
func TestSeedSettings_ProtectConfig_NoClobberUserDeny(t *testing.T) {
	wt := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(wt, ".claude"), 0o755))
	seed := map[string]any{
		"permissions": map[string]any{"deny": []any{"Read(/etc/passwd)"}},
		"sandbox":     map[string]any{"filesystem": map[string]any{"denyWrite": []any{"/var/log"}}},
	}
	data, err := json.MarshalIndent(seed, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(wt, settingsRel), data, 0o644))

	cfg := &claudenotebook.ClaudeConfig{
		ProtectConfig: boolPtr(true),
		Sandbox:       claudenotebook.ClaudeSandbox{Enabled: boolPtr(true), Filesystem: claudenotebook.ClaudeSandboxFilesystem{DenyWrite: []string{"/srv/data"}}},
		Permissions:   claudenotebook.ClaudePermissions{Deny: []string{"Read(/secret)"}},
	}
	require.NoError(t, claudenotebook.SeedSettings(wt, nil, cfg, nil))

	root := readSettings(t, wt)
	assert.Contains(t, permDeny(t, root), "Read(/etc/passwd)", "pre-existing user deny survives")
	assert.Contains(t, permDeny(t, root), "Read(/secret)", "configured user deny unioned in")
	assert.Contains(t, denyWriteAt(t, root), "/var/log", "pre-existing user denyWrite survives")
	assert.Contains(t, denyWriteAt(t, root), "/srv/data", "configured user denyWrite unioned in")
}

// TestSeedSettings_ProtectConfig_FalseStripsOnlyGroveOwned: toggling false strips
// grove-owned entries but leaves user-authored deny rules intact (reversibility).
func TestSeedSettings_ProtectConfig_FalseStripsOnlyGroveOwned(t *testing.T) {
	wt := t.TempDir()
	// First lock it.
	on := &claudenotebook.ClaudeConfig{ProtectConfig: boolPtr(true), Sandbox: claudenotebook.ClaudeSandbox{Enabled: boolPtr(true)}}
	require.NoError(t, claudenotebook.SeedSettings(wt, []string{"svc-a"}, on, nil))
	// Inject user entries alongside the grove-owned ones.
	root := readSettings(t, wt)
	root["permissions"].(map[string]any)["deny"] = append(root["permissions"].(map[string]any)["deny"].([]any), "Read(/etc/passwd)")
	root["sandbox"].(map[string]any)["filesystem"].(map[string]any)["denyWrite"] = append(
		root["sandbox"].(map[string]any)["filesystem"].(map[string]any)["denyWrite"].([]any), "/var/log")
	data, err := json.MarshalIndent(root, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(wt, settingsRel), data, 0o644))

	// Now toggle off.
	off := &claudenotebook.ClaudeConfig{ProtectConfig: boolPtr(false), Sandbox: claudenotebook.ClaudeSandbox{Enabled: boolPtr(true)}}
	require.NoError(t, claudenotebook.SeedSettings(wt, []string{"svc-a"}, off, nil))

	root = readSettings(t, wt)
	_, wantRules := protectionExpectation(t, wt, []string{"svc-a"})
	gotDeny := permDeny(t, root)
	for _, r := range wantRules {
		assert.NotContains(t, gotDeny, r, "grove-owned deny rule must be stripped on false")
	}
	assert.Contains(t, gotDeny, "Read(/etc/passwd)", "user deny rule must survive the strip")
	assert.Contains(t, denyWriteAt(t, root), "/var/log", "user denyWrite must survive the strip")
}

// TestSeedSettings_ProtectConfig_UnlockEnvStrips: GROVE_UNLOCK_CONFIG=1 makes a
// protectConfig=true seed behave as off for this launch (entries stripped).
func TestSeedSettings_ProtectConfig_UnlockEnvStrips(t *testing.T) {
	wt := t.TempDir()
	// Lock first without the env var.
	on := &claudenotebook.ClaudeConfig{ProtectConfig: boolPtr(true), Sandbox: claudenotebook.ClaudeSandbox{Enabled: boolPtr(true)}}
	require.NoError(t, claudenotebook.SeedSettings(wt, nil, on, nil))
	require.NotEmpty(t, permDeny(t, readSettings(t, wt)))

	// Re-seed with the unlock env: should strip the grove-owned rules.
	t.Setenv("GROVE_UNLOCK_CONFIG", "1")
	require.NoError(t, claudenotebook.SeedSettings(wt, nil, on, nil))
	root := readSettings(t, wt)
	_, wantRules := protectionExpectation(t, wt, nil)
	for _, r := range wantRules {
		assert.NotContains(t, permDeny(t, root), r, "unlock env must strip grove-owned deny rules")
	}
}

// TestSeedSettings_ProtectConfig_UnsetNoOp: an unset ProtectConfig writes no
// protection entries and performs no strip.
func TestSeedSettings_ProtectConfig_UnsetNoOp(t *testing.T) {
	wt := t.TempDir()
	cfg := &claudenotebook.ClaudeConfig{Permissions: claudenotebook.ClaudePermissions{Allow: []string{"Bash(git:*)"}}}
	require.NoError(t, claudenotebook.SeedSettings(wt, nil, cfg, nil))
	root := readSettings(t, wt)
	_, denyExists := root["permissions"].(map[string]any)["deny"]
	assert.False(t, denyExists, "unset protectConfig must not create a permissions.deny array")
}

// TestSeedSettings_ProtectConfig_SandboxDisabledStillWritesPermDeny: with
// protectConfig=true but sandbox disabled, the best-effort permissions.deny seam
// is still written (the warning path must not skip the write).
func TestSeedSettings_ProtectConfig_SandboxDisabledStillWritesPermDeny(t *testing.T) {
	wt := t.TempDir()
	cfg := &claudenotebook.ClaudeConfig{ProtectConfig: boolPtr(true)} // sandbox.enabled nil
	require.NoError(t, claudenotebook.SeedSettings(wt, nil, cfg, nil))
	root := readSettings(t, wt)
	assert.NotEmpty(t, permDeny(t, root), "permissions.deny written even when sandbox disabled")
	assert.NotEmpty(t, denyWriteAt(t, root), "denyWrite written even when sandbox disabled")
}

// TestClaudeConfig_ShouldSeed covers the lone-flag widening contract.
func TestClaudeConfig_ShouldSeed(t *testing.T) {
	assert.False(t, (*claudenotebook.ClaudeConfig)(nil).ShouldSeed(), "nil config should not seed")
	assert.False(t, (&claudenotebook.ClaudeConfig{}).ShouldSeed(), "empty config should not seed")
	assert.True(t, (&claudenotebook.ClaudeConfig{ProtectConfig: boolPtr(true)}).ShouldSeed(), "protectConfig=true seeds")
	assert.True(t, (&claudenotebook.ClaudeConfig{ProtectConfig: boolPtr(false)}).ShouldSeed(), "protectConfig=false seeds (to strip)")
	assert.True(t, (&claudenotebook.ClaudeConfig{AllowGroveTools: boolPtr(true)}).ShouldSeed(), "allowGroveTools=true seeds")
	assert.False(t, (&claudenotebook.ClaudeConfig{AllowGroveTools: boolPtr(false)}).ShouldSeed(), "allowGroveTools=false does not seed alone")
}

// ============================================================================
// autoMode classifier + useAutoModeDuringPlan seeding (Part B)
// ============================================================================

// sandboxHome points HOME and GROVE_HOME at throwaway dirs so no test can touch
// the developer's real ~/.claude.json or ~/.config/grove.
func sandboxHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GROVE_HOME", filepath.Join(home, "grove"))
}

// TestSeedSettings_AutoModeWritten confirms the four autoMode sections are
// written additively under the EXACT snake_case JSON keys Claude requires
// (soft_deny/hard_deny), useAutoModeDuringPlan is written at the top level, and
// pre-existing user entries (including "$defaults") are preserved.
func TestSeedSettings_AutoModeWritten(t *testing.T) {
	sandboxHome(t)
	wt := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(wt, ".claude"), 0o755))

	// Pre-seed a user autoMode.allow with "$defaults" to prove additive-preserve.
	seed := map[string]any{
		"autoMode": map[string]any{
			"allow": []any{"$defaults"},
		},
	}
	data, err := json.MarshalIndent(seed, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(wt, settingsRel), data, 0o644))

	cfg := &claudenotebook.ClaudeConfig{
		AutoMode: &claudenotebook.ClaudeAutoMode{
			Allow:       []string{"Bash(git:*)"},
			SoftDeny:    []string{"Read(secret)"},
			Environment: []string{"CI=1"},
			HardDeny:    []string{"Bash(rm:*)"},
		},
		UseAutoModeDuringPlan: boolPtr(true),
	}
	require.NoError(t, claudenotebook.SeedSettings(wt, nil, cfg, nil))

	root := readSettings(t, wt)

	// $defaults preserved and grove's rule appended (additive union).
	assert.Equal(t, []string{"$defaults", "Bash(git:*)"}, stringSliceAt(t, root, "autoMode", "allow"))
	assert.Equal(t, []string{"Read(secret)"}, stringSliceAt(t, root, "autoMode", "soft_deny"))
	assert.Equal(t, []string{"CI=1"}, stringSliceAt(t, root, "autoMode", "environment"))
	assert.Equal(t, []string{"Bash(rm:*)"}, stringSliceAt(t, root, "autoMode", "hard_deny"))
	assert.True(t, boolAt(t, root, "useAutoModeDuringPlan"))

	// The written keys MUST be exactly snake_case — assert against the raw bytes
	// so a camelCase regression (softDeny/hardDeny) is caught.
	raw, err := os.ReadFile(filepath.Join(wt, settingsRel))
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"soft_deny"`)
	assert.Contains(t, string(raw), `"hard_deny"`)
	assert.NotContains(t, string(raw), `"softDeny"`)
	assert.NotContains(t, string(raw), `"hardDeny"`)

	assertNoTmpLeak(t, wt)
}

// TestSeedSettings_AutoModeGateOff confirms a lone autoMode/useAutoModeDuringPlan
// config writes nothing when the settings gate is off.
func TestSeedSettings_AutoModeGateOff(t *testing.T) {
	sandboxHome(t)
	wt := t.TempDir()
	t.Setenv("GROVE_SEED_CLAUDE_SETTINGS", "off")

	cfg := &claudenotebook.ClaudeConfig{
		AutoMode:              &claudenotebook.ClaudeAutoMode{HardDeny: []string{"Bash(rm:*)"}},
		UseAutoModeDuringPlan: boolPtr(true),
	}
	require.NoError(t, claudenotebook.SeedSettings(wt, nil, cfg, nil))

	_, err := os.Stat(filepath.Join(wt, settingsRel))
	assert.True(t, os.IsNotExist(err), "gate off + lone autoMode must not write the file")
}

// TestSeedSettings_AutoModeEmptyNoOp confirms an autoMode present-but-all-empty
// carries no signal (treated as unset): no file is created.
func TestSeedSettings_AutoModeEmptyNoOp(t *testing.T) {
	sandboxHome(t)
	wt := t.TempDir()

	cfg := &claudenotebook.ClaudeConfig{AutoMode: &claudenotebook.ClaudeAutoMode{}}
	require.NoError(t, claudenotebook.SeedSettings(wt, nil, cfg, nil))

	_, err := os.Stat(filepath.Join(wt, settingsRel))
	assert.True(t, os.IsNotExist(err), "all-empty autoMode must not write a file")
}

// ============================================================================
// sandbox escape-hatch lock seeding (Part C)
// ============================================================================

// TestSeedSettings_AllowUnsandboxedCommandsWritten is the security-critical case:
// allowUnsandboxedCommands=false must land as literal JSON false, and
// excludedCommands must be written under the exact key, additively preserving a
// pre-existing user entry.
func TestSeedSettings_AllowUnsandboxedCommandsWritten(t *testing.T) {
	sandboxHome(t)
	wt := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(wt, ".claude"), 0o755))

	// Pre-seed a user excludedCommands entry to prove additive-preserve.
	seed := map[string]any{
		"sandbox": map[string]any{
			"excludedCommands": []any{"git"},
		},
	}
	data, err := json.MarshalIndent(seed, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(wt, settingsRel), data, 0o644))

	cfg := &claudenotebook.ClaudeConfig{
		Sandbox: claudenotebook.ClaudeSandbox{
			AllowUnsandboxedCommands: boolPtr(false),
			ExcludedCommands:         []string{"docker", "flow"},
		},
	}
	require.NoError(t, claudenotebook.SeedSettings(wt, nil, cfg, nil))

	root := readSettings(t, wt)

	// The lock: explicit false must survive all the way to literal JSON false.
	got := optionalBoolAt(root, "sandbox", "allowUnsandboxedCommands")
	require.NotNil(t, got, "expected sandbox.allowUnsandboxedCommands to be written")
	assert.False(t, *got, "the lock must land as literal false")

	// excludedCommands: additive union preserving the pre-existing "git".
	assert.ElementsMatch(t, []string{"git", "docker", "flow"},
		stringSliceAt(t, root, "sandbox", "excludedCommands"))

	// Exact key names against raw bytes.
	raw, err := os.ReadFile(filepath.Join(wt, settingsRel))
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"allowUnsandboxedCommands": false`)
	assert.Contains(t, string(raw), `"excludedCommands"`)

	assertNoTmpLeak(t, wt)
}

// TestSeedSettings_AllowUnsandboxedGateOff confirms a lone
// allowUnsandboxedCommands=false profile writes nothing when the gate is off
// (it rides the same GROVE_SEED_CLAUDE_SETTINGS gate).
func TestSeedSettings_AllowUnsandboxedGateOff(t *testing.T) {
	sandboxHome(t)
	wt := t.TempDir()
	t.Setenv("GROVE_SEED_CLAUDE_SETTINGS", "off")

	cfg := &claudenotebook.ClaudeConfig{
		Sandbox: claudenotebook.ClaudeSandbox{AllowUnsandboxedCommands: boolPtr(false)},
	}
	require.NoError(t, claudenotebook.SeedSettings(wt, nil, cfg, nil))

	_, err := os.Stat(filepath.Join(wt, settingsRel))
	assert.True(t, os.IsNotExist(err), "gate off + lone lock must not write the file")
}

// TestSeedSettings_AutoModeAndLockMalformedNoOp confirms a malformed settings
// file is left untouched (the seeder returns an error and never clobbers it)
// even when the config carries the new autoMode/sandbox-lock fields.
func TestSeedSettings_AutoModeAndLockMalformedNoOp(t *testing.T) {
	sandboxHome(t)
	wt := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(wt, ".claude"), 0o755))

	bad := []byte("{ this is not json")
	require.NoError(t, os.WriteFile(filepath.Join(wt, settingsRel), bad, 0o644))

	cfg := &claudenotebook.ClaudeConfig{
		AutoMode: &claudenotebook.ClaudeAutoMode{HardDeny: []string{"Bash(rm:*)"}},
		Sandbox:  claudenotebook.ClaudeSandbox{AllowUnsandboxedCommands: boolPtr(false)},
	}
	err := claudenotebook.SeedSettings(wt, nil, cfg, nil)
	require.Error(t, err, "malformed JSON must surface an error")

	after, rerr := os.ReadFile(filepath.Join(wt, settingsRel))
	require.NoError(t, rerr)
	assert.Equal(t, bad, after, "malformed file must be left untouched")
	assertNoTmpLeak(t, wt)
}
