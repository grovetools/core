package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadFromBytes(t *testing.T) {
	yaml := `
version: "1.0"
services:
  api:
    build: .
    ports: ["8080"]
    environment:
      - NODE_ENV=development
  db:
    image: postgres:15
profiles:
  default:
    services: [api, db]
settings:
  network_name: test-network
`

	config, err := LoadFromBytes([]byte(yaml))
	require.NoError(t, err)

	assert.Equal(t, "1.0", config.Version)
	assert.Len(t, config.Services, 2)
	assert.Equal(t, ".", config.Services["api"].Build)
	assert.Equal(t, "postgres:15", config.Services["db"].Image)
	assert.Equal(t, "test-network", config.Settings.NetworkName)
}

func TestEnvironmentVariableExpansion(t *testing.T) {
	os.Setenv("TEST_BRANCH", "feature-123")
	os.Setenv("TEST_PORT", "9000")
	defer os.Unsetenv("TEST_BRANCH")
	defer os.Unsetenv("TEST_PORT")

	yaml := `
version: "1.0"
services:
  api:
    build: .
    ports: ["${TEST_PORT:-8080}"]
    environment:
      - DATABASE_URL=postgres://localhost/app_${TEST_BRANCH}
      - DEFAULT_VAR=${UNDEFINED_VAR:-default_value}
`

	config, err := LoadFromBytes([]byte(yaml))
	require.NoError(t, err)

	assert.Equal(t, "9000", config.Services["api"].Ports[0])
	assert.Contains(t, config.Services["api"].Environment[0], "app_feature-123")
	assert.Contains(t, config.Services["api"].Environment[1], "default_value")
}

func TestFindConfigFile(t *testing.T) {
	// Create test directory structure
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "project")
	subDir := filepath.Join(projectDir, "src", "services")
	require.NoError(t, os.MkdirAll(subDir, 0755))

	// Create config file in project root
	configPath := filepath.Join(projectDir, "grove.yml")
	require.NoError(t, os.WriteFile(configPath, []byte("version: '1.0'"), 0644))

	// Test finding from subdirectory
	found, err := FindConfigFile(subDir)
	assert.NoError(t, err)
	assert.Equal(t, configPath, found)

	// Test not finding
	emptyDir := t.TempDir()
	_, err = FindConfigFile(emptyDir)
	assert.Error(t, err)
}