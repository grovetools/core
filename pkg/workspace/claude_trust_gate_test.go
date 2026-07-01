package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sandboxTrustHome isolates config + trust resolution from the developer's real
// environment: HOME/XDG point at a throwaway dir so config.LoadFrom's global
// layer resolves inside the sandbox and any ~/.claude.json write would land
// here (never the real file). See the repo memory note on HOME sandboxing.
func sandboxTrustHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	return home
}

// writeWorktreeClaudeConfig writes a grove.toml carrying a [claude] block into
// worktreePath and returns the path.
func writeWorktreeClaudeConfig(t *testing.T, worktreePath, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(worktreePath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(worktreePath, "grove.toml"), []byte(body), 0o644))
}

// TestWorktreeManagesTrust_EnabledWhenSet confirms a worktree whose [claude]
// block sets manageTrust=true resolves to ManagesTrust()==true.
func TestWorktreeManagesTrust_EnabledWhenSet(t *testing.T) {
	sandboxTrustHome(t)
	worktree := t.TempDir()
	writeWorktreeClaudeConfig(t, worktree, "[claude]\nmanageTrust = true\n")

	cfg := ResolveClaudeConfigForWorktree(worktree, nil)
	require.NotNil(t, cfg)
	assert.True(t, cfg.ManagesTrust(), "manageTrust=true should resolve enabled")
	assert.True(t, WorktreeManagesTrust(worktree, nil), "gate helper should report enabled")
}

// TestWorktreeManagesTrust_DisabledWhenUnset confirms the opt-in default: a
// worktree with no [claude] manageTrust key resolves to disabled.
func TestWorktreeManagesTrust_DisabledWhenUnset(t *testing.T) {
	sandboxTrustHome(t)
	worktree := t.TempDir()
	// A [claude] block with unrelated settings but no manageTrust key.
	writeWorktreeClaudeConfig(t, worktree, "[claude.permissions]\nallow = [\"Bash(git:*)\"]\n")

	assert.False(t, WorktreeManagesTrust(worktree, nil), "unset manageTrust defaults to disabled")
}

// TestWorktreeManagesTrust_DisabledWhenFalse confirms an explicit
// manageTrust=false resolves to disabled.
func TestWorktreeManagesTrust_DisabledWhenFalse(t *testing.T) {
	sandboxTrustHome(t)
	worktree := t.TempDir()
	writeWorktreeClaudeConfig(t, worktree, "[claude]\nmanageTrust = false\n")

	assert.False(t, WorktreeManagesTrust(worktree, nil), "explicit false is disabled")
}

// TestWorktreeManagesTrust_NoConfigDisabled confirms a worktree with no grove
// config at all resolves to disabled (nil profile => ManagesTrust()==false),
// never a panic.
func TestWorktreeManagesTrust_NoConfigDisabled(t *testing.T) {
	sandboxTrustHome(t)
	worktree := t.TempDir() // no grove.toml anywhere in the sandbox

	assert.False(t, WorktreeManagesTrust(worktree, nil), "no config degrades to disabled")
}
