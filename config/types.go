package config

import (
	"fmt"
	"reflect"

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
	Path         string   `yaml:"path" toml:"path" jsonschema:"description=Absolute path to the grove root directory" jsonschema_extras:"x-priority=1,x-important=true"`
	Enabled      *bool    `yaml:"enabled,omitempty" toml:"enabled,omitempty" jsonschema:"description=Whether this grove is enabled (default: true)" jsonschema_extras:"x-priority=2,x-important=true"`
	Description  string   `yaml:"description,omitempty" toml:"description,omitempty" jsonschema:"description=Human-readable description of this grove" jsonschema_extras:"x-priority=4,x-important=true"`
	Notebook     string   `yaml:"notebook,omitempty" toml:"notebook,omitempty" jsonschema:"description=Name of the notebook to use for projects in this grove" jsonschema_extras:"x-priority=3,x-important=true"`
	Depth        *int     `yaml:"depth,omitempty" toml:"depth,omitempty" jsonschema:"description=How many directory levels deep to scan for projects. Unset keeps current behavior; 1 means immediate children only."`
	IncludeRepos []string `yaml:"include_repos,omitempty" toml:"include_repos,omitempty" jsonschema:"description=List of directory names or relative paths to explicitly include as projects"`
	ExcludeRepos []string `yaml:"exclude_repos,omitempty" toml:"exclude_repos,omitempty" jsonschema:"description=List of directory names or relative paths to explicitly exclude"`
	Memory       *bool    `yaml:"memory,omitempty" toml:"memory,omitempty" jsonschema:"description=Whether to index this grove's notebook content into the memory store for semantic search (default: false)"`
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

	// LeaderKey is the bubbletea key string that activates the leader
	// chord (e.g. "ctrl+b", "f12"). Default: "ctrl+b".
	LeaderKey string `yaml:"leader_key,omitempty" toml:"leader_key,omitempty" jsonschema:"description=Key chord that activates the leader/workspace switcher (bubbletea key string),default=ctrl+b" jsonschema_extras:"x-layer=global,x-priority=53"`

	// ActionKey is the bubbletea key string that activates the action
	// chord for grove-specific terminal actions (sidebar, rail, agent,
	// help, etc.). Default: "ctrl+g".
	ActionKey string `yaml:"action_key,omitempty" toml:"action_key,omitempty" jsonschema:"description=Key chord that activates grove terminal actions (bubbletea key string),default=ctrl+g" jsonschema_extras:"x-layer=global,x-priority=53"`

	// SidebarExpanded controls whether the icon rail starts expanded
	// (showing labels) or collapsed (icons only). Default: false.
	SidebarExpanded bool `yaml:"sidebar_expanded,omitempty" toml:"sidebar_expanded,omitempty" jsonschema:"description=Start terminal sidebar expanded (icon + label) instead of icon-only,default=false" jsonschema_extras:"x-layer=global,x-priority=57"`

	// Shortcuts maps key chords to deep-link navigation targets.
	// Each value uses the syntax "navigate:<panel>[.<tab>]", e.g.
	// "navigate:context.stats" or "navigate:flow". Parsed by the
	// terminal host to emit embed.NavigateMsg on keypress.
	Shortcuts map[string]string `yaml:"shortcuts,omitempty" toml:"shortcuts,omitempty" jsonschema:"description=Global shortcut key → navigate:panel.tab mappings for deep-link navigation" jsonschema_extras:"x-layer=global,x-priority=56"`

	// Panels defines user-configurable ephemeral panel keybindings.
	// Each binding spawns a command in a PTY panel on keypress.
	Panels *PanelConfig `yaml:"panels,omitempty" toml:"panels,omitempty" jsonschema:"description=User-defined ephemeral panel keybindings" jsonschema_extras:"x-layer=global,x-priority=58"`

	// VimControlHjklPaneNav enables vim-tmux-navigator-style pane
	// navigation via Ctrl+h/j/k/l. When enabled, these keys navigate
	// between panes unless the active PTY's foreground process is an
	// editor (nvim, vim, hx) or a TUI (fzf, lazygit, less), in which
	// case the key is passed through to the PTY. Default: false.
	VimControlHjklPaneNav bool `yaml:"vim_control_hjkl_pane_nav,omitempty" toml:"vim_control_hjkl_pane_nav,omitempty" jsonschema:"description=Enable Ctrl+hjkl pane navigation (vim-tmux-navigator style),default=false" jsonschema_extras:"x-layer=global,x-priority=59"`

	// Plugins defines process-based plugin panels that run standalone
	// executables in PTY panels with their own rail icons.
	Plugins map[string]*PluginConfig `yaml:"plugins,omitempty" toml:"plugins,omitempty" jsonschema:"description=Process-based plugin panels" jsonschema_extras:"x-layer=global,x-priority=60"`

	// Focus configures the BSP pane focus indicator system.
	Focus *FocusConfig `yaml:"focus,omitempty" toml:"focus,omitempty" jsonschema:"description=BSP pane focus indicator configuration" jsonschema_extras:"x-layer=global,x-priority=61"`
}

