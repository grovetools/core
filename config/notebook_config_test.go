package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveNotebookContext(t *testing.T) {
	tmpDir := t.TempDir()

	groveDir := filepath.Join(tmpDir, "code")
	nestedGroveDir := filepath.Join(tmpDir, "code", "nested")
	notebookDir := filepath.Join(tmpDir, "notebooks", "nb")
	customNbDir := filepath.Join(tmpDir, "notebooks", "custom")
	require.NoError(t, os.MkdirAll(groveDir, 0755))
	require.NoError(t, os.MkdirAll(nestedGroveDir, 0755))
	require.NoError(t, os.MkdirAll(notebookDir, 0755))
	require.NoError(t, os.MkdirAll(customNbDir, 0755))

	enabled := true
	disabled := false

	cfg := &Config{
		Groves: map[string]GroveSourceConfig{
			"code": {
				Path:    groveDir,
				Enabled: &enabled,
			},
			"nested": {
				Path:     nestedGroveDir,
				Enabled:  &enabled,
				Notebook: "custom-nb",
			},
			"disabled": {
				Path:    filepath.Join(tmpDir, "disabled"),
				Enabled: &disabled,
			},
		},
		Notebooks: &NotebooksConfig{
			Rules: &NotebookRules{
				Default: "nb",
			},
			Definitions: map[string]*Notebook{
				"nb": {
					RootDir: notebookDir,
				},
				"custom-nb": {
					RootDir: customNbDir,
				},
			},
		},
	}

	t.Run("project in grove resolves correctly", func(t *testing.T) {
		ctx := resolveNotebookContext(filepath.Join(groveDir, "my-app"), cfg)
		require.NotNil(t, ctx)
		assert.Equal(t, "my-app", ctx.workspaceName)
		assert.Equal(t, notebookDir, ctx.notebookRootDir)
	})

	t.Run("project in nested grove uses longest match", func(t *testing.T) {
		ctx := resolveNotebookContext(filepath.Join(nestedGroveDir, "my-app"), cfg)
		require.NotNil(t, ctx)
		assert.Equal(t, "my-app", ctx.workspaceName)
		assert.Equal(t, customNbDir, ctx.notebookRootDir)
	})

	t.Run("project in disabled grove returns nil", func(t *testing.T) {
		ctx := resolveNotebookContext(filepath.Join(tmpDir, "disabled", "my-app"), cfg)
		assert.Nil(t, ctx)
	})

	t.Run("project not in any grove returns nil", func(t *testing.T) {
		ctx := resolveNotebookContext(filepath.Join(tmpDir, "elsewhere", "my-app"), cfg)
		assert.Nil(t, ctx)
	})

	t.Run("grove root itself returns nil", func(t *testing.T) {
		ctx := resolveNotebookContext(groveDir, cfg)
		assert.Nil(t, ctx, "grove root has workspaceName '.' and should be rejected")
	})

	t.Run("nil config returns nil", func(t *testing.T) {
		ctx := resolveNotebookContext(filepath.Join(groveDir, "app"), nil)
		assert.Nil(t, ctx)
	})

	t.Run("no notebook definitions returns nil", func(t *testing.T) {
		cfgNoNb := &Config{
			Groves: map[string]GroveSourceConfig{
				"code": {Path: groveDir, Enabled: &enabled},
			},
		}
		ctx := resolveNotebookContext(filepath.Join(groveDir, "app"), cfgNoNb)
		assert.Nil(t, ctx)
	})

	t.Run("fallback to default notebook name nb", func(t *testing.T) {
		cfgNoDefault := &Config{
			Groves: map[string]GroveSourceConfig{
				"code": {Path: groveDir, Enabled: &enabled},
			},
			Notebooks: &NotebooksConfig{
				Definitions: map[string]*Notebook{
					"nb": {RootDir: notebookDir},
				},
			},
		}
		ctx := resolveNotebookContext(filepath.Join(groveDir, "app"), cfgNoDefault)
		require.NotNil(t, ctx)
		assert.Equal(t, notebookDir, ctx.notebookRootDir)
	})
}

