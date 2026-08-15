package claudetrust_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grovetools/core/pkg/claudetrust"
)

func TestIsTrusted_MissingFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := claudetrust.IsTrusted("/Users/dev/proj/feat")
	require.NoError(t, err)
	assert.False(t, got)
}

func TestIsTrusted_PresentTrue(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	target := "/Users/dev/proj/feat"
	writeConfig(t, home, map[string]any{
		"projects": map[string]any{
			target: map[string]any{"hasTrustDialogAccepted": true},
		},
	})

	got, err := claudetrust.IsTrusted(target)
	require.NoError(t, err)
	assert.True(t, got)
}

func TestIsTrusted_PresentFalse(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	target := "/Users/dev/proj/feat"
	writeConfig(t, home, map[string]any{
		"projects": map[string]any{
			target: map[string]any{"hasTrustDialogAccepted": false},
		},
	})

	got, err := claudetrust.IsTrusted(target)
	require.NoError(t, err)
	assert.False(t, got)
}

func TestIsTrusted_MalformedFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := filepath.Join(home, ".claude.json")
	garbage := []byte("{ this is not valid json ]]")
	require.NoError(t, os.WriteFile(configPath, garbage, 0o600))

	got, err := claudetrust.IsTrusted("/Users/dev/proj/feat")
	require.Error(t, err)
	assert.False(t, got)
	assert.Contains(t, err.Error(), "parse "+configPath)

	after, readErr := os.ReadFile(configPath)
	require.NoError(t, readErr)
	assert.Equal(t, garbage, after, "checker must not modify malformed input")
}

func TestIsTrusted_RequiresExactPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	trustedPath := "/Users/dev/proj/feat"
	writeConfig(t, home, map[string]any{
		"projects": map[string]any{
			trustedPath: map[string]any{"hasTrustDialogAccepted": true},
		},
	})

	for _, path := range []string{
		trustedPath + "/child",
		trustedPath + "/.",
		"/Users/dev/proj/../proj/feat",
	} {
		t.Run(path, func(t *testing.T) {
			got, err := claudetrust.IsTrusted(path)
			require.NoError(t, err)
			assert.False(t, got)
		})
	}
}