// FocusConfig controls how the focused BSP pane is visually distinguished.
type FocusConfig struct {
	// Style selects the focus indicator strategy: border (highlight
	// separator cells adjacent to focused pane), gutter (1-col colored
	// bar on left edge), or title (1-row colored header).
	Style string `yaml:"style,omitempty" toml:"style,omitempty" jsonschema:"description=Focus indicator style,enum=border,enum=gutter,enum=title,default=border"`
	// ActiveColor is the color used for the focused pane's indicator.
	ActiveColor string `yaml:"active_color,omitempty" toml:"active_color,omitempty" jsonschema:"description=Color for focused pane indicator,default=cyan"`
	// InactiveColor is the color used for unfocused pane indicators.
	InactiveColor string `yaml:"inactive_color,omitempty" toml:"inactive_color,omitempty" jsonschema:"description=Color for unfocused pane indicator,default=gray"`
	// Thickness controls the width (for gutter) or height (for title) of the
	// focus indicator in cells. Defaults to 1. For border style this is ignored.
	Thickness int `yaml:"thickness,omitempty" toml:"thickness,omitempty" jsonschema:"description=Indicator thickness in cells,default=1,minimum=1,maximum=4"`
	// DimInactive dims unfocused panes (requires compositor support).
	DimInactive bool `yaml:"dim_inactive,omitempty" toml:"dim_inactive,omitempty" jsonschema:"description=Dim unfocused panes (requires compositor support)"`
}

// PluginConfig defines a process-based plugin that runs in its own PTY panel.
type PluginConfig struct {
	// Command is the executable to run.
	Command string `yaml:"command" toml:"command" jsonschema:"description=Executable command to run"`
	// Args are optional arguments passed to the command.
	Args []string `yaml:"args,omitempty" toml:"args,omitempty" jsonschema:"description=Arguments passed to the command"`
	// Icon is the nerd font icon displayed in the rail.
	Icon string `yaml:"icon,omitempty" toml:"icon,omitempty" jsonschema:"description=Nerd font icon for the rail"`
	// Position controls where the plugin appears: rail (persistent) or ephemeral (on-demand).
	Position string `yaml:"position,omitempty" toml:"position,omitempty" jsonschema:"description=Panel position: rail (persistent) or ephemeral (on-demand),enum=rail,enum=ephemeral,default=rail"`
	// Cwd is the working directory for the command.
	Cwd string `yaml:"cwd,omitempty" toml:"cwd,omitempty" jsonschema:"description=Working directory for the command"`
	// Env are extra environment variables (KEY=VALUE format).
	Env []string `yaml:"env,omitempty" toml:"env,omitempty" jsonschema:"description=Extra environment variables (KEY=VALUE)"`
	// Restart controls whether the plugin auto-restarts on exit.
	Restart bool `yaml:"restart,omitempty" toml:"restart,omitempty" jsonschema:"description=Auto-restart plugin on exit,default=false"`
}

// PanelConfig holds configuration for user-defined ephemeral panel
// keybindings. Command is the default binary; Bindings is a named
// map of keybindings that each spawn a panel.
type PanelConfig struct {
	// Command is the default binary to run. Defaults to $EDITOR or "vi".
	Command  string                        `yaml:"command,omitempty" toml:"command,omitempty" jsonschema:"description=Default command binary (falls back to $EDITOR or vi)"`
	Bindings map[string]PanelBindingConfig `yaml:"bindings,omitempty" toml:"bindings,omitempty" jsonschema:"description=Named panel keybindings"`
}

