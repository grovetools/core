package config

import (
	"fmt"
	"github.com/mitchellh/mapstructure"
	"gopkg.in/yaml.v3"
)

//go:generate go run ../tools/schema-generator/
//go:generate sh -c "cd .. && go run ./tools/notebook-schema-generator/"


// SearchPathConfig defines the configuration for a single search path.
// DEPRECATED: Use GroveSourceConfig instead.
type SearchPathConfig struct {
	Path        string `yaml:"path"`
	Enabled     bool   `yaml:"enabled"`
	Description string `yaml:"description,omitempty"`
}

// GroveSourceConfig defines the configuration for a single grove source.
type GroveSourceConfig struct {
	Path        string `yaml:"path"`
	Enabled     bool   `yaml:"enabled"`
	Description string `yaml:"description,omitempty"`
	Notebook    string `yaml:"notebook,omitempty"`
}

// ExplicitProject defines a specific project to include regardless of discovery.
type ExplicitProject struct {
	Path        string `yaml:"path"`
	Name        string `yaml:"name,omitempty"`
	Description string `yaml:"description,omitempty"`
	Enabled     bool   `yaml:"enabled"`
}

// NoteTypeConfig defines the configuration for a single, user-defined note type.
type NoteTypeConfig struct {
	Description    string `yaml:"description,omitempty"`
	TemplatePath   string `yaml:"template_path,omitempty"`
	FilenameFormat string `yaml:"filename_format,omitempty"` // e.g., "date-title", "timestamp-title", "title"
}

// GlobalNotebookConfig defines the configuration for the system-wide global notebook.
type GlobalNotebookConfig struct {
	RootDir string `yaml:"root_dir"`
}

// NotebookRules defines the usage rules for notebooks.
type NotebookRules struct {
	Default string                `yaml:"default,omitempty"`
	Global  *GlobalNotebookConfig `yaml:"global,omitempty"`
}

// NotebooksConfig groups all notebook-related settings.
type NotebooksConfig struct {
	Definitions map[string]*Notebook `yaml:"definitions,omitempty"`
	Rules       *NotebookRules       `yaml:"rules,omitempty"`
}

// Notebook defines the configuration for a single, named notebook system.
type Notebook struct {
	// RootDir is the absolute path to the root of the notebook.
	// If this is set, the system operates in "Centralized Mode".
	// If empty, it operates in "Local Mode".
	RootDir string `yaml:"root_dir"`

	// Path templates for customizing directory structure in Centralized Mode.
	// These are optional and have sensible defaults.
	NotesPathTemplate string `yaml:"notes_path_template,omitempty"`
	PlansPathTemplate string `yaml:"plans_path_template,omitempty"`
	ChatsPathTemplate string `yaml:"chats_path_template,omitempty"`

	// Types defines a map of user-configurable note types.
	// This will override the hardcoded defaults in grove-notebook if provided.
	Types map[string]*NoteTypeConfig `yaml:"types,omitempty"`
}

