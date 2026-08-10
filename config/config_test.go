package config

import (
	"os"
	"path/filepath"
	"strings"
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

func TestLoadLegacyTopologyErrorNamesSourcePath(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	for _, tc := range []struct {
		name    string
		path    string
		content string
	}{
		{name: "TOML", path: filepath.Join(t.TempDir(), "actual.toml"), content: "[groves.old]\npath = '/code'\n"},
		{name: "YAML", path: filepath.Join(t.TempDir(), "actual.yml"), content: "search_paths:\n  old:\n    path: /code\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(tc.path, []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := Load(tc.path)
			if err == nil || !strings.Contains(err.Error(), tc.path) || strings.Contains(err.Error(), "<TOML bytes>") || strings.Contains(err.Error(), "<YAML bytes>") {
				t.Fatalf("error = %v, want actual source path %s", err, tc.path)
			}
		})
	}
}

func TestLegacyTopologyRequiresMigration(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	for _, tc := range []struct {
		name  string
		load  func() error
		table string
	}{
		{"yaml groves", func() error { _, err := LoadFromBytes([]byte("groves: {}\n")); return err }, "groves"},
		{"toml search paths", func() error { _, err := LoadFromTOMLBytes([]byte("[search_paths.old]\npath = '/code'\n")); return err }, "search_paths"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.load()
			if err == nil || !strings.Contains(err.Error(), tc.table) || !strings.Contains(err.Error(), "grove migrate") || strings.Contains(err.Error(), "roots.toml") {
				t.Fatalf("hermetic byte-parse error = %v", err)
			}
		})
	}
	if err := os.MkdirAll(filepath.Join(configHome, "grove"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configHome, "grove", "roots.toml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadFromTOMLBytes([]byte("[groves.old]\npath = '/code'\n"))
	if err == nil || !strings.Contains(err.Error(), "groves") || !strings.Contains(err.Error(), "grove migrate") || strings.Contains(err.Error(), "forbidden mixed state") || strings.Contains(err.Error(), "roots.toml") {
		t.Fatalf("ambient roots leaked into byte-only parse error = %v", err)
	}
}

// TestExtensionsDoNotInterfereWithCoreConfig verifies that extensions don't break core config parsing
func TestExtensionsDoNotInterfereWithCoreConfig(t *testing.T) {
	yamlContent := []byte(`
version: "1.0"
settings:
  project_name: test-project
  network_name: custom-network

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

	// Verify settings extension is captured
	if _, ok := cfg.Extensions["settings"]; !ok {
		t.Error("Expected 'settings' extension to be present")
	}

	// Unmarshal and verify settings
	type SettingsConfig struct {
		ProjectName string `yaml:"project_name"`
		NetworkName string `yaml:"network_name"`
	}
	var settings SettingsConfig
	if err := cfg.UnmarshalExtension("settings", &settings); err != nil {
		t.Fatalf("Failed to unmarshal settings extension: %v", err)
	}

	if settings.ProjectName != "test-project" {
		t.Errorf("Expected project name 'test-project', got '%s'", settings.ProjectName)
	}

	if settings.NetworkName != "custom-network" {
		t.Errorf("Expected network name 'custom-network', got '%s'", settings.NetworkName)
	}

	// Verify custom extension is also captured
	if _, ok := cfg.Extensions["custom"]; !ok {
		t.Error("Expected 'custom' extension to be present")
	}
}