// PanelBindingConfig defines a single ephemeral panel keybinding.
type PanelBindingConfig struct {
	// Key is the keychord string (e.g. "ctrl+e", "alt+x").
	Key string `yaml:"key,omitempty" toml:"key,omitempty" jsonschema:"description=Key chord that triggers this panel"`
	// Label is the display text in the panel header and icon rail.
	Label string `yaml:"label,omitempty" toml:"label,omitempty" jsonschema:"description=Display label for header and sidebar"`
	// Command overrides the top-level default binary for this binding.
	Command string `yaml:"command,omitempty" toml:"command,omitempty" jsonschema:"description=Command binary override for this binding"`
	// Args are static arguments passed to the command.
	Args []string `yaml:"args,omitempty" toml:"args,omitempty" jsonschema:"description=Static arguments passed to the command"`
	// ArgsCommand is a shell command whose stdout is trimmed and appended
	// as a single argument. Runs asynchronously before spawning the panel.
	ArgsCommand string `yaml:"args_command,omitempty" toml:"args_command,omitempty" jsonschema:"description=Shell command whose stdout becomes an extra argument"`
}

// ContextConfig holds configuration for the grove-context (cx) tool.
type ContextConfig struct {
	ReposDir         *string `yaml:"repos_dir,omitempty" toml:"repos_dir,omitempty" jsonschema:"description=Directory where cx repo stores bare repositories (default: ~/.grove/cx)" jsonschema_extras:"x-layer=global,x-priority=80"`
	DefaultRulesPath string  `yaml:"default_rules_path,omitempty" toml:"default_rules_path,omitempty" jsonschema:"description=Default rules file path for context filtering" jsonschema_extras:"x-layer=project,x-priority=81"`
	DefaultRules     string  `yaml:"default_rules,omitempty" toml:"default_rules,omitempty" jsonschema:"description=Name of the default rules preset to use" jsonschema_extras:"x-layer=project,x-priority=82"`
	// IncludedWorkspaces is a strict allowlist: if set, only these workspaces are scanned for context.
	IncludedWorkspaces []string `yaml:"included_workspaces,omitempty" toml:"included_workspaces,omitempty" jsonschema:"description=Allowlist of workspace names to include in context scanning" jsonschema_extras:"x-layer=project,x-priority=83"`
	// ExcludedWorkspaces is a denylist: these workspaces are excluded from context scanning.
	ExcludedWorkspaces []string `yaml:"excluded_workspaces,omitempty" toml:"excluded_workspaces,omitempty" jsonschema:"description=Denylist of workspace names to exclude from context scanning" jsonschema_extras:"x-layer=project,x-priority=84"`
	// AllowedPaths is a list of additional paths that can be included in context,
	// regardless of workspace boundaries.
	AllowedPaths []string `yaml:"allowed_paths,omitempty" toml:"allowed_paths,omitempty" jsonschema:"description=Additional paths allowed for context inclusion regardless of workspace boundaries" jsonschema_extras:"x-layer=project,x-priority=85"`
}

// DaemonJobsConfig holds configuration for the in-process job runner.
type DaemonJobsConfig struct {
	Enabled          *bool  `yaml:"enabled,omitempty" toml:"enabled,omitempty" jsonschema:"description=Enable the background job runner (default: true)"`
	MaxConcurrent    int    `yaml:"max_concurrent,omitempty" toml:"max_concurrent,omitempty" jsonschema:"description=Maximum number of concurrent jobs (default: 4)"`
	DefaultTimeout   string `yaml:"default_timeout,omitempty" toml:"default_timeout,omitempty" jsonschema:"description=Default timeout for jobs (default: 30m)"`
	QueuePersistence *bool  `yaml:"queue_persistence,omitempty" toml:"queue_persistence,omitempty" jsonschema:"description=Persist job queue across daemon restarts (default: true)"`
	PersistDir       string `yaml:"persist_dir,omitempty" toml:"persist_dir,omitempty" jsonschema:"description=Directory to persist job state"`
}

// EnvironmentConfig holds configuration for the dev environment provider.
type EnvironmentConfig struct {
	Provider         string                 `yaml:"provider,omitempty" toml:"provider,omitempty" json:"provider,omitempty" jsonschema:"description=Provider type (native\\, docker\\, cloud\\, or custom exec plugin name)"`
	Command          string                 `yaml:"command,omitempty" toml:"command,omitempty" json:"command,omitempty" jsonschema:"description=Path to provider binary (exec plugins only). If empty\\, searches PATH for grove-env-<provider>."`
	Config           map[string]interface{} `yaml:"config,omitempty" toml:"config,omitempty" json:"config,omitempty" jsonschema:"description=Provider-specific configuration"`
	Commands         map[string]string      `yaml:"commands,omitempty" toml:"commands,omitempty" json:"commands,omitempty" jsonschema:"description=Named commands that run in the context of this environment"`
	DisplayEndpoints []string               `yaml:"display_endpoints,omitempty" toml:"display_endpoints,omitempty" json:"display_endpoints,omitempty" jsonschema:"description=Env var names whose values should surface as endpoints in the TUI. If unset\\, any http(s) value is treated as an endpoint."`
	DisplayResources []string               `yaml:"display_resources,omitempty" toml:"display_resources,omitempty" json:"display_resources,omitempty" jsonschema:"description=Human-readable resource labels shown on the Shared Infra page (e.g. 'Cloud SQL (myproject:us-central1:db)'). Purely cosmetic; no schema constraint."`
	Shared           *bool                  `yaml:"shared,omitempty" toml:"shared,omitempty" json:"shared,omitempty" jsonschema:"description=Whether this profile represents shared ecosystem infrastructure consumed by other profiles via shared_env."`
}

