package config

import (
	"fmt"

	"github.com/mitchellh/mapstructure"
	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

//go:generate go run ../tools/schema-generator/
//go:generate sh -c "cd .. && go run ./tools/schema-composer/"
//go:generate sh -c "cd .. && go run ./tools/notebook-schema-generator/"


// SearchPathConfig defines the configuration for a single search path.
// DEPRECATED: Use GroveSourceConfig instead.
type SearchPathConfig struct {
	Path        string `yaml:"path" toml:"path"`
	Enabled     bool   `yaml:"enabled" toml:"enabled"`
	Description string `yaml:"description,omitempty" toml:"description,omitempty"`
}

// GroveSourceConfig defines the configuration for a single grove source.
type GroveSourceConfig struct {
	Path        string `yaml:"path" toml:"path" jsonschema:"description=Absolute path to the grove root directory" jsonschema_extras:"x-priority=1,x-important=true"`
	Enabled     *bool  `yaml:"enabled,omitempty" toml:"enabled,omitempty" jsonschema:"description=Whether this grove is enabled (default: true)" jsonschema_extras:"x-priority=2,x-important=true"`
	Description string `yaml:"description,omitempty" toml:"description,omitempty" jsonschema:"description=Human-readable description of this grove" jsonschema_extras:"x-priority=4,x-important=true"`
	Notebook    string `yaml:"notebook,omitempty" toml:"notebook,omitempty" jsonschema:"description=Name of the notebook to use for projects in this grove" jsonschema_extras:"x-priority=3,x-important=true"`
}

// ExplicitProject defines a specific project to include regardless of discovery.
type ExplicitProject struct {
	Path        string `yaml:"path" toml:"path" jsonschema:"description=Absolute path to the project directory"`
	Name        string `yaml:"name,omitempty" toml:"name,omitempty" jsonschema:"description=Display name for the project"`
	Description string `yaml:"description,omitempty" toml:"description,omitempty" jsonschema:"description=Human-readable description of this project"`
	Enabled     bool   `yaml:"enabled" toml:"enabled" jsonschema:"description=Whether this project is enabled"`
}

// NoteTypeConfig defines the configuration for a single, user-defined note type.
type NoteTypeConfig struct {
	Description    string `yaml:"description,omitempty" toml:"description,omitempty" jsonschema:"description=Human-readable description of this note type"`
	TemplatePath   string `yaml:"template_path,omitempty" toml:"template_path,omitempty" jsonschema:"description=Path to the template file for this note type"`
	FilenameFormat string `yaml:"filename_format,omitempty" toml:"filename_format,omitempty" jsonschema:"description=Filename format: date-title, timestamp-title, or title"`
	Icon           string `yaml:"icon,omitempty" toml:"icon,omitempty" jsonschema:"description=Icon for TUI display (nerd font icon)"`
	IconColor      string `yaml:"icon_color,omitempty" toml:"icon_color,omitempty" jsonschema:"description=Lipgloss color for the icon in the TUI"`
	DefaultExpand  bool   `yaml:"default_expand,omitempty" toml:"default_expand,omitempty" jsonschema:"description=Whether this group is expanded by default in the TUI"`
	SortOrder      int    `yaml:"sort_order,omitempty" toml:"sort_order,omitempty" jsonschema:"description=Sort order in the TUI (lower numbers appear first)"`
}

// GlobalNotebookConfig defines the configuration for the system-wide global notebook.
type GlobalNotebookConfig struct {
	RootDir string `yaml:"root_dir" toml:"root_dir" jsonschema:"description=Absolute path to the global notebook root directory"`
}

// NotebookRules defines the usage rules for notebooks.
type NotebookRules struct {
	Default string                `yaml:"default,omitempty" toml:"default,omitempty" jsonschema:"description=Name of the default notebook to use"`
	Global  *GlobalNotebookConfig `yaml:"global,omitempty" toml:"global,omitempty" jsonschema:"description=Configuration for the system-wide global notebook"`
}

// NotebooksConfig groups all notebook-related settings.
type NotebooksConfig struct {
	Definitions map[string]*Notebook `yaml:"definitions,omitempty" toml:"definitions,omitempty" jsonschema:"description=Map of notebook name to notebook configuration"`
	Rules       *NotebookRules       `yaml:"rules,omitempty" toml:"rules,omitempty" jsonschema:"description=Rules for notebook usage (default notebook, global notebook)"`
}

// NvimEmbedConfig holds settings for the embedded Neovim component.
type NvimEmbedConfig struct {
	UserConfig bool `yaml:"user_config" toml:"user_config" jsonschema:"description=If true, loads the user's default Neovim config (~/.config/nvim)"`
}

// KeybindingSectionConfig defines keybindings for a specific section (navigation, actions, etc.)
// Keys are action names (e.g., "up", "down", "quit"), values are lists of key combinations.
type KeybindingSectionConfig map[string][]string

// KeybindingsConfig defines the structure for custom keybindings.
type KeybindingsConfig struct {
	// Standard sections - apply to all TUIs
	Navigation KeybindingSectionConfig `yaml:"navigation,omitempty" toml:"navigation,omitempty" jsonschema:"description=Navigation keybindings (up, down, left, right, page_up, page_down, top, bottom)"`
	Selection  KeybindingSectionConfig `yaml:"selection,omitempty" toml:"selection,omitempty" jsonschema:"description=Selection keybindings (select, select_all, select_none, toggle_select)"`
	Actions    KeybindingSectionConfig `yaml:"actions,omitempty" toml:"actions,omitempty" jsonschema:"description=Action keybindings (confirm, cancel, back, edit, delete, yank)"`
	Search     KeybindingSectionConfig `yaml:"search,omitempty" toml:"search,omitempty" jsonschema:"description=Search keybindings (search, next_match, prev_match, clear_search, grep)"`
	View       KeybindingSectionConfig `yaml:"view,omitempty" toml:"view,omitempty" jsonschema:"description=View keybindings (switch_view, next_tab, prev_tab, toggle_preview)"`
	Fold       KeybindingSectionConfig `yaml:"fold,omitempty" toml:"fold,omitempty" jsonschema:"description=Fold keybindings (open, close, toggle, open_all, close_all)"`
	System     KeybindingSectionConfig `yaml:"system,omitempty" toml:"system,omitempty" jsonschema:"description=System keybindings (quit, help, refresh)"`

	// Per-TUI overrides - nested by package then TUI name
	// e.g., TUIOverrides["nb"]["browser"]["create_note"] = ["n"]
	// Config path: [tui.keybindings.nb.browser]
	TUIOverrides map[string]map[string]KeybindingSectionConfig `yaml:"-" toml:"-" jsonschema:"-"`

	// Overrides is kept for backward compatibility with old config format
	// [tui.keybindings.overrides.flow.status] -> migrated to TUIOverrides
	Overrides map[string]map[string]KeybindingSectionConfig `yaml:"overrides,omitempty" toml:"overrides,omitempty" jsonschema:"-"`
}

// GetTUIOverrides returns the per-TUI keybinding overrides, checking both
// the new TUIOverrides field and the legacy Overrides field for backward compatibility.
func (k *KeybindingsConfig) GetTUIOverrides() map[string]map[string]KeybindingSectionConfig {
	// Prefer TUIOverrides (new format) if populated
	if len(k.TUIOverrides) > 0 {
		return k.TUIOverrides
	}
	// Fall back to Overrides (old format) for backward compatibility
	return k.Overrides
}

// keybindingsSectionNames lists the reserved section names that apply globally.
var keybindingsSectionNames = map[string]bool{
	"navigation": true,
	"selection":  true,
	"actions":    true,
	"search":     true,
	"view":       true,
	"fold":       true,
	"system":     true,
}

// UnmarshalYAML implements custom YAML unmarshaling for KeybindingsConfig.
// Any key that's not a known section name is treated as a package name for per-TUI overrides.
func (k *KeybindingsConfig) UnmarshalYAML(node *yaml.Node) error {
	// First, decode into a map to get all keys
	var raw map[string]yaml.Node
	if err := node.Decode(&raw); err != nil {
		return err
	}

	// Process known sections
	if navNode, ok := raw["navigation"]; ok {
		if err := navNode.Decode(&k.Navigation); err != nil {
			return fmt.Errorf("failed to decode navigation: %w", err)
		}
	}
	if selNode, ok := raw["selection"]; ok {
		if err := selNode.Decode(&k.Selection); err != nil {
			return fmt.Errorf("failed to decode selection: %w", err)
		}
	}
	if actNode, ok := raw["actions"]; ok {
		if err := actNode.Decode(&k.Actions); err != nil {
			return fmt.Errorf("failed to decode actions: %w", err)
		}
	}
	if searchNode, ok := raw["search"]; ok {
		if err := searchNode.Decode(&k.Search); err != nil {
			return fmt.Errorf("failed to decode search: %w", err)
		}
	}
	if viewNode, ok := raw["view"]; ok {
		if err := viewNode.Decode(&k.View); err != nil {
			return fmt.Errorf("failed to decode view: %w", err)
		}
	}
	if foldNode, ok := raw["fold"]; ok {
		if err := foldNode.Decode(&k.Fold); err != nil {
			return fmt.Errorf("failed to decode fold: %w", err)
		}
	}
	if sysNode, ok := raw["system"]; ok {
		if err := sysNode.Decode(&k.System); err != nil {
			return fmt.Errorf("failed to decode system: %w", err)
		}
	}

	// Process unknown keys as package names (per-TUI overrides)
	for key, valueNode := range raw {
		if keybindingsSectionNames[key] {
			continue // Already processed
		}

		// This is a package name - decode its TUI map
		var tuiMap map[string]KeybindingSectionConfig
		if err := valueNode.Decode(&tuiMap); err != nil {
			return fmt.Errorf("failed to decode TUI overrides for package %q: %w", key, err)
		}

		if k.TUIOverrides == nil {
			k.TUIOverrides = make(map[string]map[string]KeybindingSectionConfig)
		}
		k.TUIOverrides[key] = tuiMap
	}

	return nil
}

// UnmarshalTOML implements custom TOML unmarshaling for KeybindingsConfig.
// Any key that's not a known section name is treated as a package name for per-TUI overrides.
func (k *KeybindingsConfig) UnmarshalTOML(data []byte) error {
	// First, decode into a map to get all keys
	var raw map[string]interface{}
	if err := toml.Unmarshal(data, &raw); err != nil {
		return err
	}

	// Helper to decode a section
	decodeSection := func(key string, target *KeybindingSectionConfig) error {
		if v, ok := raw[key]; ok {
			if m, ok := v.(map[string]interface{}); ok {
				*target = make(KeybindingSectionConfig)
				for action, keys := range m {
					if arr, ok := keys.([]interface{}); ok {
						var strKeys []string
						for _, k := range arr {
							if s, ok := k.(string); ok {
								strKeys = append(strKeys, s)
							}
						}
						(*target)[action] = strKeys
					}
				}
			}
		}
		return nil
	}

	// Process known sections
	decodeSection("navigation", &k.Navigation)
	decodeSection("selection", &k.Selection)
	decodeSection("actions", &k.Actions)
	decodeSection("search", &k.Search)
	decodeSection("view", &k.View)
	decodeSection("fold", &k.Fold)
	decodeSection("system", &k.System)

	// Process unknown keys as package names (per-TUI overrides)
	for key, value := range raw {
		if keybindingsSectionNames[key] {
			continue // Already processed
		}

		// This is a package name - decode its TUI map
		pkgMap, ok := value.(map[string]interface{})
		if !ok {
			continue
		}

		if k.TUIOverrides == nil {
			k.TUIOverrides = make(map[string]map[string]KeybindingSectionConfig)
		}

		k.TUIOverrides[key] = make(map[string]KeybindingSectionConfig)
		for tuiName, tuiValue := range pkgMap {
			tuiMap, ok := tuiValue.(map[string]interface{})
			if !ok {
				continue
			}

			k.TUIOverrides[key][tuiName] = make(KeybindingSectionConfig)
			for action, keys := range tuiMap {
				if arr, ok := keys.([]interface{}); ok {
					var strKeys []string
					for _, kv := range arr {
						if s, ok := kv.(string); ok {
							strKeys = append(strKeys, s)
						}
					}
					k.TUIOverrides[key][tuiName][action] = strKeys
				}
			}
		}
	}

	return nil
}

// TUIConfig holds TUI-specific settings.
type TUIConfig struct {
	Icons       string             `yaml:"icons,omitempty" toml:"icons,omitempty" jsonschema:"description=Icon set to use: nerd or ascii,enum=nerd,enum=ascii" jsonschema_extras:"x-layer=global,x-priority=52,x-important=true"`
	Theme       string             `yaml:"theme,omitempty" toml:"theme,omitempty" jsonschema:"description=Color theme for terminal interfaces,enum=kanagawa,enum=gruvbox,enum=terminal" jsonschema_extras:"x-layer=global,x-priority=51,x-important=true"`
	Preset      string             `yaml:"preset,omitempty" toml:"preset,omitempty" jsonschema:"description=Keybinding preset: vim (default), emacs, or arrows,enum=vim,enum=emacs,enum=arrows,default=vim" jsonschema_extras:"x-layer=global,x-priority=50,x-important=true"`
	Keybindings *KeybindingsConfig `yaml:"keybindings,omitempty" toml:"keybindings,omitempty" jsonschema:"description=Custom keybinding overrides" jsonschema_extras:"x-layer=global,x-priority=54"`
	NvimEmbed   *NvimEmbedConfig   `yaml:"nvim_embed,omitempty" toml:"nvim_embed,omitempty" jsonschema:"description=Embedded Neovim configuration" jsonschema_extras:"x-status=alpha,x-layer=global,x-priority=55"`
}

// ContextConfig holds configuration for the grove-context (cx) tool.
type ContextConfig struct {
	ReposDir         *string `yaml:"repos_dir,omitempty" toml:"repos_dir,omitempty" jsonschema:"description=Directory where cx repo stores bare repositories (default: ~/.grove/cx)" jsonschema_extras:"x-layer=global,x-priority=80"`
	DefaultRulesPath string  `yaml:"default_rules_path,omitempty" toml:"default_rules_path,omitempty" jsonschema:"description=Default rules file path for context filtering" jsonschema_extras:"x-layer=project,x-priority=81"`
}

// DaemonConfig holds configuration for the grove daemon (groved).
type DaemonConfig struct {
	GitInterval       string `yaml:"git_interval,omitempty" toml:"git_interval,omitempty" jsonschema:"description=How often to poll git status (default: 10s)"`
	SessionInterval   string `yaml:"session_interval,omitempty" toml:"session_interval,omitempty" jsonschema:"description=How often to poll sessions (default: 2s)"`
	WorkspaceInterval string `yaml:"workspace_interval,omitempty" toml:"workspace_interval,omitempty" jsonschema:"description=How often to refresh workspace discovery (default: 30s)"`
	PlanInterval      string `yaml:"plan_interval,omitempty" toml:"plan_interval,omitempty" jsonschema:"description=How often to poll plan stats (default: 30s)"`
	NoteInterval      string `yaml:"note_interval,omitempty" toml:"note_interval,omitempty" jsonschema:"description=How often to poll note counts (default: 60s)"`
	ConfigWatch       *bool  `yaml:"config_watch,omitempty" toml:"config_watch,omitempty" jsonschema:"description=Enable config watching (default: true)"`
	ConfigDebounceMs  int    `yaml:"config_debounce_ms,omitempty" toml:"config_debounce_ms,omitempty" jsonschema:"description=Debounce window for rapid config changes in milliseconds (default: 100)"`
}

// HookCommand defines a command to be executed for a hook.
type HookCommand struct {
	Name    string `yaml:"name" toml:"name" jsonschema:"description=Name of the hook command"`
	Command string `yaml:"command" toml:"command" jsonschema:"description=Shell command to execute"`
	RunIf   string `yaml:"run_if,omitempty" toml:"run_if,omitempty" jsonschema:"enum=always,enum=changes,description=Condition to run the command (always or changes)"`
}

// HooksConfig groups all hook-related settings.
type HooksConfig struct {
	OnStop []HookCommand `yaml:"on_stop,omitempty" toml:"on_stop,omitempty" jsonschema:"description=Commands to run when a session stops"`
}

// Notebook defines the configuration for a single, named notebook system.
type Notebook struct {
	RootDir                string                     `yaml:"root_dir" toml:"root_dir" jsonschema:"description=Absolute path to the notebook root (enables Centralized Mode)"`
	NotesPathTemplate      string                     `yaml:"notes_path_template,omitempty" toml:"notes_path_template,omitempty" jsonschema:"description=Path template for notes directory"`
	PlansPathTemplate      string                     `yaml:"plans_path_template,omitempty" toml:"plans_path_template,omitempty" jsonschema:"description=Path template for plans directory"`
	ChatsPathTemplate      string                     `yaml:"chats_path_template,omitempty" toml:"chats_path_template,omitempty" jsonschema:"description=Path template for chats directory"`
	TemplatesPathTemplate  string                     `yaml:"templates_path_template,omitempty" toml:"templates_path_template,omitempty" jsonschema:"description=Path template for templates directory"`
	RecipesPathTemplate    string                     `yaml:"recipes_path_template,omitempty" toml:"recipes_path_template,omitempty" jsonschema:"description=Path template for recipes directory"`
	InProgressPathTemplate string                     `yaml:"in_progress_path_template,omitempty" toml:"in_progress_path_template,omitempty" jsonschema:"description=Path template for in-progress items"`
	CompletedPathTemplate  string                     `yaml:"completed_path_template,omitempty" toml:"completed_path_template,omitempty" jsonschema:"description=Path template for completed items"`
	PromptsPathTemplate    string                     `yaml:"prompts_path_template,omitempty" toml:"prompts_path_template,omitempty" jsonschema:"description=Path template for prompts directory"`
	Types                  map[string]*NoteTypeConfig `yaml:"types,omitempty" toml:"types,omitempty" jsonschema:"description=Map of note type name to configuration"`
	Sync                   interface{}                `yaml:"sync,omitempty" toml:"sync,omitempty" jsonschema:"description=Synchronization configuration for this notebook"`
}

// Config represents the grove.yml configuration
type Config struct {
	Name       string   `yaml:"name,omitempty" toml:"name,omitempty" jsonschema:"description=Name of the project or ecosystem"`
	Version    string   `yaml:"version" toml:"version" jsonschema:"description=Configuration version (e.g. 1.0)"`
	Workspaces []string `yaml:"workspaces,omitempty" toml:"workspaces,omitempty" jsonschema:"description=Glob patterns for workspace directories in this ecosystem"`
	BuildCmd   string   `yaml:"build_cmd,omitempty" toml:"build_cmd,omitempty" jsonschema:"description=Custom build command (default: make build)"`
	BuildAfter []string `yaml:"build_after,omitempty" toml:"build_after,omitempty" jsonschema:"description=Projects that must be built before this one"`

	Notebooks *NotebooksConfig `yaml:"notebooks,omitempty" toml:"notebooks,omitempty" jsonschema:"description=Notebook configuration"`
	TUI       *TUIConfig       `yaml:"tui,omitempty" toml:"tui,omitempty" jsonschema:"description=TUI appearance and behavior settings"`
	Context   *ContextConfig   `yaml:"context,omitempty" toml:"context,omitempty" jsonschema:"description=Configuration for the cx (context) tool"`
	Daemon    *DaemonConfig    `yaml:"daemon,omitempty" toml:"daemon,omitempty" jsonschema:"description=Configuration for the grove daemon (groved)"`

	Groves           map[string]GroveSourceConfig `yaml:"groves,omitempty" toml:"groves,omitempty" jsonschema:"description=Root directories to search for projects and ecosystems"`
	SearchPaths      map[string]SearchPathConfig  `yaml:"search_paths,omitempty" toml:"search_paths,omitempty" jsonschema:"description=DEPRECATED: Use groves instead,deprecated=true" jsonschema_extras:"x-deprecated=true,x-deprecated-message=Use 'groves' for project discovery,x-deprecated-replacement=groves,x-deprecated-version=v0.5.0,x-deprecated-removal=v1.0.0"`
	ExplicitProjects []ExplicitProject            `yaml:"explicit_projects,omitempty" toml:"explicit_projects,omitempty" jsonschema:"description=Specific projects to include without discovery"`

	// Extensions captures all other top-level keys for extensibility.
	Extensions map[string]interface{} `yaml:",inline" toml:"-" jsonschema:"-"`
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
		Daemon           *DaemonConfig                `yaml:"daemon,omitempty"`
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
	c.Daemon = raw.Daemon
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
	SourceDefault         ConfigSource = "default"
	SourceGlobal          ConfigSource = "global"
	SourceGlobalFragment  ConfigSource = "global-fragment"
	SourceGlobalOverride  ConfigSource = "global-override"
	SourceEnvOverlay      ConfigSource = "env-overlay" // GROVE_CONFIG_OVERLAY
	SourceEcosystem       ConfigSource = "ecosystem"
	SourceProject         ConfigSource = "project"
	SourceOverride        ConfigSource = "override"
	SourceUnknown         ConfigSource = "unknown"
)

// OverrideSource holds a raw configuration from an override file and its path.
type OverrideSource struct {
	Path   string
	Config *Config
}

// LayeredConfig holds the raw configuration from each source file,
// as well as the final merged configuration, for analysis purposes.
type LayeredConfig struct {
	Default         *Config                 // Config with only default values applied.
	Global          *Config                 // Raw config from the global file.
	GlobalFragments []OverrideSource        // Raw configs from modular ~/.config/grove/*.toml files.
	GlobalOverride  *OverrideSource         // Raw config from the global override file.
	EnvOverlay      *OverrideSource         // Raw config from GROVE_CONFIG_OVERLAY env var.
	Ecosystem       *Config                 // Raw config from the ecosystem file (if workspace is in an ecosystem).
	Project         *Config                 // Raw config from the project file.
	Overrides       []OverrideSource        // Raw configs from override files, in order of application.
	Final           *Config                 // The fully merged and validated config.
	FilePaths       map[ConfigSource]string // Maps sources to their file paths.
}
