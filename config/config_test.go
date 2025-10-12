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

// TestBackwardCompatibilityGrovesToSearchPaths verifies that old "groves" key
// is automatically migrated to "search_paths"
func TestBackwardCompatibilityGrovesToSearchPaths(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		expected int // expected number of search paths
	}{
		{
			name: "old groves key",
			yaml: `
version: "1.0"
groves:
  home:
    path: ~/Code
    enabled: true
  work:
    path: ~/Work
    enabled: true
`,
			expected: 2,
		},
		{
			name: "new search_paths key",
			yaml: `
version: "1.0"
search_paths:
  home:
    path: ~/Code
    enabled: true
  work:
    path: ~/Work
    enabled: true
`,
			expected: 2,
		},
		{
			name: "both keys present (search_paths wins)",
			yaml: `
version: "1.0"
groves:
  old:
    path: ~/OldPath
    enabled: true
search_paths:
  new:
    path: ~/NewPath
    enabled: true
`,
			expected: 1, // only search_paths should be used
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := LoadFromBytes([]byte(tt.yaml))
			if err != nil {
				t.Fatalf("Failed to load config: %v", err)
			}

			if len(cfg.SearchPaths) != tt.expected {
				t.Errorf("Expected %d search paths, got %d", tt.expected, len(cfg.SearchPaths))
			}

			// For the "both keys" test, verify the right one was used
			if tt.name == "both keys present (search_paths wins)" {
				if _, ok := cfg.SearchPaths["new"]; !ok {
					t.Error("Expected 'new' search path to be present")
				}
				if _, ok := cfg.SearchPaths["old"]; ok {
					t.Error("Expected 'old' search path (from groves) to NOT be present")
				}
			}
		})
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