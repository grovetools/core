package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeConfigs(t *testing.T) {
	base := &Config{
		Version: "1.0",
		Services: map[string]ServiceConfig{
			"api": {
				Build:       ".",
				Ports:       []string{"8080"},
				Environment: []string{"ENV=base"},
			},
			"db": {
				Image: "postgres:14",
			},
		},
		Settings: Settings{
			NetworkName:    "grove",
			DomainSuffix:   "localhost",
			DefaultProfile: "default",
		},
	}

	override := &Config{
		Services: map[string]ServiceConfig{
			"api": {
				Ports:       []string{"9090"},
				Environment: []string{"ENV=override", "NEW_VAR=value"},
			},
			"cache": {
				Image: "redis:7",
			},
		},
		Settings: Settings{
			DefaultProfile: "dev",
		},
	}

	merged := mergeConfigs(base, override)

	// Check merged values
	assert.Equal(t, "1.0", merged.Version)
	assert.Equal(t, ".", merged.Services["api"].Build)
	assert.Equal(t, []string{"9090"}, merged.Services["api"].Ports)
	assert.Contains(t, merged.Services["api"].Environment, "ENV=override")
	assert.Equal(t, "postgres:14", merged.Services["db"].Image)
	assert.Equal(t, "redis:7", merged.Services["cache"].Image)
	assert.Equal(t, "dev", merged.Settings.DefaultProfile)
	assert.Equal(t, "grove", merged.Settings.NetworkName)
}

func TestLoadWithOverrides(t *testing.T) {
	tmpDir := t.TempDir()

	// Create base config
	baseConfig := `
version: "1.0"
services:
  api:
    build: .
    ports: ["8080"]
settings:
  default_profile: production
`
	baseFile := filepath.Join(tmpDir, "grove.yml")
	require.NoError(t, os.WriteFile(baseFile, []byte(baseConfig), 0644))

	// Create override config
	overrideConfig := `
services:
  api:
    ports: ["9090"]
settings:
  default_profile: development
`
	overrideFile := filepath.Join(tmpDir, "grove.override.yml")
	require.NoError(t, os.WriteFile(overrideFile, []byte(overrideConfig), 0644))

	// Load with overrides
	config, err := LoadWithOverrides(baseFile)
	require.NoError(t, err)

	assert.Equal(t, []string{"9090"}, config.Services["api"].Ports)
	assert.Equal(t, "development", config.Settings.DefaultProfile)
}