// DaemonConfig holds configuration for the grove daemon (groved).
type DaemonConfig struct {
	GitInterval         string            `yaml:"git_interval,omitempty" toml:"git_interval,omitempty" jsonschema:"description=How often to poll git status (default: 10s)"`
	SessionInterval     string            `yaml:"session_interval,omitempty" toml:"session_interval,omitempty" jsonschema:"description=How often to poll sessions (default: 2s)"`
	WorkspaceInterval   string            `yaml:"workspace_interval,omitempty" toml:"workspace_interval,omitempty" jsonschema:"description=How often to refresh workspace discovery (default: 30s)"`
	PlanInterval        string            `yaml:"plan_interval,omitempty" toml:"plan_interval,omitempty" jsonschema:"description=How often to poll plan stats (default: 30s)"`
	NoteInterval        string            `yaml:"note_interval,omitempty" toml:"note_interval,omitempty" jsonschema:"description=How often to poll note counts (default: 60s)"`
	ConfigWatch         *bool             `yaml:"config_watch,omitempty" toml:"config_watch,omitempty" jsonschema:"description=Enable config watching (default: true)"`
	ConfigDebounceMs    int               `yaml:"config_debounce_ms,omitempty" toml:"config_debounce_ms,omitempty" jsonschema:"description=Debounce window for rapid config changes in milliseconds (default: 100)"`
	AutoSyncSkills      *bool             `yaml:"auto_sync_skills,omitempty" toml:"auto_sync_skills,omitempty" jsonschema:"description=Enable automatic syncing of skills on file change (default: true)"`
	SkillSyncDebounceMs int               `yaml:"skill_sync_debounce_ms,omitempty" toml:"skill_sync_debounce_ms,omitempty" jsonschema:"description=Debounce window for skill syncs in milliseconds (default: 1000)"`
	Hooks               *DaemonHooks      `yaml:"hooks,omitempty" toml:"hooks,omitempty" jsonschema:"description=Daemon-specific hooks configuration"`
	Jobs                *DaemonJobsConfig `yaml:"jobs,omitempty" toml:"jobs,omitempty" jsonschema:"description=Job runner configuration"`
	SSH                 *DaemonSSHConfig  `yaml:"ssh,omitempty" toml:"ssh,omitempty" jsonschema:"description=Embedded SSH server configuration"`
	PairWithTreemux     *bool             `yaml:"pair_with_treemux,omitempty" toml:"pair_with_treemux,omitempty" jsonschema:"description=Opt-in to kill daemon when the parent treemux exits"`
}

// DaemonSSHConfig holds configuration for the embedded SSH server.
type DaemonSSHConfig struct {
	Enabled     *bool  `yaml:"enabled,omitempty" toml:"enabled,omitempty" jsonschema:"description=Enable the embedded SSH server (default: false)"`
	Port        int    `yaml:"port,omitempty" toml:"port,omitempty" jsonschema:"description=Port to listen on (default: 2222)"`
	HostKeyPath string `yaml:"host_key_path,omitempty" toml:"host_key_path,omitempty" jsonschema:"description=Path to the SSH host key (default: ~/.local/state/grove/ssh_host_key)"`
}

