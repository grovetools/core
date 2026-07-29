package workspace

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/pkg/exectrust"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sandboxTrustHome isolates config + trust resolution from the developer's real
// environment: HOME/XDG point at a throwaway dir so config.LoadFrom's global
// layer resolves inside the sandbox and any ~/.claude.json write would land
// here (never the real file). See the repo memory note on HOME sandboxing.
// GROVE_EXEC_TRUST_FILE is redirected too, so these tests neither read nor
// write the developer's real exec-provenance trust decisions.
func sandboxTrustHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv(exectrust.EnvStorePath, filepath.Join(home, "exec-trust.json"))
	config.ResetLoadCache()
	t.Cleanup(config.ResetLoadCache)
	return home
}

// writeWorktreeClaudeConfig writes a grove.toml carrying a [claude] block into
// worktreePath and returns the path.
func writeWorktreeClaudeConfig(t *testing.T, worktreePath, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(worktreePath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(worktreePath, "grove.toml"), []byte(body), 0o644))
}

// trustWorktreeConfig performs what `grove config trust --yes` does for the
// worktree's own grove.toml: read the digest the gate reports for it, and
// record it. [claude] is capability-granting config (core/config/execgate.go),
// so a workspace-layer block is quarantined until the user does this.
func trustWorktreeConfig(t *testing.T, worktreePath string) {
	t.Helper()
	cfgPath := filepath.Join(worktreePath, "grove.toml")
	loaded, err := config.LoadFrom(worktreePath)
	require.NoError(t, err)
	require.NotNil(t, loaded.ExecGate, "the gate must report the workspace config")

	var digest string
	for _, f := range loaded.ExecGate.Files {
		if f.Path == cfgPath {
			digest = f.Digest
		}
	}
	require.NotEmpty(t, digest, "the gate must report a digest for %s", cfgPath)

	store := exectrust.Load()
	store.Trust(cfgPath, digest, time.Now())
	require.NoError(t, store.Save())
	config.ResetLoadCache()
}

// TestWorktreeManagesTrust_EnabledWhenSet confirms a worktree whose [claude]
// block sets manageTrust=true resolves to ManagesTrust()==true — once the user
// has trusted the config file that declares it.
func TestWorktreeManagesTrust_EnabledWhenSet(t *testing.T) {
	sandboxTrustHome(t)
	worktree := t.TempDir()
	writeWorktreeClaudeConfig(t, worktree, "[claude]\nmanageTrust = true\n")
	trustWorktreeConfig(t, worktree)

	cfg := ResolveClaudeConfigForWorktree(worktree, nil)
	require.NotNil(t, cfg)
	assert.True(t, cfg.ManagesTrust(), "manageTrust=true should resolve enabled")
	assert.True(t, WorktreeManagesTrust(worktree, nil), "gate helper should report enabled")
}

// TestWorktreeManagesTrust_GatedWhenWorkspaceUntrusted is the F1 property at
// the folder-trust seam: an untrusted workspace layer declaring manageTrust=true
// must NOT arm grove's writes to ~/.claude.json. The exec-provenance gate is the
// single trust source for this path — the resolver reads config.LoadFrom, so
// there is no second store to disagree with.
func TestWorktreeManagesTrust_GatedWhenWorkspaceUntrusted(t *testing.T) {
	sandboxTrustHome(t)
	worktree := t.TempDir()
	writeWorktreeClaudeConfig(t, worktree, "[claude]\nmanageTrust = true\n")

	assert.False(t, WorktreeManagesTrust(worktree, nil),
		"an untrusted workspace must not arm folder-trust management")
}

// TestWorktreeManagesTrust_DisabledWhenUnset confirms the opt-in default: a
// worktree with no [claude] manageTrust key resolves to disabled.
func TestWorktreeManagesTrust_DisabledWhenUnset(t *testing.T) {
	sandboxTrustHome(t)
	worktree := t.TempDir()
	// A [claude] block with unrelated settings but no manageTrust key.
	writeWorktreeClaudeConfig(t, worktree, "[claude.permissions]\nallow = [\"Bash(git:*)\"]\n")
	trustWorktreeConfig(t, worktree)

	assert.False(t, WorktreeManagesTrust(worktree, nil), "unset manageTrust defaults to disabled")
}

// TestWorktreeManagesTrust_DisabledWhenFalse confirms an explicit
// manageTrust=false resolves to disabled.
func TestWorktreeManagesTrust_DisabledWhenFalse(t *testing.T) {
	sandboxTrustHome(t)
	worktree := t.TempDir()
	writeWorktreeClaudeConfig(t, worktree, "[claude]\nmanageTrust = false\n")
	trustWorktreeConfig(t, worktree)

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