// Config represents the grove.yml configuration
type Config struct {
	Name       string   `yaml:"name,omitempty"`
	Version    string   `yaml:"version"`
	Workspaces []string `yaml:"workspaces,omitempty"`

	// Notebooks contains all notebook-related configuration.
	Notebooks *NotebooksConfig `yaml:"notebooks,omitempty"`

	// Groves defines the root directories to search for projects and ecosystems.
	// This is typically set in the global ~/.config/grove/grove.yml file.
	Groves map[string]GroveSourceConfig `yaml:"groves,omitempty"`

	// SearchPaths is a legacy field for backward compatibility.
	// DEPRECATED: Use Groves instead.
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
// for the old configuration formats.
func (c *Config) UnmarshalYAML(node *yaml.Node) error {
	// Create a temporary struct with all fields to capture the data, including legacy ones.
	type rawConfig struct {
		Name             string                       `yaml:"name,omitempty"`
		Version          string                       `yaml:"version"`
		Workspaces       []string                     `yaml:"workspaces,omitempty"`
		Notebooks        *NotebooksConfig             `yaml:"notebooks,omitempty"`
		Groves           map[string]GroveSourceConfig `yaml:"groves,omitempty"`
		ExplicitProjects []ExplicitProject            `yaml:"explicit_projects,omitempty"`
		Extensions       map[string]interface{}       `yaml:",inline"`

		// --- Legacy Fields for Backward Compatibility ---
		SearchPaths       map[string]SearchPathConfig `yaml:"search_paths,omitempty"`      // Old name for Groves
		LegacyNotebooks   map[string]*Notebook        `yaml:"-"`                           // To catch top-level notebooks map
		LegacyNotebook    *Notebook                   `yaml:"notebook,omitempty"`          // Very old single notebook
		DefaultNotebook   string                      `yaml:"default_notebook,omitempty"`  // Old top-level default
		GlobalNotebookDir string                      `yaml:"global_notebook_dir,omitempty"` // Old top-level global dir
	}

	var raw rawConfig
	if err := node.Decode(&raw); err != nil {
		return err
	}

	// Copy standard fields
	c.Name = raw.Name
	c.Version = raw.Version
	c.Workspaces = raw.Workspaces
	c.ExplicitProjects = raw.ExplicitProjects
	c.Extensions = raw.Extensions

	// Handle backward compatibility for `search_paths` -> `groves`
	if len(raw.Groves) > 0 {
		c.Groves = raw.Groves
	} else if len(raw.SearchPaths) > 0 {
		// Migrate old `search_paths` key to new `groves`
		c.Groves = make(map[string]GroveSourceConfig)
		for k, v := range raw.SearchPaths {
			c.Groves[k] = GroveSourceConfig{
				Path:        v.Path,
				Enabled:     v.Enabled,
				Description: v.Description,
			}
		}
	}

	// Handle new nested `notebooks` structure
	c.Notebooks = raw.Notebooks
	if c.Notebooks == nil {
		c.Notebooks = &NotebooksConfig{}
	}

	// We need to detect if the YAML has the old flat notebooks map format
	// This requires checking the raw YAML node directly
	var legacyNotebooksMap map[string]*Notebook
	for i := 0; i < len(node.Content); i += 2 {
		if i+1 < len(node.Content) && node.Content[i].Value == "notebooks" {
			// Check if this is a map of notebook definitions (old format)
			// vs the new nested NotebooksConfig format
			nbNode := node.Content[i+1]
			if nbNode.Kind == yaml.MappingNode {
				// Try to detect if it's the old format by checking for "definitions" or "rules" keys
				hasDefinitions := false
				hasRules := false
				for j := 0; j < len(nbNode.Content); j += 2 {
					if j+1 < len(nbNode.Content) {
						key := nbNode.Content[j].Value
						if key == "definitions" {
							hasDefinitions = true
						} else if key == "rules" {
							hasRules = true
						}
					}
				}
				// If it doesn't have definitions or rules, it's the old flat format
				if !hasDefinitions && !hasRules {
					legacyNotebooksMap = make(map[string]*Notebook)
					if err := nbNode.Decode(&legacyNotebooksMap); err == nil {
						raw.LegacyNotebooks = legacyNotebooksMap
					}
				}
			}
			break
		}
	}

	// Handle backward compatibility for top-level `notebooks` map (old format)
	if len(raw.LegacyNotebooks) > 0 && c.Notebooks.Definitions == nil {
		c.Notebooks.Definitions = raw.LegacyNotebooks
	}

	// Handle very old single `notebook` field
	if raw.LegacyNotebook != nil && c.Notebooks.Definitions == nil {
		c.Notebooks.Definitions = map[string]*Notebook{
			"default": raw.LegacyNotebook,
		}
	}

	// Handle backward compatibility for top-level `default_notebook` and `global_notebook_dir`
	if c.Notebooks.Rules == nil {
		c.Notebooks.Rules = &NotebookRules{}
	}
	if raw.DefaultNotebook != "" && c.Notebooks.Rules.Default == "" {
		c.Notebooks.Rules.Default = raw.DefaultNotebook
	}
	if raw.GlobalNotebookDir != "" {
		if c.Notebooks.Rules.Global == nil {
			c.Notebooks.Rules.Global = &GlobalNotebookConfig{}
		}
		if c.Notebooks.Rules.Global.RootDir == "" {
			c.Notebooks.Rules.Global.RootDir = raw.GlobalNotebookDir
		}
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
