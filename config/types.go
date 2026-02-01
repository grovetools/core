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
	Enabled     *bool  `yaml:"enabled,omitempty"`
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
	// Icon specifies the icon for TUI display (e.g., from theme.Icon... constants)
	Icon string `yaml:"icon,omitempty"`
	// IconColor specifies the lipgloss color for the icon in the TUI
	IconColor string `yaml:"icon_color,omitempty"`
	// DefaultExpand determines if this group is expanded by default in the TUI
	DefaultExpand bool `yaml:"default_expand,omitempty"`
	// SortOrder is used for sorting groups in the TUI (lower numbers appear first)
	SortOrder int `yaml:"sort_order,omitempty"`
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

// NvimEmbedConfig holds settings for the embedded Neovim component.
type NvimEmbedConfig struct {
	UserConfig bool `yaml:"user_config"` // If true, loads the user's default Neovim config (~/.config/nvim)
}

// TUIConfig holds TUI-specific settings.
type TUIConfig struct {
	Icons     string           `yaml:"icons,omitempty"`      // Can be "nerd" or "ascii"
	Theme     string           `yaml:"theme,omitempty"`      // Color theme: "kanagawa", "gruvbox", or "terminal"
	NvimEmbed *NvimEmbedConfig `yaml:"nvim_embed,omitempty"` // Embedded Neovim configuration
}

// ContextConfig holds configuration for the grove-context (cx) tool.
type ContextConfig struct {
	// ReposDir specifies where 'cx repo' stores bare repositories.
	// If nil, defaults to ~/.grove/cx.
	// If set to empty string "", repository discovery/management is disabled.
	ReposDir *string `yaml:"repos_dir,omitempty"`
}

// HookCommand defines a command to be executed for a hook.
type HookCommand struct {
	Name    string `yaml:"name" jsonschema:"description=Name of the hook command"`
	Command string `yaml:"command" jsonschema:"description=Shell command to execute"`
	RunIf   string `yaml:"run_if,omitempty" jsonschema:"enum=always,enum=changes,description=Condition to run the command (always or changes)"`
}

// HooksConfig groups all hook-related settings.
type HooksConfig struct {
	OnStop []HookCommand `yaml:"on_stop,omitempty" jsonschema:"description=Commands to run when a session stops"`
}

// Notebook defines the configuration for a single, named notebook system.
type Notebook struct {
	// RootDir is the absolute path to the root of the notebook.
	// If this is set, the system operates in "Centralized Mode".
	// If empty, it operates in "Local Mode".
	RootDir string `yaml:"root_dir"`

	// Path templates for customizing directory structure in Centralized Mode.
	// These are optional and have sensible defaults.
	NotesPathTemplate      string `yaml:"notes_path_template,omitempty"`
	PlansPathTemplate      string `yaml:"plans_path_template,omitempty"`
	ChatsPathTemplate      string `yaml:"chats_path_template,omitempty"`
	TemplatesPathTemplate  string `yaml:"templates_path_template,omitempty"`
	RecipesPathTemplate    string `yaml:"recipes_path_template,omitempty"`
	InProgressPathTemplate string `yaml:"in_progress_path_template,omitempty"`
	CompletedPathTemplate  string `yaml:"completed_path_template,omitempty"`
	PromptsPathTemplate    string `yaml:"prompts_path_template,omitempty"`

	// Types defines a map of user-configurable note types.
	// This will override the hardcoded defaults in grove-notebook if provided.
	Types map[string]*NoteTypeConfig `yaml:"types,omitempty"`

	// Sync defines the synchronization configuration for this notebook.
	// This is a list of sync provider configurations.
	Sync interface{} `yaml:"sync,omitempty"`
}

// Config represents the grove.yml configuration
type Config struct {
	Name       string   `yaml:"name,omitempty"`
	Version    string   `yaml:"version"`
	Workspaces []string `yaml:"workspaces,omitempty"`
	BuildCmd   string   `yaml:"build_cmd,omitempty"`
	BuildAfter []string `yaml:"build_after,omitempty"`

	// Notebooks contains all notebook-related configuration.
	Notebooks *NotebooksConfig `yaml:"notebooks,omitempty"`

	// TUI contains TUI-specific configuration.
	TUI *TUIConfig `yaml:"tui,omitempty"`

	// Context contains configuration for the grove-context (cx) tool.
	Context *ContextConfig `yaml:"context,omitempty"`

	// Hooks contains configuration for repository lifecycle hooks.
	Hooks *HooksConfig `yaml:"hooks,omitempty"`

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
		BuildCmd         string                       `yaml:"build_cmd,omitempty"`
		BuildAfter       []string                     `yaml:"build_after,omitempty"`
		Notebooks        *NotebooksConfig             `yaml:"notebooks,omitempty"`
		TUI              *TUIConfig                   `yaml:"tui,omitempty"`
		Context          *ContextConfig               `yaml:"context,omitempty"`
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
	c.BuildCmd = raw.BuildCmd
	c.BuildAfter = raw.BuildAfter
	c.TUI = raw.TUI
	c.Context = raw.Context
	c.ExplicitProjects = raw.ExplicitProjects
	c.Extensions = raw.Extensions

	// Handle backward compatibility for `search_paths` -> `groves`
	if len(raw.Groves) > 0 {
		c.Groves = raw.Groves
	} else if len(raw.SearchPaths) > 0 {
		// Migrate old `search_paths` key to new `groves`
		c.Groves = make(map[string]GroveSourceConfig)
		for k, v := range raw.SearchPaths {
			var enabledPtr *bool
			if v.Enabled {
				trueVal := true
				enabledPtr = &trueVal
			} else {
				falseVal := false
				enabledPtr = &falseVal
			}
			c.Groves[k] = GroveSourceConfig{
				Path:        v.Path,
				Enabled:     enabledPtr,
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

	// Set default Enabled=true for all grove sources that don't explicitly set it
	for key, grove := range c.Groves {
		if grove.Enabled == nil {
			trueVal := true
			grove.Enabled = &trueVal
			c.Groves[key] = grove
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
	SourceDefault        ConfigSource = "default"
	SourceGlobal         ConfigSource = "global"
	SourceGlobalOverride ConfigSource = "global-override"
	SourceEnvOverlay     ConfigSource = "env-overlay" // GROVE_CONFIG_OVERLAY
	SourceEcosystem      ConfigSource = "ecosystem"
	SourceProject        ConfigSource = "project"
	SourceOverride       ConfigSource = "override"
	SourceUnknown        ConfigSource = "unknown"
)

// OverrideSource holds a raw configuration from an override file and its path.
type OverrideSource struct {
	Path   string
	Config *Config
}

// LayeredConfig holds the raw configuration from each source file,
// as well as the final merged configuration, for analysis purposes.
type LayeredConfig struct {
	Default        *Config                 // Config with only default values applied.
	Global         *Config                 // Raw config from the global file.
	GlobalOverride *OverrideSource         // Raw config from the global override file.
	EnvOverlay     *OverrideSource         // Raw config from GROVE_CONFIG_OVERLAY env var.
	Ecosystem      *Config                 // Raw config from the ecosystem file (if workspace is in an ecosystem).
	Project        *Config                 // Raw config from the project file.
	Overrides      []OverrideSource        // Raw configs from override files, in order of application.
	Final          *Config                 // The fully merged and validated config.
	FilePaths      map[ConfigSource]string // Maps sources to their file paths.
}
