package config

import (
	"fmt"
	"github.com/mitchellh/mapstructure"
)

// Config represents the grove.yml configuration
type Config struct {
	Version string      `yaml:"version"`
	Agent   AgentConfig `yaml:"agent,omitempty"`

	// Extensions captures all other top-level keys for extensibility.
	// This allows other tools in the Grove ecosystem to define their
	// own configuration sections in grove.yml.
	Extensions map[string]interface{} `yaml:",inline"`
}

// AgentConfig defines the configuration for the built-in Grove agent
type AgentConfig struct {
	Enabled                  bool     `yaml:"enabled"`
	Image                    string   `yaml:"image"`
	LogsPath                 string   `yaml:"logs_path"`
	ExtraVolumes             []string `yaml:"extra_volumes"`
	NotesDir                 string   `yaml:"notes_dir,omitempty"`
	Args                     []string `yaml:"args"`
	OutputFormat             string   `yaml:"output_format"` // text (default), json, or stream-json
	MountWorkspaceAtHostPath bool     `yaml:"mount_workspace_at_host_path,omitempty"`
	UseSuperprojectRoot      bool     `yaml:"use_superproject_root,omitempty"`
}

// SetDefaults sets default values for configuration
func (c *Config) SetDefaults() {
	if c.Version == "" {
		c.Version = "1.0"
	}

	// Set Agent defaults
	if c.Agent.Enabled {
		if c.Agent.Image == "" {
			// Default to v0.1.0 for now - this should be injected from CLI
			c.Agent.Image = "ghcr.io/mattsolo1/grove-agent:v0.1.0"
		}
		if c.Agent.LogsPath == "" {
			c.Agent.LogsPath = "~/.claude/projects"
		}
	}
}

// UnmarshalExtension decodes a specific extension's configuration from the
// loaded grove.yml into the provided target struct. The target must be a pointer.
// This provides a type-safe way for extensions to access their
// custom configuration sections.
//
// Example:
//
//	var flowCfg myapp.FlowConfig
//	err := coreCfg.UnmarshalExtension("flow", &flowCfg)
func (c *Config) UnmarshalExtension(key string, target interface{}) error {
	extensionConfig, ok := c.Extensions[key]
	if !ok {
		// It's not an error if the key doesn't exist.
		// The target struct will simply remain zero-valued.
		return nil
	}

	// Use mapstructure to decode the generic map[string]interface{}
	// into the strongly-typed target struct. We configure it to use
	// `yaml` tags for consistency.
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  target,
		TagName: "yaml",
	})
	if err != nil {
		return fmt.Errorf("failed to create mapstructure decoder: %w", err)
	}

	if err := decoder.Decode(extensionConfig); err != nil {
		return fmt.Errorf("failed to decode extension config for '%s': %w", key, err)
	}

	return nil
}

// ConfigSource identifies the origin of a configuration value.
type ConfigSource string

const (
	SourceDefault   ConfigSource = "default"
	SourceGlobal    ConfigSource = "global"
	SourceProject   ConfigSource = "project"
	SourceOverride  ConfigSource = "override"
	SourceUnknown   ConfigSource = "unknown"
)

// OverrideSource holds a raw configuration from an override file and its path.
type OverrideSource struct {
	Path   string
	Config *Config
}

// LayeredConfig holds the raw configuration from each source file,
// as well as the final merged configuration, for analysis purposes.
type LayeredConfig struct {
	Default   *Config          // Config with only default values applied.
	Global    *Config          // Raw config from the global file.
	Project   *Config          // Raw config from the project file.
	Overrides []OverrideSource // Raw configs from override files, in order of application.
	Final     *Config          // The fully merged and validated config.
	FilePaths map[ConfigSource]string // Maps sources to their file paths.
}
