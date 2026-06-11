package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncConfigTOMLTableShape(t *testing.T) {
	data := []byte(`
[notebooks.definitions.default]
root_dir = "/tmp/notebook"

[notebooks.definitions.default.sync]
server = "https://sync.example.com"
token = "static-token"
token_command = "echo cmd-token"

[[notebooks.definitions.default.sync.workspaces]]
name = "grovetools"
mode = "full"
pull = true
excludes = [".obsidian/", "*.lock"]

[[notebooks.definitions.default.sync.workspaces]]
name = "personal"
mode = "plans-only"
`)

	cfg, err := unmarshalConfig("grove.toml", data)
	require.NoError(t, err)
	require.NotNil(t, cfg.Notebooks)
	nb := cfg.Notebooks.Definitions["default"]
	require.NotNil(t, nb)
	require.NotNil(t, nb.Sync)

	assert.Equal(t, "https://sync.example.com", nb.Sync.Server)
	assert.Equal(t, "static-token", nb.Sync.Token)
	assert.Equal(t, "echo cmd-token", nb.Sync.TokenCommand)
	require.Len(t, nb.Sync.Workspaces, 2)
	assert.Equal(t, "grovetools", nb.Sync.Workspaces[0].Name)
	assert.Equal(t, SyncModeFull, nb.Sync.Workspaces[0].Mode)
	assert.True(t, nb.Sync.Workspaces[0].Pull)
	assert.Equal(t, []string{".obsidian/", "*.lock"}, nb.Sync.Workspaces[0].Excludes)
	assert.Equal(t, SyncModePlansOnly, nb.Sync.Workspaces[1].Mode)
	assert.False(t, nb.Sync.Workspaces[1].Pull)
	assert.Empty(t, nb.Sync.Providers)
}

func TestSyncConfigTOMLLegacyListShape(t *testing.T) {
	data := []byte(`
[notebooks.definitions.default]
root_dir = "/tmp/notebook"

[[notebooks.definitions.default.sync]]
provider = "github"
issues_type = "issues"
prs_type = "prs"
`)

	cfg, err := unmarshalConfig("grove.toml", data)
	require.NoError(t, err)
	nb := cfg.Notebooks.Definitions["default"]
	require.NotNil(t, nb)
	require.NotNil(t, nb.Sync)

	require.Len(t, nb.Sync.Providers, 1)
	assert.Equal(t, "github", nb.Sync.Providers[0].Provider)
	assert.Equal(t, "issues", nb.Sync.Providers[0].IssuesType)
	assert.Equal(t, "prs", nb.Sync.Providers[0].PRsType)
	assert.Empty(t, nb.Sync.Server)
}

func TestSyncConfigYAMLTableShape(t *testing.T) {
	data := []byte(`
notebooks:
  definitions:
    default:
      root_dir: /tmp/notebook
      sync:
        server: https://sync.example.com
        token_command: echo tok
        workspaces:
          - name: grovetools
            mode: search-only
`)

	cfg, err := unmarshalConfig("grove.yml", data)
	require.NoError(t, err)
	nb := cfg.Notebooks.Definitions["default"]
	require.NotNil(t, nb)
	require.NotNil(t, nb.Sync)

	assert.Equal(t, "https://sync.example.com", nb.Sync.Server)
	assert.Equal(t, "echo tok", nb.Sync.TokenCommand)
	require.Len(t, nb.Sync.Workspaces, 1)
	assert.Equal(t, SyncModeSearchOnly, nb.Sync.Workspaces[0].Mode)
}

func TestSyncConfigYAMLLegacyListShape(t *testing.T) {
	data := []byte(`
notebooks:
  definitions:
    default:
      root_dir: /tmp/notebook
      sync:
        - provider: github
          issues_type: issues
`)

	cfg, err := unmarshalConfig("grove.yml", data)
	require.NoError(t, err)
	nb := cfg.Notebooks.Definitions["default"]
	require.NotNil(t, nb)
	require.NotNil(t, nb.Sync)

	require.Len(t, nb.Sync.Providers, 1)
	assert.Equal(t, "github", nb.Sync.Providers[0].Provider)
	assert.Equal(t, "issues", nb.Sync.Providers[0].IssuesType)
}

func TestSyncConfigAbsent(t *testing.T) {
	data := []byte(`
[notebooks.definitions.default]
root_dir = "/tmp/notebook"
`)

	cfg, err := unmarshalConfig("grove.toml", data)
	require.NoError(t, err)
	nb := cfg.Notebooks.Definitions["default"]
	require.NotNil(t, nb)
	assert.Nil(t, nb.Sync, "no sync key must mean sync stays dark")
}

func TestSyncConfigSchemaValidation(t *testing.T) {
	// Both shapes must pass full schema validation (Load path).
	t.Run("typed table shape", func(t *testing.T) {
		data := []byte(`
[notebooks.definitions.default]
root_dir = "/tmp/notebook"

[notebooks.definitions.default.sync]
server = "https://sync.example.com"
`)
		_, err := LoadFromTOMLBytes(data)
		require.NoError(t, err)
	})

	t.Run("legacy list shape", func(t *testing.T) {
		data := []byte(`
[notebooks.definitions.default]
root_dir = "/tmp/notebook"

[[notebooks.definitions.default.sync]]
provider = "github"
`)
		_, err := LoadFromTOMLBytes(data)
		require.NoError(t, err)
	})
}