// DaemonHooks defines hooks that are triggered by daemon events.
type DaemonHooks struct {
	OnSkillSync []HookCommand `yaml:"on_skill_sync,omitempty" toml:"on_skill_sync,omitempty" jsonschema:"description=Commands to run after skills are synced for a workspace"`
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

// SyncthingConfig holds settings for automated Syncthing folder setup.
type SyncthingConfig struct {
	Devices     []string `yaml:"devices,omitempty" toml:"devices,omitempty" jsonschema:"description=Syncthing device IDs to share this notebook with" jsonschema_extras:"x-layer=global,x-priority=40,x-important=true"`
	FolderTitle string   `yaml:"folder_title,omitempty" toml:"folder_title,omitempty" jsonschema:"description=Custom title for the Syncthing folder (defaults to grove-<notebook>)" jsonschema_extras:"x-layer=global,x-priority=41"`
}

// ObsidianConfig holds settings for automated Obsidian vault setup.
type ObsidianConfig struct {
	VaultName      string `yaml:"vault_name,omitempty" toml:"vault_name,omitempty" jsonschema:"description=Display name for the generated Obsidian vault" jsonschema_extras:"x-layer=global,x-priority=45"`
	AutoLinkPlugin bool   `yaml:"auto_link_plugin,omitempty" toml:"auto_link_plugin,omitempty" jsonschema:"description=Automatically symlink the nb-integration plugin on setup,default=false" jsonschema_extras:"x-layer=global,x-priority=46"`
	TemplateRepo   string `yaml:"template_repo,omitempty" toml:"template_repo,omitempty" jsonschema:"description=Git repo URL containing .obsidian template (e.g. github.com/user/obsidian-dotfiles)" jsonschema_extras:"x-layer=global,x-priority=47"`
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
	ContextPathTemplate    string                     `yaml:"context_path_template,omitempty" toml:"context_path_template,omitempty" jsonschema:"description=Path template for context directory"`
	Types                  map[string]*NoteTypeConfig `yaml:"types,omitempty" toml:"types,omitempty" jsonschema:"description=Map of note type name to configuration"`
	Sync                   interface{}                `yaml:"sync,omitempty" toml:"sync,omitempty" jsonschema:"description=Synchronization configuration for this notebook"`
	Syncthing              *SyncthingConfig           `yaml:"syncthing,omitempty" toml:"syncthing,omitempty" jsonschema:"description=Syncthing automated setup configuration"`
	Obsidian               *ObsidianConfig            `yaml:"obsidian,omitempty" toml:"obsidian,omitempty" jsonschema:"description=Obsidian vault automated setup configuration"`
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

	Environment  *EnvironmentConfig            `yaml:"environment,omitempty" toml:"environment,omitempty" jsonschema:"description=Development environment provider configuration"`
	Environments map[string]*EnvironmentConfig `yaml:"environments,omitempty" toml:"environments,omitempty" jsonschema:"description=Named environment profiles selected via --env flag"`

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
		Environment      *EnvironmentConfig            `yaml:"environment,omitempty"`
		Environments     map[string]*EnvironmentConfig `yaml:"environments,omitempty"`
		Groves           map[string]GroveSourceConfig  `yaml:"groves,omitempty"`
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
	c.Environment = raw.Environment
	c.Environments = raw.Environments
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
		Result:     target,
		TagName:    "yaml",
		DecodeHook: stringToPathStructHook(),
	})
	if err != nil {
		return fmt.Errorf("failed to create mapstructure decoder: %w", err)
	}

	if err := decoder.Decode(extensionConfig); err != nil {
		return fmt.Errorf("failed to decode extension config for '%s': %w", key, err)
	}

	return nil
}

// stringToPathStructHook returns a DecodeHookFunc that converts strings to structs
// with a single "path" or "Path" field. This enables shorthand config syntax like:
//
//	[nav.groups.personal.sessions]
//	o = "/path/to/dir"
//
// Instead of the verbose:
//
//	[nav.groups.personal.sessions.o]
//	path = "/path/to/dir"
func stringToPathStructHook() mapstructure.DecodeHookFunc {
	return func(from reflect.Type, to reflect.Type, data interface{}) (interface{}, error) {
		// Only handle string -> struct conversions
		if from.Kind() != reflect.String || to.Kind() != reflect.Struct {
			return data, nil
		}

		// Check if target struct has a "Path" field
		pathField, hasPath := to.FieldByName("Path")
		if !hasPath || pathField.Type.Kind() != reflect.String {
			return data, nil
		}

		// Create a new instance of the target struct and set the Path field
		result := reflect.New(to).Elem()
		result.FieldByName("Path").SetString(data.(string))
		return result.Interface(), nil
	}
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
	SourceProjectNotebook ConfigSource = "project-notebook"
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
	ProjectNotebook *Config                 // Raw config from the project's notebook directory.
	Project         *Config                 // Raw config from the local project file.
	Overrides       []OverrideSource        // Raw configs from override files, in order of application.
	Final           *Config                 // The fully merged and validated config.
	FilePaths       map[ConfigSource]string // Maps sources to their file paths.
}
