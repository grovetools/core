package config

import (
	"fmt"
	"github.com/mitchellh/mapstructure"
)

// Config represents the grove.yml configuration
type Config struct {
	Version       string                   `yaml:"version"`
	Networks      map[string]NetworkConfig `yaml:"networks"`
	Services      map[string]ServiceConfig `yaml:"services"`
	Volumes       map[string]VolumeConfig  `yaml:"volumes"`
	Profiles      map[string]ProfileConfig `yaml:"profiles"`
	Agent         AgentConfig              `yaml:"agent,omitempty"`
	Settings      Settings                 `yaml:"settings"`
	Workspaces    []string                 `yaml:"workspaces,omitempty"`

	// Extensions captures all other top-level keys for extensibility.
	// This allows other tools in the Grove ecosystem to define their
	// own configuration sections in grove.yml.
	Extensions map[string]interface{} `yaml:",inline"`
}

type NetworkConfig struct {
	External bool   `yaml:"external"`
	Driver   string `yaml:"driver"`
}

type VolumeConfig struct {
	External bool   `yaml:"external"`
	Driver   string `yaml:"driver"`
}

type ServiceConfig struct {
	Build       interface{}       `yaml:"build"`
	Image       string            `yaml:"image"`
	Ports       []string          `yaml:"ports"`
	Environment []string          `yaml:"environment"`
	EnvFile     []string          `yaml:"env_file"`
	Volumes     []string          `yaml:"volumes"`
	DependsOn   []string          `yaml:"depends_on"`
	Labels      map[string]string `yaml:"labels"`
	Command     interface{}       `yaml:"command"`
	Profiles    []string          `yaml:"profiles"`
}

type ProfileConfig struct {
	Services []string `yaml:"services"`
	EnvFile  []string `yaml:"env_file"`
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

type Settings struct {
	ProjectName    string `yaml:"project_name"`
	DefaultProfile string `yaml:"default_profile"`
	TraefikEnabled *bool  `yaml:"traefik_enabled,omitempty"`
	NetworkName    string `yaml:"network_name"`
	DomainSuffix   string `yaml:"domain_suffix"`
	AutoInference  *bool  `yaml:"auto_inference,omitempty"`
	Concurrency    int    `yaml:"concurrency,omitempty"`
	McpPort        int    `yaml:"mcp_port,omitempty"`
	CanopyPort     int    `yaml:"canopy_port,omitempty"`
}

// SetDefaults sets default values for configuration
func (c *Config) SetDefaults() {
	if c.Version == "" {
		c.Version = "1.0"
	}
	if c.Settings.DomainSuffix == "" {
		c.Settings.DomainSuffix = "localhost"
	}
	if c.Settings.NetworkName == "" {
		c.Settings.NetworkName = "grove"
	}
	// Enable Traefik by default
	if c.Settings.TraefikEnabled == nil {
		enabled := true
		c.Settings.TraefikEnabled = &enabled
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

	// Set Settings defaults
	if c.Settings.McpPort == 0 {
		c.Settings.McpPort = 1667
	}
	if c.Settings.CanopyPort == 0 {
		c.Settings.CanopyPort = 8888
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
