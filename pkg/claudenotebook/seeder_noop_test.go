package claudenotebook_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/grovetools/core/pkg/claudenotebook"
)

// TestSeedSettingsChanged_SecondPassNoOp: seeding the same inputs twice must
// write on the first pass (changed=true) and skip the write entirely on the
// second (changed=false, identical bytes, untouched mtime — no tmp+rename).
func TestSeedSettingsChanged_SecondPassNoOp(t *testing.T) {
	wt := t.TempDir()
	nb := filepath.Join(t.TempDir(), "nb")

	changed, err := claudenotebook.SeedSettingsChanged(wt, nil, nil, []string{nb})
	require.NoError(t, err)
	require.True(t, changed, "first seed must write")

	settingsPath := filepath.Join(wt, ".claude", "settings.local.json")
	before, err := os.ReadFile(settingsPath)
	require.NoError(t, err)
	infoBefore, err := os.Stat(settingsPath)
	require.NoError(t, err)

	// Ensure a rewrite would be observable as an mtime bump.
	time.Sleep(20 * time.Millisecond)

	changed, err = claudenotebook.SeedSettingsChanged(wt, nil, nil, []string{nb})
	require.NoError(t, err)
	require.False(t, changed, "second seed of identical inputs must be a no-op")

	after, err := os.ReadFile(settingsPath)
	require.NoError(t, err)
	require.Equal(t, string(before), string(after))

	infoAfter, err := os.Stat(settingsPath)
	require.NoError(t, err)
	require.True(t, infoBefore.ModTime().Equal(infoAfter.ModTime()),
		"no-op pass must not rewrite the file (mtime changed)")

	// No orphaned tmp file from a skipped write.
	_, err = os.Stat(settingsPath + ".tmp")
	require.True(t, os.IsNotExist(err))
}

// TestSeedSettingsChanged_NothingToSeed: no config and no dirs is changed=false.
func TestSeedSettingsChanged_NothingToSeed(t *testing.T) {
	wt := t.TempDir()
	changed, err := claudenotebook.SeedSettingsChanged(wt, nil, nil, nil)
	require.NoError(t, err)
	require.False(t, changed)
}