func TestFindNotebookConfigPath(t *testing.T) {
	tmpDir := t.TempDir()

	groveDir := filepath.Join(tmpDir, "code")
	notebookDir := filepath.Join(tmpDir, "notebooks", "nb")
	require.NoError(t, os.MkdirAll(groveDir, 0755))

	enabled := true
	cfg := &Config{
		Groves: map[string]GroveSourceConfig{
			"code": {Path: groveDir, Enabled: &enabled},
		},
		Notebooks: &NotebooksConfig{
			Rules:       &NotebookRules{Default: "nb"},
			Definitions: map[string]*Notebook{"nb": {RootDir: notebookDir}},
		},
	}

	t.Run("finds grove.toml", func(t *testing.T) {
		wsDir := filepath.Join(notebookDir, "workspaces", "my-app")
		require.NoError(t, os.MkdirAll(wsDir, 0755))
		configPath := filepath.Join(wsDir, "grove.toml")
		require.NoError(t, os.WriteFile(configPath, []byte("name = \"my-app\"\n"), 0644))
		defer os.RemoveAll(filepath.Join(notebookDir, "workspaces", "my-app"))

		result := findNotebookConfigPath(filepath.Join(groveDir, "my-app"), cfg)
		assert.Equal(t, configPath, result)
	})

	t.Run("finds grove.yml", func(t *testing.T) {
		wsDir := filepath.Join(notebookDir, "workspaces", "yml-app")
		require.NoError(t, os.MkdirAll(wsDir, 0755))
		configPath := filepath.Join(wsDir, "grove.yml")
		require.NoError(t, os.WriteFile(configPath, []byte("name: yml-app\n"), 0644))
		defer os.RemoveAll(filepath.Join(notebookDir, "workspaces", "yml-app"))

		result := findNotebookConfigPath(filepath.Join(groveDir, "yml-app"), cfg)
		assert.Equal(t, configPath, result)
	})

	t.Run("prefers grove.toml over grove.yml", func(t *testing.T) {
		wsDir := filepath.Join(notebookDir, "workspaces", "both-app")
		require.NoError(t, os.MkdirAll(wsDir, 0755))
		tomlPath := filepath.Join(wsDir, "grove.toml")
		require.NoError(t, os.WriteFile(tomlPath, []byte("name = \"both-app\"\n"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(wsDir, "grove.yml"), []byte("name: both-app\n"), 0644))
		defer os.RemoveAll(filepath.Join(notebookDir, "workspaces", "both-app"))

		result := findNotebookConfigPath(filepath.Join(groveDir, "both-app"), cfg)
		assert.Equal(t, tomlPath, result)
	})

	t.Run("returns empty when no config file exists", func(t *testing.T) {
		result := findNotebookConfigPath(filepath.Join(groveDir, "nonexistent"), cfg)
		assert.Empty(t, result)
	})

	t.Run("returns empty for nil config", func(t *testing.T) {
		result := findNotebookConfigPath(filepath.Join(groveDir, "app"), nil)
		assert.Empty(t, result)
	})
}

func TestLoadFromWithLogger_NotebookConfig(t *testing.T) {
	tmpDir, _ := filepath.EvalSymlinks(t.TempDir())

	fakeHome := filepath.Join(tmpDir, "home")
	fakeConfigDir := filepath.Join(fakeHome, ".config", "grove")
	groveDir := filepath.Join(tmpDir, "code")
	notebookDir := filepath.Join(tmpDir, "notebooks", "nb")
	projectDir := filepath.Join(groveDir, "my-project")

	require.NoError(t, os.MkdirAll(fakeConfigDir, 0755))
	require.NoError(t, os.MkdirAll(projectDir, 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(notebookDir, "workspaces", "my-project"), 0755))

	// Initialize git repo so getGitRoot works
	initGitRepo(t, projectDir)

	origHome := os.Getenv("HOME")
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	defer func() {
		os.Setenv("HOME", origHome)
		os.Setenv("XDG_CONFIG_HOME", origXDG)
	}()
	os.Setenv("HOME", fakeHome)
	os.Unsetenv("XDG_CONFIG_HOME")

	globalConfig := `
version: "1.0"
groves:
  code:
    path: ` + groveDir + `
    enabled: true
notebooks:
  rules:
    default: nb
  definitions:
    nb:
      root_dir: ` + notebookDir + `

monitoring:
  interval: 10
`
	require.NoError(t, os.WriteFile(filepath.Join(fakeConfigDir, "grove.yml"), []byte(globalConfig), 0644))

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	t.Run("notebook config merged when no local config", func(t *testing.T) {
		nbConfig := `
name: from-notebook
monitoring:
  interval: 42
`
		require.NoError(t, os.WriteFile(
			filepath.Join(notebookDir, "workspaces", "my-project", "grove.yml"),
			[]byte(nbConfig), 0644,
		))
		defer os.Remove(filepath.Join(notebookDir, "workspaces", "my-project", "grove.yml"))

		cfg, err := LoadFromWithLogger(projectDir, logger)
		require.NoError(t, err)
		assert.Equal(t, "from-notebook", cfg.Name)

		type MonitoringConfig struct {
			Interval int `yaml:"interval"`
		}
		var mon MonitoringConfig
		require.NoError(t, cfg.UnmarshalExtension("monitoring", &mon))
		assert.Equal(t, 42, mon.Interval, "notebook config should override global")
	})

	t.Run("local config overrides notebook config", func(t *testing.T) {
		ResetLoadCache()
		nbConfig := `
name: from-notebook
monitoring:
  interval: 42
`
		localConfig := `
name: from-local
`
		require.NoError(t, os.WriteFile(
			filepath.Join(notebookDir, "workspaces", "my-project", "grove.yml"),
			[]byte(nbConfig), 0644,
		))
		require.NoError(t, os.WriteFile(
			filepath.Join(projectDir, "grove.yml"),
			[]byte(localConfig), 0644,
		))
		defer os.Remove(filepath.Join(notebookDir, "workspaces", "my-project", "grove.yml"))
		defer os.Remove(filepath.Join(projectDir, "grove.yml"))

		cfg, err := LoadFromWithLogger(projectDir, logger)
		require.NoError(t, err)
		assert.Equal(t, "from-local", cfg.Name, "local config should override notebook")

		type MonitoringConfig struct {
			Interval int `yaml:"interval"`
		}
		var mon MonitoringConfig
		require.NoError(t, cfg.UnmarshalExtension("monitoring", &mon))
		assert.Equal(t, 42, mon.Interval, "notebook monitoring should still be merged")
	})
}

func TestLoadLayered_NotebookConfig(t *testing.T) {
	tmpDir, _ := filepath.EvalSymlinks(t.TempDir())

	fakeHome := filepath.Join(tmpDir, "home")
	fakeConfigDir := filepath.Join(fakeHome, ".config", "grove")
	groveDir := filepath.Join(tmpDir, "code")
	notebookDir := filepath.Join(tmpDir, "notebooks", "nb")
	projectDir := filepath.Join(groveDir, "my-project")

	require.NoError(t, os.MkdirAll(fakeConfigDir, 0755))
	require.NoError(t, os.MkdirAll(projectDir, 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(notebookDir, "workspaces", "my-project"), 0755))

	initGitRepo(t, projectDir)

	origHome := os.Getenv("HOME")
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	defer func() {
		os.Setenv("HOME", origHome)
		os.Setenv("XDG_CONFIG_HOME", origXDG)
	}()
	os.Setenv("HOME", fakeHome)
	os.Unsetenv("XDG_CONFIG_HOME")

	globalConfig := `
version: "1.0"
groves:
  code:
    path: ` + groveDir + `
    enabled: true
notebooks:
  rules:
    default: nb
  definitions:
    nb:
      root_dir: ` + notebookDir + `
`
	require.NoError(t, os.WriteFile(filepath.Join(fakeConfigDir, "grove.yml"), []byte(globalConfig), 0644))

	nbConfigPath := filepath.Join(notebookDir, "workspaces", "my-project", "grove.yml")
	nbConfig := `
name: from-notebook
`
	require.NoError(t, os.WriteFile(nbConfigPath, []byte(nbConfig), 0644))

	layered, err := LoadLayered(projectDir)
	require.NoError(t, err)

	assert.NotNil(t, layered.ProjectNotebook, "ProjectNotebook layer should be populated")
	if layered.ProjectNotebook != nil {
		assert.Equal(t, "from-notebook", layered.ProjectNotebook.Name)
	}
	assert.Equal(t, nbConfigPath, layered.FilePaths[SourceProjectNotebook])
}

func TestLoadLayered_EcosystemNotebookLookup(t *testing.T) {
	tmpDir, _ := filepath.EvalSymlinks(t.TempDir())

	fakeHome := filepath.Join(tmpDir, "home")
	fakeConfigDir := filepath.Join(fakeHome, ".config", "grove")
	groveDir := filepath.Join(tmpDir, "code")
	notebookDir := filepath.Join(tmpDir, "notebooks", "nb")
	ecoDir := filepath.Join(groveDir, "my-eco")
	projectDir := filepath.Join(ecoDir, "sub-project")

	require.NoError(t, os.MkdirAll(fakeConfigDir, 0755))
	require.NoError(t, os.MkdirAll(ecoDir, 0755))
	require.NoError(t, os.MkdirAll(projectDir, 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(notebookDir, "workspaces", "my-eco"), 0755))

	initGitRepo(t, projectDir)

	origHome := os.Getenv("HOME")
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	defer func() {
		os.Setenv("HOME", origHome)
		os.Setenv("XDG_CONFIG_HOME", origXDG)
	}()
	os.Setenv("HOME", fakeHome)
	os.Unsetenv("XDG_CONFIG_HOME")

	// Global config defines grove but NOT notebook — notebook comes from ecosystem
	globalConfig := `
version: "1.0"
groves:
  code:
    path: ` + groveDir + `
    enabled: true
`
	require.NoError(t, os.WriteFile(filepath.Join(fakeConfigDir, "grove.yml"), []byte(globalConfig), 0644))

	// Ecosystem config defines notebook
	ecoConfig := `
version: "1.0"
workspaces:
  - "` + projectDir + `"
notebooks:
  rules:
    default: nb
  definitions:
    nb:
      root_dir: ` + notebookDir + `
`
	require.NoError(t, os.WriteFile(filepath.Join(ecoDir, "grove.yml"), []byte(ecoConfig), 0644))

	nbConfigPath := filepath.Join(notebookDir, "workspaces", "my-eco", "grove.yml")
	nbConfig := `
name: from-eco-notebook
`
	require.NoError(t, os.WriteFile(nbConfigPath, []byte(nbConfig), 0644))

	layered, err := LoadLayered(ecoDir)
	require.NoError(t, err)

	assert.NotNil(t, layered.ProjectNotebook, "ProjectNotebook should be found via ecosystem-defined notebook")
	if layered.ProjectNotebook != nil {
		assert.Equal(t, "from-eco-notebook", layered.ProjectNotebook.Name)
	}
}

// initGitRepo creates a real git repository at the given path using git init.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("git", "init", dir)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git init failed: %s", string(out))
}
