package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHookManager_InstallHooks(t *testing.T) {
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git", "hooks")
	require.NoError(t, os.MkdirAll(gitDir, 0755))

	manager := NewHookManager("grove")

	// Install hooks
	err := manager.InstallHooks(context.Background(), tmpDir)
	require.NoError(t, err)

	// Check hooks exist
	hooks := []string{"post-checkout", "post-merge", "pre-commit"}
	for _, hook := range hooks {
		hookPath := filepath.Join(gitDir, hook)
		assert.FileExists(t, hookPath)

		// Check it's executable
		info, err := os.Stat(hookPath)
		require.NoError(t, err)
		assert.True(t, info.Mode()&0100 != 0, "hook should be executable")

		// Check content
		content, err := os.ReadFile(hookPath)
		require.NoError(t, err)
		assert.Contains(t, string(content), "Grove git hook")
		assert.Contains(t, string(content), hook)
	}
}

func TestHookManager_UninstallHooks(t *testing.T) {
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git", "hooks")
	require.NoError(t, os.MkdirAll(gitDir, 0755))

	manager := NewHookManager("grove")

	// Install then uninstall
	require.NoError(t, manager.InstallHooks(context.Background(), tmpDir))
	require.NoError(t, manager.UninstallHooks(context.Background(), tmpDir))

	// Check hooks removed
	hooks := []string{"post-checkout", "post-merge", "pre-commit"}
	for _, hook := range hooks {
		hookPath := filepath.Join(gitDir, hook)
		assert.NoFileExists(t, hookPath)
	}
}

func TestHookManager_PreserveExistingHooks(t *testing.T) {
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git", "hooks")
	require.NoError(t, os.MkdirAll(gitDir, 0755))

	// Create existing hook
	existingHook := filepath.Join(gitDir, "post-checkout")
	existingContent := "github.com/mattsolo1/grove-core/bin/sh\necho 'existing hook'"
	require.NoError(t, os.WriteFile(existingHook, []byte(existingContent), 0755))

	manager := NewHookManager("grove")

	// Install hooks
	err := manager.InstallHooks(context.Background(), tmpDir)
	require.NoError(t, err)

	// Check backup created
	backupPath := existingHook + ".pre-grove"
	assert.FileExists(t, backupPath)

	backupContent, err := os.ReadFile(backupPath)
	require.NoError(t, err)
	assert.Equal(t, existingContent, string(backupContent))
}