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

agent:
  enabled: true
  image: global-agent:latest

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

	// Test settings merging
	if cfg.Settings.ProjectName != "my-project" {
		t.Errorf("Expected project_name 'my-project' from project, got '%s'", cfg.Settings.ProjectName)
	}
	if cfg.Settings.NetworkName != "project-network" {
		t.Errorf("Expected network_name 'project-network' from project, got '%s'", cfg.Settings.NetworkName)
	}
	if cfg.Settings.DomainSuffix != "global.local" {
		t.Errorf("Expected domain_suffix 'global.local' from global, got '%s'", cfg.Settings.DomainSuffix)
	}
	if cfg.Settings.McpPort != 5678 {
		t.Errorf("Expected mcp_port 5678 from override, got %d", cfg.Settings.McpPort)
	}

	// Test agent (from global, not overridden)
	if !cfg.Agent.Enabled {
		t.Error("Expected agent to be enabled from global config")
	}
	if cfg.Agent.Image != "global-agent:latest" {
		t.Errorf("Expected agent image 'global-agent:latest' from global, got '%s'", cfg.Agent.Image)
	}

	// Test services (from project + override)
	apiService, ok := cfg.Services["api"]
	if !ok {
		t.Fatal("Expected 'api' service to exist")
	}
	if apiService.Image != "api:latest" {
		t.Errorf("Expected api image 'api:latest', got '%s'", apiService.Image)
	}
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

	if cfg.Settings.ProjectName != "test-project" {
		t.Errorf("Expected project_name 'test-project', got '%s'", cfg.Settings.ProjectName)
	}

	// Check that defaults are still applied (with auto_inference disabled)
	if cfg.Settings.NetworkName != "grove" {
		t.Errorf("Expected default network_name 'grove', got '%s'", cfg.Settings.NetworkName)
	}
}

// TestServiceMerging specifically tests the service merging logic
func TestServiceMerging(t *testing.T) {
	base := ServiceConfig{
		Image: "base:latest",
		Ports: []string{"8080:8080"},
		Environment: []string{"ENV=base"},
		Labels: map[string]string{
			"app": "base",
			"env": "dev",
		},
	}

	override := ServiceConfig{
		Ports: []string{"9090:8080"},
		Labels: map[string]string{
			"env": "prod",
			"new": "label",
		},
	}

	result := mergeService(base, override)

	// Image should remain from base
	if result.Image != "base:latest" {
		t.Errorf("Expected image 'base:latest', got '%s'", result.Image)
	}

	// Ports should be replaced
	if len(result.Ports) != 1 || result.Ports[0] != "9090:8080" {
		t.Errorf("Expected ports to be replaced, got %v", result.Ports)
	}

	// Environment should remain from base (not overridden)
	if len(result.Environment) != 1 || result.Environment[0] != "ENV=base" {
		t.Errorf("Expected environment to remain from base, got %v", result.Environment)
	}

	// Labels should be merged
	if result.Labels["app"] != "base" {
		t.Errorf("Expected label 'app' to remain 'base', got '%s'", result.Labels["app"])
	}
	if result.Labels["env"] != "prod" {
		t.Errorf("Expected label 'env' to be overridden to 'prod', got '%s'", result.Labels["env"])
	}
	if result.Labels["new"] != "label" {
		t.Errorf("Expected new label 'new' to be 'label', got '%s'", result.Labels["new"])
	}
}