func TestLoadSyncConfigFrom(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "sync.toml")

	t.Run("missing file is dark, not an error", func(t *testing.T) {
		cfg, err := LoadSyncConfigFrom(path)
		require.NoError(t, err)
		assert.Nil(t, cfg)
	})

	t.Run("full config parses", func(t *testing.T) {
		require.NoError(t, os.WriteFile(path, []byte(`
server = "https://sync.example.com"
token_command = "echo from-command"

[[workspaces]]
name = "grovetools"
mode = "full"
pull = true

[[workspaces]]
name = "personal"
mode = "plans-only"
excludes = ["journal/"]
`), 0o644))

		cfg, err := LoadSyncConfigFrom(path)
		require.NoError(t, err)
		require.NotNil(t, cfg)
		assert.Equal(t, "https://sync.example.com", cfg.Server)
		assert.Equal(t, "echo from-command", cfg.TokenCommand)
		require.Len(t, cfg.Workspaces, 2)
		assert.True(t, cfg.Workspaces[0].Pull)
		assert.Equal(t, []string{"journal/"}, cfg.Workspaces[1].Excludes)
	})

	t.Run("invalid mode rejected", func(t *testing.T) {
		require.NoError(t, os.WriteFile(path, []byte(`
server = "https://sync.example.com"

[[workspaces]]
name = "grovetools"
mode = "everything"
`), 0o644))

		_, err := LoadSyncConfigFrom(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid mode")
	})

	t.Run("env var expansion", func(t *testing.T) {
		t.Setenv("TEST_SYNC_SERVER", "https://env.example.com")
		require.NoError(t, os.WriteFile(path, []byte(`server = "${TEST_SYNC_SERVER}"`), 0o644))

		cfg, err := LoadSyncConfigFrom(path)
		require.NoError(t, err)
		require.NotNil(t, cfg)
		assert.Equal(t, "https://env.example.com", cfg.Server)
	})
}

func TestLoadSyncConfigUsesGroveHome(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("GROVE_HOME", tmpHome)

	configDir := filepath.Join(tmpHome, "config", "grove")
	require.NoError(t, os.MkdirAll(configDir, 0o755))

	assert.Equal(t, filepath.Join(configDir, "sync.toml"), SyncConfigPath())

	// Absent: dark.
	cfg, err := LoadSyncConfig()
	require.NoError(t, err)
	assert.Nil(t, cfg)

	// Present: parsed.
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "sync.toml"), []byte(`server = "https://sync.example.com"`), 0o644))
	cfg, err = LoadSyncConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "https://sync.example.com", cfg.Server)
}

func TestSyncTomlNotLoadedAsConfigFragment(t *testing.T) {
	// sync.toml lives in the grove config dir, where *.toml files are merged
	// as config fragments. It must be skipped: it has its own schema.
	tmpHome := t.TempDir()
	t.Setenv("GROVE_HOME", tmpHome)
	ResetLoadCache()
	t.Cleanup(ResetLoadCache)

	configDir := filepath.Join(tmpHome, "config", "grove")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "grove.toml"), []byte(`name = "from-grove-toml"`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "sync.toml"), []byte(`
name = "leaked-from-sync-toml"
server = "https://sync.example.com"
`), 0o644))

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	cfg, err := LoadFromWithLogger(t.TempDir(), logger)
	require.NoError(t, err)
	assert.Equal(t, "from-grove-toml", cfg.Name)
	assert.NotContains(t, cfg.Extensions, "server")
}

func TestSyncConfigResolveToken(t *testing.T) {
	t.Run("env var wins", func(t *testing.T) {
		t.Setenv(SyncTokenEnvVar, "env-token")
		s := &SyncConfig{Token: "static-token", TokenCommand: "echo cmd-token"}
		token, err := s.ResolveToken()
		require.NoError(t, err)
		assert.Equal(t, "env-token", token)
	})

	t.Run("token_command beats static", func(t *testing.T) {
		t.Setenv(SyncTokenEnvVar, "")
		s := &SyncConfig{Token: "static-token", TokenCommand: "echo cmd-token"}
		token, err := s.ResolveToken()
		require.NoError(t, err)
		assert.Equal(t, "cmd-token", token)
	})

	t.Run("static token fallback", func(t *testing.T) {
		t.Setenv(SyncTokenEnvVar, "")
		s := &SyncConfig{Token: "static-token"}
		token, err := s.ResolveToken()
		require.NoError(t, err)
		assert.Equal(t, "static-token", token)
	})

	t.Run("nothing configured resolves empty", func(t *testing.T) {
		t.Setenv(SyncTokenEnvVar, "")
		s := &SyncConfig{}
		token, err := s.ResolveToken()
		require.NoError(t, err)
		assert.Empty(t, token)
	})

	t.Run("failing token_command errors", func(t *testing.T) {
		t.Setenv(SyncTokenEnvVar, "")
		s := &SyncConfig{TokenCommand: "exit 1"}
		_, err := s.ResolveToken()
		require.Error(t, err)
	})

	t.Run("empty token_command output errors", func(t *testing.T) {
		t.Setenv(SyncTokenEnvVar, "")
		s := &SyncConfig{TokenCommand: "true"}
		_, err := s.ResolveToken()
		require.Error(t, err)
	})
}

func TestSyncConfigValidate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		s := &SyncConfig{
			Workspaces: []SyncWorkspace{
				{Name: "a"},
				{Name: "b", Mode: SyncModeFull},
				{Name: "c", Mode: SyncModeSearchOnly},
			},
			Providers: []SyncProviderConfig{{Provider: "github"}},
		}
		require.NoError(t, s.Validate())
	})

	t.Run("missing workspace name", func(t *testing.T) {
		s := &SyncConfig{Workspaces: []SyncWorkspace{{Mode: SyncModeFull}}}
		require.Error(t, s.Validate())
	})

	t.Run("missing provider name", func(t *testing.T) {
		s := &SyncConfig{Providers: []SyncProviderConfig{{IssuesType: "issues"}}}
		require.Error(t, s.Validate())
	})
}
