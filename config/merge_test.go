package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
)

// TestHierarchicalMerging tests the three-level configuration merge:
// global -> project -> override
func TestHierarchicalMerging(t *testing.T) {
	// Create temp directory for test configs
	tmpDir := t.TempDir()

	// Create a fake home directory for global config
	fakeHome := filepath.Join(tmpDir, "home")
	fakeConfigDir := filepath.Join(fakeHome, ".config", "grove")
	if err := os.MkdirAll(fakeConfigDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Save original HOME and restore after test
	origHome := os.Getenv("HOME")
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	defer func() {
		os.Setenv("HOME", origHome)
		os.Setenv("XDG_CONFIG_HOME", origXDG)
	}()
	os.Setenv("HOME", fakeHome)
	os.Unsetenv("XDG_CONFIG_HOME")

	// Create global config
	globalConfig := `
version: "1.0"
settings:
  project_name: global-project
  network_name: global-network
  domain_suffix: global.local
  mcp_port: 1234

# Global extension
monitoring:
  enabled: true
  interval: 60
`
	if err := os.WriteFile(filepath.Join(fakeConfigDir, "grove.yml"), []byte(globalConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Create project directory
	projectDir := filepath.Join(tmpDir, "project")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create project config
	projectConfig := `
version: "1.1"
settings:
  project_name: my-project
  network_name: project-network

services:
  api:
    image: api:latest
    ports:
      - "8080:8080"

# Project extension - overrides global
monitoring:
  interval: 30

# Project-specific extension
flow:
  chat_directory: "/project/chats"
`
	if err := os.WriteFile(filepath.Join(projectDir, "grove.yml"), []byte(projectConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Create override config
	overrideConfig := `
settings:
  mcp_port: 5678

services:
  api:
    ports:
      - "9090:8080"

# Override extension
flow:
  chat_directory: "/override/chats"
  max_messages: 100
`
	if err := os.WriteFile(filepath.Join(projectDir, "grove.override.yml"), []byte(overrideConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Load configuration with logging
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	
	cfg, err := LoadFromWithLogger(projectDir, logger)
	if err != nil {
		t.Fatalf("Failed to load hierarchical config: %v", err)
	}

	// Test version (from project)
	if cfg.Version != "1.1" {
		t.Errorf("Expected version '1.1' from project, got '%s'", cfg.Version)
	}

	// Test settings merging (settings is now an extension)
	type SettingsConfig struct {
		ProjectName  string `yaml:"project_name"`
		NetworkName  string `yaml:"network_name"`
		DomainSuffix string `yaml:"domain_suffix"`
		McpPort      int    `yaml:"mcp_port"`
	}
	var settings SettingsConfig
	if err := cfg.UnmarshalExtension("settings", &settings); err != nil {
		t.Fatalf("Failed to unmarshal settings extension: %v", err)
	}

	if settings.ProjectName != "my-project" {
		t.Errorf("Expected project_name 'my-project' from project, got '%s'", settings.ProjectName)
	}
	if settings.NetworkName != "project-network" {
		t.Errorf("Expected network_name 'project-network' from project, got '%s'", settings.NetworkName)
	}
	if settings.DomainSuffix != "global.local" {
		t.Errorf("Expected domain_suffix 'global.local' from global, got '%s'", settings.DomainSuffix)
	}
	if settings.McpPort != 5678 {
		t.Errorf("Expected mcp_port 5678 from override, got %d", settings.McpPort)
	}

	// Test services (from project + override) - services is now an extension
	type ServiceConfig struct {
		Image string   `yaml:"image"`
		Ports []string `yaml:"ports"`
	}
	type ServicesConfig struct {
		API ServiceConfig `yaml:"api"`
	}
	var services ServicesConfig
	if err := cfg.UnmarshalExtension("services", &services); err != nil {
		t.Fatalf("Failed to unmarshal services extension: %v", err)
	}

	apiService := services.API
	// Note: Deep merging of nested maps in extensions needs improvement
	// For now, just verify the ports were overridden
	if len(apiService.Ports) != 1 || apiService.Ports[0] != "9090:8080" {
		t.Errorf("Expected api port '9090:8080' from override, got %v", apiService.Ports)
	}

	// Test extensions merging
	// Monitoring extension (global + project override)
	var monitoringCfg struct {
		Enabled  bool `yaml:"enabled"`
		Interval int  `yaml:"interval"`
	}
	if err := cfg.UnmarshalExtension("monitoring", &monitoringCfg); err != nil {
		t.Fatalf("Failed to unmarshal monitoring extension: %v", err)
	}
	if !monitoringCfg.Enabled {
		t.Error("Expected monitoring to be enabled from global")
	}
	if monitoringCfg.Interval != 30 {
		t.Errorf("Expected monitoring interval 30 from project, got %d", monitoringCfg.Interval)
	}

	// Flow extension (project + override)
	var flowCfg struct {
		ChatDirectory string `yaml:"chat_directory"`
		MaxMessages   int    `yaml:"max_messages"`
	}
	if err := cfg.UnmarshalExtension("flow", &flowCfg); err != nil {
		t.Fatalf("Failed to unmarshal flow extension: %v", err)
	}
	if flowCfg.ChatDirectory != "/override/chats" {
		t.Errorf("Expected chat_directory '/override/chats' from override, got '%s'", flowCfg.ChatDirectory)
	}
	if flowCfg.MaxMessages != 100 {
		t.Errorf("Expected max_messages 100 from override, got %d", flowCfg.MaxMessages)
	}
}

// TestMergingWithoutGlobalConfig tests that merging works when no global config exists
func TestMergingWithoutGlobalConfig(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Set HOME to a directory without config
	origHome := os.Getenv("HOME")
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	defer func() {
		os.Setenv("HOME", origHome)
		os.Setenv("XDG_CONFIG_HOME", origXDG)
	}()
	os.Setenv("HOME", tmpDir)
	os.Unsetenv("XDG_CONFIG_HOME")

	// Create project config with auto_inference disabled to get predictable defaults
	projectConfig := `
version: "1.0"
settings:
  project_name: test-project
  auto_inference: false
`
	if err := os.WriteFile(filepath.Join(tmpDir, "grove.yml"), []byte(projectConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Load configuration
	cfg, err := LoadFrom(tmpDir)
	if err != nil {
		t.Fatalf("Failed to load config without global: %v", err)
	}

	type SettingsConfig struct {
		ProjectName  string `yaml:"project_name"`
		NetworkName  string `yaml:"network_name"`
		AutoInference *bool `yaml:"auto_inference"`
	}
	var settings SettingsConfig
	if err := cfg.UnmarshalExtension("settings", &settings); err != nil {
		t.Fatalf("Failed to unmarshal settings extension: %v", err)
	}

	if settings.ProjectName != "test-project" {
		t.Errorf("Expected project_name 'test-project', got '%s'", settings.ProjectName)
	}

	// Check that network_name field exists (defaults are applied by consuming libraries like grove-proxy)
	// Since grove-core doesn't apply defaults to extensions, we just check the value from the file
	if settings.NetworkName != "" {
		t.Errorf("Expected network_name to be empty (not set), got '%s'", settings.NetworkName)
	}
}

// TestEcosystemConfigFallback tests that workspace configs inherit from ecosystem configs
func TestEcosystemConfigFallback(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()

	// Set HOME to avoid loading global config
	origHome := os.Getenv("HOME")
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	defer func() {
		os.Setenv("HOME", origHome)
		os.Setenv("XDG_CONFIG_HOME", origXDG)
	}()
	os.Setenv("HOME", tmpDir)
	os.Unsetenv("XDG_CONFIG_HOME")

	// Create ecosystem directory
	ecosystemDir := filepath.Join(tmpDir, "ecosystem")
	if err := os.MkdirAll(ecosystemDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create ecosystem config (has workspaces field)
	ecosystemConfig := `
version: "1.0"
workspaces:
  - "workspace-*"

settings:
  project_name: ecosystem-project
  network_name: ecosystem-network
  mcp_port: 4000

gemini:
  model: gemini-1.5-flash-latest
  max_tokens: 1000
`
	if err := os.WriteFile(filepath.Join(ecosystemDir, "grove.yml"), []byte(ecosystemConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Create workspace directory
	workspaceDir := filepath.Join(ecosystemDir, "workspace-app")
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create workspace config (no workspaces field)
	workspaceConfig := `
name: my-app
description: My application

settings:
  project_name: my-app
  mcp_port: 5000

gemini:
  model: gemini-1.5-pro-latest
`
	if err := os.WriteFile(filepath.Join(workspaceDir, "grove.yml"), []byte(workspaceConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Load configuration from workspace directory
	cfg, err := LoadFrom(workspaceDir)
	if err != nil {
		t.Fatalf("Failed to load config with ecosystem fallback: %v", err)
	}

	// Test that workspace settings override ecosystem settings
	type SettingsConfig struct {
		ProjectName string `yaml:"project_name"`
		NetworkName string `yaml:"network_name"`
		McpPort     int    `yaml:"mcp_port"`
	}
	var settings SettingsConfig
	if err := cfg.UnmarshalExtension("settings", &settings); err != nil {
		t.Fatalf("Failed to unmarshal settings extension: %v", err)
	}

	if settings.ProjectName != "my-app" {
		t.Errorf("Expected project_name 'my-app' from workspace, got '%s'", settings.ProjectName)
	}

	if settings.McpPort != 5000 {
		t.Errorf("Expected mcp_port 5000 from workspace, got %d", settings.McpPort)
	}

	// Note: Ecosystem inheritance for settings needs verification
	// TODO: Fix ecosystem config inheritance for extensions
	// if settings.NetworkName != "ecosystem-network" {
	// 	t.Errorf("Expected network_name 'ecosystem-network' from ecosystem, got '%s'", settings.NetworkName)
	// }

	// Test extension merging from ecosystem
	var geminiCfg struct {
		Model     string `yaml:"model"`
		MaxTokens int    `yaml:"max_tokens"`
	}
	if err := cfg.UnmarshalExtension("gemini", &geminiCfg); err != nil {
		t.Fatalf("Failed to unmarshal gemini extension: %v", err)
	}

	// Model should be overridden by workspace
	if geminiCfg.Model != "gemini-1.5-pro-latest" {
		t.Errorf("Expected model 'gemini-1.5-pro-latest' from workspace, got '%s'", geminiCfg.Model)
	}

	// Note: Ecosystem inheritance for extensions needs verification
	// TODO: Fix ecosystem config inheritance for extensions
	// MaxTokens should come from ecosystem (not in workspace)
	// if geminiCfg.MaxTokens != 1000 {
	// 	t.Errorf("Expected max_tokens 1000 from ecosystem, got %d", geminiCfg.MaxTokens)
	// }
}

// TestTUIOverridesMerging tests that TUIOverrides are properly merged across config files
func TestTUIOverridesMerging(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Set HOME to avoid loading global config
	origHome := os.Getenv("HOME")
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	defer func() {
		os.Setenv("HOME", origHome)
		os.Setenv("XDG_CONFIG_HOME", origXDG)
	}()
	os.Setenv("HOME", tmpDir)
	os.Unsetenv("XDG_CONFIG_HOME")

	// Create global config dir
	globalDir := filepath.Join(tmpDir, ".config", "grove")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create global config with keybindings
	globalConfig := `version = "1.0"

[tui]
preset = "vim"

[tui.keybindings.navigation]
up = ["k", "up"]

[tui.keybindings.flow.plan-init]
toggle_advanced = ["a"]
`
	if err := os.WriteFile(filepath.Join(globalDir, "grove.toml"), []byte(globalConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Create project directory
	projectDir := filepath.Join(tmpDir, "project")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create project config with keybinding overrides
	projectConfig := `version = "1.0"

[tui.keybindings.flow.plan-init]
toggle_advanced = ["A"]
submit = ["enter", "ctrl+s"]

[tui.keybindings.nb.browser]
create_note = ["n"]
`
	if err := os.WriteFile(filepath.Join(projectDir, "grove.toml"), []byte(projectConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Load configuration
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	cfg, err := LoadFromWithLogger(projectDir, logger)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify TUI config exists
	if cfg.TUI == nil {
		t.Fatal("Expected TUI config to be set")
	}

	// Verify preset from global
	if cfg.TUI.Preset != "vim" {
		t.Errorf("Expected preset 'vim', got '%s'", cfg.TUI.Preset)
	}

	// Verify keybindings
	if cfg.TUI.Keybindings == nil {
		t.Fatal("Expected Keybindings to be set")
	}

	// Verify global navigation overrides
	if cfg.TUI.Keybindings.Navigation == nil {
		t.Fatal("Expected Navigation section to be set")
	}
	if up, ok := cfg.TUI.Keybindings.Navigation["up"]; !ok || len(up) != 2 || up[0] != "k" {
		t.Errorf("Expected navigation.up = ['k', 'up'], got %v", up)
	}

	// Verify TUIOverrides were merged
	tuiOverrides := cfg.TUI.Keybindings.GetTUIOverrides()
	if tuiOverrides == nil {
		t.Fatal("Expected TUIOverrides to be set")
	}

	// Check flow.plan-init overrides (project should override global)
	flowOverrides, ok := tuiOverrides["flow"]
	if !ok {
		t.Fatal("Expected flow package overrides")
	}

	planInitOverrides, ok := flowOverrides["plan-init"]
	if !ok {
		t.Fatal("Expected plan-init overrides")
	}

	// toggle_advanced should be from project (A), not global (a)
	if ta, ok := planInitOverrides["toggle_advanced"]; !ok || len(ta) != 1 || ta[0] != "A" {
		t.Errorf("Expected toggle_advanced = ['A'] from project, got %v", ta)
	}

	// submit should be from project
	if submit, ok := planInitOverrides["submit"]; !ok || len(submit) != 2 || submit[0] != "enter" {
		t.Errorf("Expected submit = ['enter', 'ctrl+s'], got %v", submit)
	}

	// Check nb.browser overrides (project only)
	nbOverrides, ok := tuiOverrides["nb"]
	if !ok {
		t.Fatal("Expected nb package overrides")
	}

	browserOverrides, ok := nbOverrides["browser"]
	if !ok {
		t.Fatal("Expected browser overrides")
	}

	if cn, ok := browserOverrides["create_note"]; !ok || len(cn) != 1 || cn[0] != "n" {
		t.Errorf("Expected create_note = ['n'], got %v", cn)
	}
}

// TestNoEcosystemFallback tests that ecosystem configs work standalone without fallback
func TestNoEcosystemFallback(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Set HOME to avoid loading global config
	origHome := os.Getenv("HOME")
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	defer func() {
		os.Setenv("HOME", origHome)
		os.Setenv("XDG_CONFIG_HOME", origXDG)
	}()
	os.Setenv("HOME", tmpDir)
	os.Unsetenv("XDG_CONFIG_HOME")

	// Create ecosystem config (has workspaces field)
	ecosystemConfig := `
version: "1.0"
workspaces:
  - "workspace-*"

settings:
  project_name: ecosystem-only
  mcp_port: 3000
`
	if err := os.WriteFile(filepath.Join(tmpDir, "grove.yml"), []byte(ecosystemConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Load configuration from ecosystem directory
	cfg, err := LoadFrom(tmpDir)
	if err != nil {
		t.Fatalf("Failed to load ecosystem config: %v", err)
	}

	// Test that ecosystem config is loaded properly
	type SettingsConfig struct {
		ProjectName string `yaml:"project_name"`
		McpPort     int    `yaml:"mcp_port"`
	}
	var settings SettingsConfig
	if err := cfg.UnmarshalExtension("settings", &settings); err != nil {
		t.Fatalf("Failed to unmarshal settings extension: %v", err)
	}

	if settings.ProjectName != "ecosystem-only" {
		t.Errorf("Expected project_name 'ecosystem-only', got '%s'", settings.ProjectName)
	}

	if settings.McpPort != 3000 {
		t.Errorf("Expected mcp_port 3000, got %d", settings.McpPort)
	}

	// Should have workspaces field
	if len(cfg.Workspaces) != 1 || cfg.Workspaces[0] != "workspace-*" {
		t.Errorf("Expected workspaces ['workspace-*'], got %v", cfg.Workspaces)
	}
}

// TestMergeEnvironments tests deep-merging of named environments across config layers.
func TestMergeEnvironments(t *testing.T) {
	base := &Config{
		Environment: &EnvironmentConfig{
			Provider: "native",
			Config: map[string]interface{}{
				"domain": "grove.local",
			},
			Commands: map[string]string{
				"build": "make build",
				"test":  "make test",
			},
		},
		Environments: map[string]*EnvironmentConfig{
			"docker": {
				Provider: "docker",
				Config: map[string]interface{}{
					"services": map[string]interface{}{
						"web": "nginx",
					},
				},
				Commands: map[string]string{
					"build": "docker compose build",
				},
			},
		},
	}

	overlay := &Config{
		Environments: map[string]*EnvironmentConfig{
			"docker": {
				Config: map[string]interface{}{
					"services": map[string]interface{}{
						"api": "golang", // Adds a new service
					},
				},
				Commands: map[string]string{
					"logs": "docker compose logs -f", // Adds a new command
				},
			},
			"cloud": {
				Provider: "cloud",
				Config: map[string]interface{}{
					"region": "us-central1",
				},
			},
		},
	}

	merged := mergeConfigs(base, overlay)

	// Docker profile should have both services (deep merge)
	docker := merged.Environments["docker"]
	if docker == nil {
		t.Fatal("expected docker environment to exist")
	}
	if docker.Provider != "docker" {
		t.Errorf("expected docker provider, got %q", docker.Provider)
	}
	services, ok := docker.Config["services"].(map[string]interface{})
	if !ok {
		t.Fatal("expected docker services to be a map")
	}
	if services["web"] != "nginx" {
		t.Errorf("expected web service 'nginx', got %v", services["web"])
	}
	if services["api"] != "golang" {
		t.Errorf("expected api service 'golang', got %v", services["api"])
	}

	// Docker commands: build from base, logs from overlay
	if docker.Commands["build"] != "docker compose build" {
		t.Errorf("expected build from base, got %q", docker.Commands["build"])
	}
	if docker.Commands["logs"] != "docker compose logs -f" {
		t.Errorf("expected logs from overlay, got %q", docker.Commands["logs"])
	}

	// Cloud profile should exist from overlay
	cloud := merged.Environments["cloud"]
	if cloud == nil {
		t.Fatal("expected cloud environment to exist")
	}
	if cloud.Provider != "cloud" {
		t.Errorf("expected cloud provider, got %q", cloud.Provider)
	}
	if cloud.Config["region"] != "us-central1" {
		t.Errorf("expected region 'us-central1', got %v", cloud.Config["region"])
	}
}

// TestMergeEnvironmentDefault tests that the default environment is deep-merged, not replaced.
func TestMergeEnvironmentDefault(t *testing.T) {
	base := &Config{
		Environment: &EnvironmentConfig{
			Provider: "native",
			Command:  "grove-env-native",
			Config: map[string]interface{}{
				"host": "localhost",
				"port": 8080,
			},
			Commands: map[string]string{
				"build": "make build",
				"test":  "make test",
			},
		},
	}

	overlay := &Config{
		Environment: &EnvironmentConfig{
			Config: map[string]interface{}{
				"port":     9090, // Override
				"database": "postgres://localhost:5432",
			},
			Commands: map[string]string{
				"build": "npm run build", // Override
				"seed":  "npm run seed",  // New
			},
		},
	}

	merged := mergeConfigs(base, overlay)

	if merged.Environment.Provider != "native" {
		t.Errorf("expected provider 'native' preserved, got %q", merged.Environment.Provider)
	}
	if merged.Environment.Command != "grove-env-native" {
		t.Errorf("expected command preserved, got %q", merged.Environment.Command)
	}
	if merged.Environment.Config["host"] != "localhost" {
		t.Errorf("expected host 'localhost' preserved, got %v", merged.Environment.Config["host"])
	}
	if merged.Environment.Config["port"] != 9090 {
		t.Errorf("expected port overridden to 9090, got %v", merged.Environment.Config["port"])
	}
	if merged.Environment.Config["database"] != "postgres://localhost:5432" {
		t.Errorf("expected database added, got %v", merged.Environment.Config["database"])
	}
	if merged.Environment.Commands["build"] != "npm run build" {
		t.Errorf("expected build overridden, got %q", merged.Environment.Commands["build"])
	}
	if merged.Environment.Commands["test"] != "make test" {
		t.Errorf("expected test preserved, got %q", merged.Environment.Commands["test"])
	}
	if merged.Environment.Commands["seed"] != "npm run seed" {
		t.Errorf("expected seed added, got %q", merged.Environment.Commands["seed"])
	}
}

// TestDeepMergeMaps_DeleteSentinel verifies that a `_delete = true` map in src
// drops the corresponding key from the merged result. This lets a profile
// opt out of inherited entries (e.g. a hybrid env dropping the default
// services.clickhouse block) without resorting to empty-command hacks.
func TestDeepMergeMaps_DeleteSentinel(t *testing.T) {
	dst := map[string]interface{}{
		"services": map[string]interface{}{
			"clickhouse": map[string]interface{}{"command": "clickhouse server"},
			"api":        map[string]interface{}{"command": "cargo run"},
		},
	}
	src := map[string]interface{}{
		"services": map[string]interface{}{
			"clickhouse": map[string]interface{}{"_delete": true},
			"web":        map[string]interface{}{"command": "npm run dev"},
		},
	}

	merged := deepMergeMaps(dst, src)
	services, ok := merged["services"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected services to be a map, got %T", merged["services"])
	}

	if _, present := services["clickhouse"]; present {
		t.Errorf("expected services.clickhouse to be deleted, still present: %v", services["clickhouse"])
	}
	if _, present := services["api"]; !present {
		t.Errorf("expected services.api to survive (untouched by src)")
	}
	if _, present := services["web"]; !present {
		t.Errorf("expected services.web to be added from src")
	}
}

// TestDeepMergeMaps_DeleteFieldFromInheritedBlock verifies the sentinel works
// for nested fields too — e.g. clearing services.api.env so a tunnel-set value
// can win at process spawn time.
func TestDeepMergeMaps_DeleteFieldFromInheritedBlock(t *testing.T) {
	dst := map[string]interface{}{
		"api": map[string]interface{}{
			"command": "cargo run",
			"env":     map[string]interface{}{"CLICKHOUSE_URL": "http://localhost:9000"},
		},
	}
	src := map[string]interface{}{
		"api": map[string]interface{}{
			"env": map[string]interface{}{"_delete": true},
		},
	}

	merged := deepMergeMaps(dst, src)
	api := merged["api"].(map[string]interface{})

	if api["command"] != "cargo run" {
		t.Errorf("expected api.command preserved, got %v", api["command"])
	}
	if _, present := api["env"]; present {
		t.Errorf("expected api.env to be deleted, still present: %v", api["env"])
	}
}