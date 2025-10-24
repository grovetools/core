package config

import (
	"fmt"
	"github.com/mitchellh/mapstructure"
	"gopkg.in/yaml.v3"
)

//go:generate go run ../tools/schema-generator/
//go:generate sh -c "cd .. && go run ./tools/notebook-schema-generator/"

// SearchPathConfig defines the configuration for a single search path.
type SearchPathConfig struct {
	Path        string `yaml:"path"`
	Enabled     bool   `yaml:"enabled"`
	Description string `yaml:"description,omitempty"`
}

// ExplicitProject defines a specific project to include regardless of discovery.
type ExplicitProject struct {
	Path        string `yaml:"path"`
	Name        string `yaml:"name,omitempty"`
	Description string `yaml:"description,omitempty"`
	Enabled     bool   `yaml:"enabled"`
}

// NotebookConfig defines the configuration for the centralized notebook system.
type NotebookConfig struct {
	// RootDir is the absolute path to the root of the notebook.
	// If this is set, the system operates in "Centralized Mode".
	// If empty, it operates in "Local Mode".
	RootDir string `yaml:"root_dir,omitempty"`

	// Path templates for customizing directory structure in Centralized Mode.
	// These are optional and have sensible defaults.
	NotesPathTemplate       string `yaml:"notes_path_template,omitempty"`
	PlansPathTemplate       string `yaml:"plans_path_template,omitempty"`
	ChatsPathTemplate       string `yaml:"chats_path_template,omitempty"`
	GlobalNotesPathTemplate string `yaml:"global_notes_path_template,omitempty"`
	GlobalPlansPathTemplate string `yaml:"global_plans_path_template,omitempty"`
	GlobalChatsPathTemplate string `yaml:"global_chats_path_template,omitempty"`
}

// Config represents the grove.yml configuration
type Config struct {
	Name       string   `yaml:"name,omitempty"`
	Version    string   `yaml:"version"`
	Workspaces []string `yaml:"workspaces,omitempty"`

	// Notebook defines the configuration for the centralized note and plan storage.
	// This is the new, standardized way to configure where persistent data lives.
	Notebook *NotebookConfig `yaml:"notebook,omitempty"`

	// SearchPaths defines the root directories to search for projects and ecosystems.
	// This is typically set in the global ~/.config/grove/grove.yml file.
	//
	// Note: For backward compatibility, the old "groves" key is still supported and will
	// be automatically migrated to "search_paths" when loading configuration.
	SearchPaths map[string]SearchPathConfig `yaml:"search_paths,omitempty"`

	// ExplicitProjects defines specific projects to include without discovery.
	// Useful for including individual directories that don't fit the grove model.
	ExplicitProjects []ExplicitProject `yaml:"explicit_projects,omitempty"`

	// Extensions captures all other top-level keys for extensibility.
	// This allows other tools in the Grove ecosystem to define their
	// own configuration sections in grove.yml.
	Extensions map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements custom YAML unmarshaling to handle backward compatibility
// for the old "groves" key, now renamed to "search_paths".
func (c *Config) UnmarshalYAML(node *yaml.Node) error {
	// Create a temporary struct with all fields to capture the data
	type rawConfig struct {
		Name             string                       `yaml:"name,omitempty"`
		Version          string                       `yaml:"version"`
		Workspaces       []string                     `yaml:"workspaces,omitempty"`
		Notebook         *NotebookConfig              `yaml:"notebook,omitempty"`
		SearchPaths      map[string]SearchPathConfig  `yaml:"search_paths,omitempty"`
		Groves           map[string]SearchPathConfig  `yaml:"groves,omitempty"` // Legacy field
		ExplicitProjects []ExplicitProject            `yaml:"explicit_projects,omitempty"`
		Extensions       map[string]interface{}       `yaml:",inline"`
	}

	var raw rawConfig
	if err := node.Decode(&raw); err != nil {
		return err
	}

	// Copy all fields
	c.Name = raw.Name
	c.Version = raw.Version
	c.Workspaces = raw.Workspaces
	c.Notebook = raw.Notebook
	c.ExplicitProjects = raw.ExplicitProjects
	c.Extensions = raw.Extensions

	// Handle backward compatibility: if "groves" is present but "search_paths" is not,
	// use "groves" as "search_paths"
	if len(raw.SearchPaths) > 0 {
		c.SearchPaths = raw.SearchPaths
	} else if len(raw.Groves) > 0 {
		// Migrate old "groves" key to new "search_paths"
		c.SearchPaths = raw.Groves
	}

	return nil
}

// SetDefaults sets default values for configuration
func (c *Config) SetDefaults() {
	if c.Version == "" {
		c.Version = "1.0"
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
	SourceEcosystem ConfigSource = "ecosystem"
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
	Ecosystem *Config          // Raw config from the ecosystem file (if workspace is in an ecosystem).
	Project   *Config          // Raw config from the project file.
	Overrides []OverrideSource // Raw configs from override files, in order of application.
	Final     *Config          // The fully merged and validated config.
	FilePaths map[ConfigSource]string // Maps sources to their file paths.
}
