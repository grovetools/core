package config

import (
	"testing"
)

// TestExtensions verifies that custom extensions in grove.yml are properly loaded
func TestExtensions(t *testing.T) {
	yamlContent := []byte(`
version: "1.0"
settings:
  project_name: test-project

# Extension fields from grove-flow
flow:
  chat_directory: "/path/to/chats"
  max_messages: 100

# Extension fields from another hypothetical tool
monitoring:
  enabled: true
  interval: 30
`)

	cfg, err := LoadFromBytes(yamlContent)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify extensions were captured
	if cfg.Extensions == nil {
		t.Fatal("Extensions map should not be nil")
	}

	// Test flow extension
	flowExt, ok := cfg.Extensions["flow"]
	if !ok {
		t.Fatal("Expected 'flow' extension to be present")
	}

	// Test UnmarshalExtension for flow
	type FlowConfig struct {
		ChatDirectory string `yaml:"chat_directory"`
		MaxMessages   int    `yaml:"max_messages"`
	}

	var flowCfg FlowConfig
	if err := cfg.UnmarshalExtension("flow", &flowCfg); err != nil {
		t.Fatalf("Failed to unmarshal flow extension: %v", err)
	}

	if flowCfg.ChatDirectory != "/path/to/chats" {
		t.Errorf("Expected chat_directory to be '/path/to/chats', got '%s'", flowCfg.ChatDirectory)
	}

	if flowCfg.MaxMessages != 100 {
		t.Errorf("Expected max_messages to be 100, got %d", flowCfg.MaxMessages)
	}

	// Test monitoring extension
	monitoringExt, ok := cfg.Extensions["monitoring"]
	if !ok {
		t.Fatal("Expected 'monitoring' extension to be present")
	}

	// Test UnmarshalExtension for monitoring
	type MonitoringConfig struct {
		Enabled  bool `yaml:"enabled"`
		Interval int  `yaml:"interval"`
	}

	var monCfg MonitoringConfig
	if err := cfg.UnmarshalExtension("monitoring", &monCfg); err != nil {
		t.Fatalf("Failed to unmarshal monitoring extension: %v", err)
	}

	if !monCfg.Enabled {
		t.Error("Expected monitoring to be enabled")
	}

	if monCfg.Interval != 30 {
		t.Errorf("Expected interval to be 30, got %d", monCfg.Interval)
	}

	// Test non-existent extension (should not error)
	type UnknownConfig struct {
		SomeField string `yaml:"some_field"`
	}

	var unknownCfg UnknownConfig
	if err := cfg.UnmarshalExtension("unknown", &unknownCfg); err != nil {
		t.Fatalf("UnmarshalExtension should not error for non-existent keys: %v", err)
	}

	// unknownCfg should remain zero-valued
	if unknownCfg.SomeField != "" {
		t.Errorf("Expected SomeField to be empty for non-existent extension")
	}

	// Verify that flow extension is a map
	if _, ok := flowExt.(map[string]interface{}); !ok {
		t.Errorf("Expected flow extension to be a map[string]interface{}, got %T", flowExt)
	}

	// Verify that monitoring extension is a map
	if _, ok := monitoringExt.(map[string]interface{}); !ok {
		t.Errorf("Expected monitoring extension to be a map[string]interface{}, got %T", monitoringExt)
	}
}

// TestExtensionsDoNotInterfereWithCoreConfig verifies that extensions don't break core config parsing
func TestExtensionsDoNotInterfereWithCoreConfig(t *testing.T) {
	yamlContent := []byte(`
version: "1.0"
settings:
  project_name: test-project
  network_name: custom-network

agent:
  enabled: true

# Custom extension
custom:
  feature: enabled
  config:
    nested: true
`)

	cfg, err := LoadFromBytes(yamlContent)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify core config is properly loaded
	if cfg.Version != "1.0" {
		t.Errorf("Expected version '1.0', got '%s'", cfg.Version)
	}

	if cfg.Settings.ProjectName != "test-project" {
		t.Errorf("Expected project name 'test-project', got '%s'", cfg.Settings.ProjectName)
	}

	if cfg.Settings.NetworkName != "custom-network" {
		t.Errorf("Expected network name 'custom-network', got '%s'", cfg.Settings.NetworkName)
	}

	if !cfg.Agent.Enabled {
		t.Error("Expected agent to be enabled")
	}

	// Verify extension is also captured
	if _, ok := cfg.Extensions["custom"]; !ok {
		t.Error("Expected 'custom' extension to be present")
	}
}