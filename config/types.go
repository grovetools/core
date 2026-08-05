package config

import (
	"fmt"
	"reflect"

	"github.com/mitchellh/mapstructure"
	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

//go:generate sh -c "cd .. && go run ./tools/schema-generator/"
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
	_ = decodeSection("navigation", &k.Navigation)
	_ = decodeSection("selection", &k.Selection)
	_ = decodeSection("actions", &k.Actions)
	_ = decodeSection("search", &k.Search)
	_ = decodeSection("view", &k.View)
	_ = decodeSection("fold", &k.Fold)
	_ = decodeSection("system", &k.System)

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
	Icons string `yaml:"icons,omitempty" toml:"icons,omitempty" jsonschema:"description=Icon set to use: nerd or ascii,enum=nerd,enum=ascii" jsonschema_extras:"x-layer=global,x-priority=52,x-important=true"`
	// Theme's schema enum is generated from the theme registry (see
	// config.GenerateSchemaWithThemeNames and tools/schema-generator); do
	// not hardcode theme names in the tag.
	Theme       string             `yaml:"theme,omitempty" toml:"theme,omitempty" jsonschema:"description=Color theme for terminal interfaces" jsonschema_extras:"x-layer=global,x-priority=51,x-important=true"`
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

	// HideSplashOnStartup suppresses the welcome splash overlay that
	// treemux otherwise opens on every start. Toggled from the splash
	// itself (h) and persisted to the global config layer; `treemux
	// start --welcome` still forces the splash. Default: false.
	HideSplashOnStartup bool `yaml:"hide_splash_on_startup,omitempty" toml:"hide_splash_on_startup,omitempty" jsonschema:"description=Hide the treemux welcome splash on startup,default=false" jsonschema_extras:"x-layer=global,x-priority=67"`

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

	// PluginOrder lists plugin config keys in the preferred rail order.
	// Configured plugins omitted from the list follow in key-sorted order;
	// stale or unknown keys are ignored.
	PluginOrder []string `yaml:"plugin_order,omitempty" toml:"plugin_order,omitempty" jsonschema:"description=Preferred plugin rail order by plugin ID; omitted plugins follow sorted by ID" jsonschema_extras:"x-layer=global,x-priority=60"`

	// Plugins defines process-based plugin panels that run standalone
	// executables in PTY panels with their own rail icons.
	Plugins map[string]*PluginConfig `yaml:"plugins,omitempty" toml:"plugins,omitempty" jsonschema:"description=Process-based plugin panels" jsonschema_extras:"x-layer=global,x-priority=60"`

	// Focus configures the BSP pane focus indicator system.
	Focus *FocusConfig `yaml:"focus,omitempty" toml:"focus,omitempty" jsonschema:"description=BSP pane focus indicator configuration" jsonschema_extras:"x-layer=global,x-priority=61"`

	// DrawerOrientation controls the position of the active sessions drawer.
	// "right" places it as a vertical sidebar; "bottom" places it as a
	// horizontal bar. Default: "right".
	DrawerOrientation string `yaml:"drawer_orientation,omitempty" toml:"drawer_orientation,omitempty" jsonschema:"description=Active sessions drawer position,enum=right,enum=bottom,default=right" jsonschema_extras:"x-layer=global,x-priority=62"`

	// DrawerSize is the expanded drawer's extent along its own axis: columns
	// for the right orientation, rows for the bottom one. Written either
	// absolutely (35) or as a share of the terminal ("25%"). Unset leaves the
	// host's per-orientation default in place, and the host also applies a
	// floor — a drawer narrower than its panes can render is not a smaller
	// drawer, it is a broken one.
	DrawerSize DrawerSize `yaml:"drawer_size,omitempty" toml:"drawer_size,omitempty" json:"drawer_size,omitempty" jsonschema:"description=Expanded drawer extent along its own axis - columns on the right / rows on the bottom - as an absolute count (35) or a percentage of the terminal (25%)" jsonschema_extras:"x-layer=global,x-priority=62"`

	// DrawerExpanded controls whether the active sessions drawer starts
	// expanded (showing full list) or collapsed (mini icons only).
	// Default: false (collapsed).
	DrawerExpanded bool `yaml:"drawer_expanded,omitempty" toml:"drawer_expanded,omitempty" jsonschema:"description=Start active sessions drawer expanded,default=false" jsonschema_extras:"x-layer=global,x-priority=63"`

	// Drawer configures the named pages shown in the global drawer. Nil keeps
	// the built-in page configuration unchanged.
	Drawer *DrawerViewsConfig `yaml:"drawer,omitempty" toml:"drawer,omitempty" json:"drawer,omitempty" jsonschema:"description=Named global drawer pages and their layouts" jsonschema_extras:"x-layer=global,x-priority=63"`

	ExperimentalPages []string `yaml:"experimental_pages,omitempty" toml:"experimental_pages,omitempty" json:"experimental_pages,omitempty" jsonschema:"description=List of experimental pages to enable (env,memory,keymap,logs,inspector)" jsonschema_extras:"x-layer=global,x-priority=64"`

	// JobDetail configures keybinds for the job detail tab wrapper.
	JobDetail *JobDetailConfig `yaml:"job_detail,omitempty" toml:"job_detail,omitempty" json:"job_detail,omitempty" jsonschema:"description=Job detail pane tab keybindings" jsonschema_extras:"x-layer=global,x-priority=65"`

	// Agent configures native agent pane hosting (TERM override,
	// automatic repaint nudges for renderer corruption healing).
	Agent *AgentPaneConfig `yaml:"agent,omitempty" toml:"agent,omitempty" json:"agent,omitempty" jsonschema:"description=Native agent pane behavior" jsonschema_extras:"x-layer=global,x-priority=66"`

	// WhichKeyDelayMs is the hold-time (milliseconds) a chord prefix must be
	// armed before the which-key popup renders — the vim `timeoutlen` idiom, so
	// a fast two-key chord (e.g. "vl") never flashes the popup. Pointer so
	// "unset" is distinguishable from an explicit 0; nil falls back to the
	// keymap.WhichKeyDelay default (400ms). 0 shows the popup immediately. This
	// is the SHOW clock, distinct from the sequence EXPIRE timeout.
	WhichKeyDelayMs *int `yaml:"whichkey_delay_ms,omitempty" toml:"whichkey_delay_ms,omitempty" json:"whichkey_delay_ms,omitempty" jsonschema:"description=Delay in milliseconds before the which-key chord popup appears (0 = immediate),default=400" jsonschema_extras:"x-layer=global,x-priority=68"`
}

// DrawerViewsConfig configures named pages in the global drawer.
type DrawerViewsConfig struct {
	CycleKey    string                       `yaml:"cycle_key,omitempty" toml:"cycle_key,omitempty" json:"cycle_key,omitempty" jsonschema:"description=Action sub-key used to cycle drawer pages; none disables it"`
	DefaultPage string                       `yaml:"default_page,omitempty" toml:"default_page,omitempty" json:"default_page,omitempty" jsonschema:"description=Drawer page selected at startup"`
	PageOrder   []string                     `yaml:"page_order,omitempty" toml:"page_order,omitempty" json:"page_order,omitempty" jsonschema:"description=Ordered drawer page names"`
	Pages       map[string]*DrawerPageConfig `yaml:"pages,omitempty" toml:"pages,omitempty" json:"pages,omitempty" jsonschema:"description=Named drawer page definitions"`
	// Files configures the accessed-files pane. Pane-level settings live beside
	// the pages rather than inside them because a pane is not owned by a page:
	// any page's layout may mount "files", and all of them want the same view.
	Files *DrawerFilesConfig `yaml:"files,omitempty" toml:"files,omitempty" json:"files,omitempty" jsonschema:"description=Settings for the accessed-files drawer pane"`
	// Panes declares drawer panes whose BACKEND is chosen by the user rather
	// than compiled in. It sits beside Files for the same reason Files does: a
	// pane is not owned by a page, so what a pane IS belongs next to the page
	// definitions, never inside one.
	//
	// An entry naming a name the host already registers REDEFINES that pane —
	// which is what makes a widget dual-mode. `changes` is the exemplar: it is
	// an in-process git-viewer widget by default, and an entry here pointing at
	// git-viewer's own binary makes the same pane a sidecar over embed/v1, in
	// the same page and the same tree as its in-process neighbours.
	Panes map[string]*DrawerPaneConfig `yaml:"panes,omitempty" toml:"panes,omitempty" json:"panes,omitempty" jsonschema:"description=Drawer panes whose backend and view are chosen in config: in-process or an embed/v1 sidecar process or a host-drawn digest"`
	// Responsive lets a mounted pane with nothing to show shrink to its header
	// plus its one ⓘ line, handing the rows it cannot use to a content-bearing
	// sibling on the same page. The tree never changes shape: nothing is
	// unmounted, reordered or recompiled — only the row split moves.
	//
	// Unset is ON: the feature has baked, and a pane that hands its unused rows
	// to a sibling is what a reader wants often enough that it is not worth
	// asking for. false restores pure ratio layout permanently, which is the
	// setting to reach for if a pane ever resizes at a moment you did not
	// expect.
	Responsive *bool `yaml:"responsive,omitempty" toml:"responsive,omitempty" json:"responsive,omitempty" jsonschema:"description=Let empty drawer panes give their unused rows to content-bearing siblings (default: true)"`
	// HideInapplicablePages omits a page whose scope SUBJECT is absent from the
	// page map entirely, instead of dimming its run. It is opt-in because the
	// map's whole premise is a fixed shape you can learn by looking at it, and a
	// page that comes and goes with the focus teaches less than one that is
	// always there greyed out — but a user who never works with agents has a
	// standing answer, not a transient one, and should be able to stop paying
	// bar width for it.
	//
	// It is PAGE-level only and deliberately does not extend to hiding an
	// unavailable pane inside an applicable page: a page's scope is a declared
	// statement about what it is for, while a pane going quiet is transient, and
	// hiding on that signal would make the bar twitch continuously.
	//
	// The ACTIVE page is never hidden whatever this says — see the host's page
	// map builder. A map with no accent anywhere is strictly worse than a map
	// with one dim run in it.
	HideInapplicablePages *bool `yaml:"hide_inapplicable_pages,omitempty" toml:"hide_inapplicable_pages,omitempty" json:"hide_inapplicable_pages,omitempty" jsonschema:"description=Omit drawer pages whose scope subject is absent from the page map instead of dimming them; the active page is never hidden (default: false)"`
	// PageMapLongForm renders the open drawer's tab bar as labelled pages —
	// NAME (jump key) followed by the glyph run — wrapping across lines rather
	// than as the compact glyph strip. Opt-in because the compact form is what
	// fits a narrow drawer; see the page map's own doc comment for why labelling
	// EVERY page does not reintroduce the reflow that removed the single active
	// label.
	PageMapLongForm *bool `yaml:"page_map_long_form,omitempty" toml:"page_map_long_form,omitempty" json:"page_map_long_form,omitempty" jsonschema:"description=Render the drawer page map as labelled pages with their jump keys instead of the compact glyph strip (default: false)"`
}

// ResponsiveDrawer reports whether content-aware pane sizing is on, treating an
// absent config and an absent key alike. Hosts must call this rather than
// dereferencing the field, so the default lives in exactly one place — which is
// what made turning it on by default the one-line change it turned out to be.
func (c *DrawerViewsConfig) ResponsiveDrawer() bool {
	return c == nil || c.Responsive == nil || *c.Responsive
}

// HideInapplicableDrawerPages reports whether a page whose scope subject is
// absent is omitted from the page map rather than dimmed. Same contract as
// [DrawerViewsConfig.ResponsiveDrawer]: absent config and absent key are the
// same answer, and hosts call this rather than dereferencing the field so the
// default lives in exactly one place.
func (c *DrawerViewsConfig) HideInapplicableDrawerPages() bool {
	return c != nil && c.HideInapplicablePages != nil && *c.HideInapplicablePages
}

// LongFormPageMap reports whether the drawer's page map renders in its labelled
// long form. Named for the question rather than the field because the field
// name is already taken by the struct — same accessor contract as its two
// neighbours otherwise.
func (c *DrawerViewsConfig) LongFormPageMap() bool {
	return c != nil && c.PageMapLongForm != nil && *c.PageMapLongForm
}

// Drawer files view modes. Tree is the default: the repo → dir → file shape is
// what a reader is looking for in a list of files, and follow mode reveals the
// newest access inside it, so the live question the flat list answers by ORDER
// ("what is this agent doing right now") is still answered by the cursor.
//
// Flat remains a first-class view, not a fallback — its append-ordered-by-last-
// access sequence is information no trie can carry — and stays one t away.
const (
	DrawerFilesViewFlat = "flat"
	DrawerFilesViewTree = "tree"
)

// DrawerFilesConfig configures the accessed-files drawer pane.
type DrawerFilesConfig struct {
	// View is the view the pane STARTS in ("flat" or "tree"); the pane's own t
	// key toggles it per session and outranks this once used. Only "flat" turns
	// the trie off — any other value, including an unrecognized one, leaves the
	// default in place: a bad spelling here should cost you the preference, not
	// the pane.
	View string `yaml:"view,omitempty" toml:"view,omitempty" json:"view,omitempty" jsonschema:"description=Initial view for the accessed-files drawer pane,enum=flat,enum=tree,default=tree"`
}

// Drawer pane backends. In-process is the default and the one every built-in
// widget uses; sidecar spawns a process in the pane's own PTY and speaks
// embed/v1 to it over a per-pane control socket.
//
// The choice is a PROCESS boundary, not an ownership one. A first-party widget
// in the same go.work should stay in-process whatever repository owns it —
// compile-time types, no serialization, and a dynamic size hint the layout can
// poll. The sidecar backend buys isolation and independent release, and is paid
// for with everything crossing a wire; it is worth it for third-party,
// other-language or untrusted panels, and for nothing that can simply be
// imported.
//
// This is a CLOSED set, and deliberately so: each value names a different
// mechanism the host implements, so the host is the only party that can add
// one. It is the opposite of [DrawerPaneConfig.View], which is open because
// only the panel can name its own layouts. Keeping the two apart is the whole
// design — a backend says what is behind the pane, a view says which of its own
// renderings the panel should draw.
const (
	DrawerBackendInProcess = "in-process"
	DrawerBackendSidecar   = "sidecar"
	// DrawerBackendDigest is a pane with NO child, no PTY and no blit: the host
	// draws it, from a short message the panel published from wherever it is
	// actually running.
	//
	// A digest is a PROJECTION of a running panel, not a second instance of one
	// — a PTY is one grid at one size, and a drawer pane may be mounted at most
	// once per tree, so there is no way for one process to produce two
	// differently-sized renderings. Nothing is spawned headless to feed it
	// either: a PTY-less instance would be a second lifecycle nobody asked for.
	//
	// What it draws is a panelproto.Digest the panel published: a required
	// line, an optional second line, a host-owned state enum and an icon key.
	// No number and no pre-styled text — the host owns every escape code in a
	// digest row, and a drawer column has no room for a gauge beside a
	// sentence. A pane whose publisher is not running says so instead — naming
	// it, so "pomodoro publishes no digest" (declared, not running) is not
	// mistaken for "no panel named pomodoro" (a Source that resolves to
	// nothing) — which is the degenerate case of the feature rather than a
	// placeholder for it.
	//
	// Which panel it projects is [DrawerPaneConfig.Source], defaulting to the
	// pane's own name.
	DrawerBackendDigest = "digest"
)

// DrawerPaneConfig declares one drawer pane's backend and, for a sidecar-backed
// pane, the process behind it.
//
// The sidecar fields mirror [PluginConfig] deliberately: a drawer sidecar and a
// rail sidecar are the same kind of thing spawned in two different places, and
// a panel author who has written one manifest should not have to learn a second
// vocabulary to mount it in a drawer. What is NOT mirrored is as deliberate:
// there is no `position` (the position is the drawer slot a page's layout puts
// it in) and no dynamic size hint (see MinWidth).
type DrawerPaneConfig struct {
	// Backend selects the implementation. Empty means [DrawerBackendInProcess],
	// so an entry that sets only `min_width` tunes the built-in widget rather
	// than accidentally asking for a process.
	Backend string `yaml:"backend,omitempty" toml:"backend,omitempty" json:"backend,omitempty" jsonschema:"description=Which implementation backs this pane: in-process (the default) or a sidecar process speaking embed/v1 or digest (no child — the host draws what the panel published from wherever it is running),enum=in-process,enum=sidecar,enum=digest,default=in-process"`
	// View names which of the PANEL's own layouts to draw. It is an OPAQUE
	// string: the host carries it verbatim from here into the welcome frame
	// (panelproto.Welcome.View) and does nothing else with it.
	//
	// Not an enum, not validated against a list this package holds, not
	// branched on anywhere in host code. [PluginConfig.Settings] is the
	// precedent and the comparison is exact — a free-form value the host
	// delivers and does not interpret. That property is worth restating beside
	// the field rather than only in a design note, because it erodes the first
	// time someone adds one convenient `switch view {`.
	//
	// Deliberately NOT a surface name ("rail" / "drawer") and not a size tier.
	// A panel does not need to know where it is; it needs to know which of its
	// own layouts to draw, and only the panel knows what those are. A place
	// would make every author invent their own mapping from place to layout,
	// all slightly differently; a size tier would make the host own a
	// vocabulary it cannot define — a panel whose two modes are `tree` and
	// `flat` has no tier to be.
	//
	// Empty means the panel's own default: whatever it renders today with no
	// host at all. So every existing declaration and every existing panel keeps
	// working untouched, and a name the panel does not implement is the panel's
	// to handle — the host cannot know it was wrong.
	View string `yaml:"view,omitempty" toml:"view,omitempty" json:"view,omitempty" jsonschema:"description=Which of the panel's own named layouts to draw; carried verbatim to the panel and never interpreted by the host. Empty means the panel's default."`
	// Source names which panel's digest a digest-backed pane shows. Read only
	// for [DrawerBackendDigest]; ignored otherwise.
	//
	// Empty means the pane's own name, which is the case worth optimizing for:
	// `[tui.drawer.panes.breaktimer] backend = "digest"` projects
	// `[tui.plugins.breaktimer]`, the panel running on the rail, and the user
	// wrote the name once. This key exists for the case that convention cannot
	// express — a digest of a pane whose own name is already taken, because a
	// drawer pane and its digest cannot both be `[tui.drawer.panes.probe]`.
	//
	// It resolves to a PANEL, not to a pane: a rail plugin and a sidecar-backed
	// drawer pane are both publishers, and a digest of a digest is not a thing.
	// A name that resolves to nothing running is not an error — the pane says
	// so in its empty state, which is the same answer it gives while the panel
	// is still starting.
	Source string `yaml:"source,omitempty" toml:"source,omitempty" json:"source,omitempty" jsonschema:"description=Which panel's digest a digest-backed pane shows; empty means the pane's own name"`
	// Command is the executable to run for a sidecar-backed pane. Required when
	// Backend is sidecar; ignored otherwise.
	Command string `yaml:"command,omitempty" toml:"command,omitempty" json:"command,omitempty" jsonschema:"description=Executable to run for a sidecar-backed pane"`
	// Args, Cwd and Env are fixed at spawn, exactly as for a rail plugin.
	Args []string `yaml:"args,omitempty" toml:"args,omitempty" json:"args,omitempty" jsonschema:"description=Arguments passed to the sidecar command"`
	Cwd  string   `yaml:"cwd,omitempty" toml:"cwd,omitempty" json:"cwd,omitempty" jsonschema:"description=Working directory for the sidecar command"`
	Env  []string `yaml:"env,omitempty" toml:"env,omitempty" json:"env,omitempty" jsonschema:"description=Extra environment variables for the sidecar (KEY=VALUE)"`
	// Label is the pane heading the HOST draws above a sidecar's blit region.
	// Empty falls back to the pane name.
	Label string `yaml:"label,omitempty" toml:"label,omitempty" json:"label,omitempty" jsonschema:"description=Heading the host draws above the pane (defaults to the pane name)"`
	// Icon is the page-map icon KEY, resolved through the host's own icon
	// table — the same thing widget.Spec.Glyph is, named as data so a sidecar
	// can have one at all.
	Icon string `yaml:"icon,omitempty" toml:"icon,omitempty" json:"icon,omitempty" jsonschema:"description=Page-map icon name for this pane; resolved through the host icon table"`
	// Protocol opts the pane into the embed/v1 control plane. Empty means
	// embed/v1 for a sidecar pane — unlike [tui.plugins], where empty means a
	// plain PTY plugin. A drawer pane that never speaks the control plane can
	// never report availability, so the default that makes the feature work is
	// the right one here.
	Protocol string `yaml:"protocol,omitempty" toml:"protocol,omitempty" json:"protocol,omitempty" jsonschema:"description=Control-plane protocol for the sidecar; empty means embed/v1,enum=,enum=embed/v1"`
	// ProtocolTimeout bounds the handshake the same way PluginConfig's does.
	ProtocolTimeout string `yaml:"protocol_timeout,omitempty" toml:"protocol_timeout,omitempty" json:"protocol_timeout,omitempty" jsonschema:"description=Handshake deadline for the sidecar (Go duration; default 2s)"`
	// Restart auto-restarts the sidecar when it exits, as for a rail plugin.
	Restart bool `yaml:"restart,omitempty" toml:"restart,omitempty" json:"restart,omitempty" jsonschema:"description=Auto-restart the sidecar when it exits"`
	// MinWidth and MinHeight are the pane's DECLARED minimum, and they are the
	// whole of what a sidecar can say about its size.
	//
	// Declared, never dynamic: the host polls a widget's minimum during page
	// compilation and its dynamic counterpart during layout, and both are
	// documented as pure and cheap — which a socket round-trip is not. So a
	// sidecar pane opts out of content-aware shrinking (it is `flexible`), and
	// its in-process siblings keep doing it normally. Below this minimum the
	// pane renders the too-small placeholder rather than a squeezed column of
	// punctuation.
	MinWidth  int `yaml:"min_width,omitempty" toml:"min_width,omitempty" json:"min_width,omitempty" jsonschema:"description=Smallest column count at which this pane still renders something worth showing; below it the pane shows the too-small placeholder,minimum=0"`
	MinHeight int `yaml:"min_height,omitempty" toml:"min_height,omitempty" json:"min_height,omitempty" jsonschema:"description=Smallest row count at which this pane still renders something worth showing,minimum=0"`
	// Settings is the free-form table handed to the sidecar verbatim in the
	// welcome frame and re-delivered on config_reload. Same contract, and the
	// same non-interpretation guarantee, as PluginConfig.Settings.
	Settings map[string]interface{} `yaml:"settings,omitempty" toml:"settings,omitempty" json:"settings,omitempty" jsonschema:"description=Free-form settings table delivered to the sidecar over embed/v1 (data only; never executed by the host)"`
	// Keys mirrors the chords the sidecar declares it intends to claim. It is a
	// DECLARATION and grants nothing — the real claims arrive in the handshake
	// — but the host lints it against the drawer's own vocabulary at
	// registration time, so a pane claiming `<`, `>` or `?` is warned about
	// before it silently swallows a page switch.
	Keys []PluginKey `yaml:"keys,omitempty" toml:"keys,omitempty" json:"keys,omitempty" jsonschema:"description=Chords the sidecar declares it intends to claim while focused (declaration only; the host arbitrates the real claims at handshake time)"`
	// Views mirrors the views the PANEL declares it can draw — `grove plugin
	// install` copies them here from the manifest's [panel.views.<name>], in
	// declaration order.
	//
	// It is the author's half of [DrawerPaneConfig.View]: the user names one
	// view, the author says which views exist and which of them are meant for a
	// pane this narrow. Like [Keys] it is a DECLARATION that grants and forbids
	// nothing — a `view` naming something absent from this list still mounts and
	// still reaches the panel verbatim, because only the panel knows what it
	// implements.
	//
	// The host reads exactly one thing from it, [PluginView.Drawer], and only in
	// the two ways [EffectiveView] and [DeclaredView] express. It never reads a
	// NAME here, which is what keeps the view an open set.
	Views []PluginView `yaml:"views,omitempty" toml:"views,omitempty" json:"views,omitempty" jsonschema:"description=Views the panel declares it can draw, in declaration order (declaration only; the host reads only each view's drawer suitability, never its name)"`
	// Notebook mirrors the panel's declared notebook subtree, exactly as
	// [PluginConfig.Notebook] does — see there for the whole contract, which is
	// the same contract in both places because it is a fact about the panel
	// rather than about where it is mounted: declarative, host-ignored, and
	// deliberately outside the exec gate because no host ever acts on it.
	Notebook *PluginNotebook `yaml:"notebook,omitempty" toml:"notebook,omitempty" json:"notebook,omitempty" jsonschema:"description=Notebook subtree the panel declares it writes into (declaration only; the host never resolves or enforces the path)"`
}

// SidecarBacked reports whether this pane asks for a process. Hosts call it
// rather than comparing Backend themselves, so the empty-means-in-process
// default lives in one place.
func (c *DrawerPaneConfig) SidecarBacked() bool {
	return c != nil && c.Backend == DrawerBackendSidecar
}

// DigestBacked reports whether this pane asks the host to draw a projection of
// a panel running elsewhere. The counterpart to SidecarBacked, and mutually
// exclusive with it: a digest pane spawns nothing at all.
func (c *DrawerPaneConfig) DigestBacked() bool {
	return c != nil && c.Backend == DrawerBackendDigest
}

// DigestSource is the panel whose digest this pane projects: [Source] when the
// user named one, and the pane's own name when they did not.
//
// Here rather than in the host for the reason [EffectiveProtocol] is: "what does
// an unset field mean" is the config vocabulary's question. It takes the pane
// name as an argument because a declaration does not know its own map key —
// which is also why this cannot be a plain accessor.
func (c *DrawerPaneConfig) DigestSource(pane string) string {
	if c == nil || c.Source == "" {
		return pane
	}
	return c.Source
}

// EffectiveProtocol is the control-plane version a sidecar pane is spawned
// with. Empty means embed/v1 here, which is the opposite of [PluginConfig]'s
// default and for a stated reason: a rail plugin with no control plane is a
// perfectly good terminal pane, while a drawer pane with none can never tell
// the host it is unavailable or what its empty state says.
func (c *DrawerPaneConfig) EffectiveProtocol() string {
	if c == nil || c.Protocol == "" {
		return "embed/v1"
	}
	return c.Protocol
}

// EffectiveView is the view a drawer pane actually mounts with: the one the user
// named, or — when they named none — the first view the panel's author declared
// drawer-suitable.
//
// That default is what makes an installed panel work well in a drawer before the
// user has read its docs. Declaration order carries the preference because the
// author writes their preferred drawer view first; there is deliberately no
// separate `preferred` key, which would be a second way to say the same thing
// and a second way to say it inconsistently.
//
// Empty is a legitimate answer and stays empty: a panel that declares no
// drawer-suitable view is declining to offer one, which is information rather
// than a gap, and empty on the wire means the panel's own default.
//
// This lives here rather than in a host for the reason [EffectiveProtocol] does
// — "what does an unset field mean" is the config vocabulary's own question. Note
// what it does NOT do: it never compares a view name to anything, so the host
// still holds no view vocabulary. Presence is not identity.
func (c *DrawerPaneConfig) EffectiveView() string {
	if c == nil {
		return ""
	}
	if c.View != "" {
		return c.View
	}
	for _, v := range c.Views {
		if v.Drawer {
			return v.Name
		}
	}
	return ""
}

// DeclaredView looks up what the panel's author said about one view. The second
// return distinguishes "declared, and not for a drawer" from "not declared at
// all", which are different situations: the first is something the host can
// report honestly, and the second is a name only the panel can judge.
func (c *DrawerPaneConfig) DeclaredView(name string) (PluginView, bool) {
	if c == nil || name == "" {
		return PluginView{}, false
	}
	for _, v := range c.Views {
		if v.Name == name {
			return v, true
		}
	}
	return PluginView{}, false
}

// DrawerPageConfig defines one named drawer page. A partial built-in page
// inherits omitted fields during final resolution by the TUI host.
type DrawerPageConfig struct {
	Key string `yaml:"key,omitempty" toml:"key,omitempty" json:"key,omitempty" jsonschema:"description=Action sub-key for this page; none explicitly unbinds it"`
	// LeaderKey binds this page under the LEADER chord instead of the action
	// chord. It is unset by default because the leader digits 1-9 already jump
	// to the Nth window (tmux-style); binding one here deliberately shadows that
	// jump for that digit, which is why it must be opted into per page.
	LeaderKey string            `yaml:"leader_key,omitempty" toml:"leader_key,omitempty" json:"leader_key,omitempty" jsonschema:"description=Leader sub-key for this page; shadows the leader window jump on that key; none explicitly unbinds it"`
	Icon      string            `yaml:"icon,omitempty" toml:"icon,omitempty" json:"icon,omitempty" jsonschema:"description=Named icon used for the page"`
	Layout    *DrawerNodeConfig `yaml:"layout,omitempty" toml:"layout,omitempty" json:"layout,omitempty" jsonschema:"description=Recursive BSP layout for the page"`
	// Size overrides the shared [tui] drawer_size while this page is showing,
	// in the same absolute-or-percentage syntax. A page whose panes want the
	// room (a wide diff, a deep tree) can ask for it without every other page
	// paying for it. Hosts honor it on a direct jump to the page and ignore it
	// while cycling: cycling is browsing, and reflowing the main area on every
	// step of a browse is worse than a page rendering at the shared width.
	Size DrawerSize `yaml:"size,omitempty" toml:"size,omitempty" json:"size,omitempty" jsonschema:"description=Drawer extent while this page is showing - honored on a direct jump and ignored while cycling - as an absolute count (70) or a percentage of the terminal (40%)"`
	// Scope declares what the page is ABOUT. Unset means mixed. It never
	// reorders pages and never switches them; it lets a host explain an empty
	// page in one sentence, group adjacent same-subject pages in the page map,
	// and dim a whole page when its subject is absent. See [DrawerPageScope].
	Scope  DrawerPageScope `yaml:"scope,omitempty" toml:"scope,omitempty" json:"scope,omitempty" jsonschema:"description=What this page is about - drives page-level empty reasons and page-map grouping - never reorders or switches pages,enum=global,enum=workspace,enum=worktree,enum=agent,enum=agents,enum=mixed,default=mixed"`
	Delete bool            `yaml:"delete,omitempty" toml:"delete,omitempty" json:"delete,omitempty" jsonschema:"description=Remove this page from inherited configuration"`
}

// DrawerNodeConfig is either a Pane leaf or a split with First and Second
// children. Shape and value validation is performed by the TUI host.
type DrawerNodeConfig struct {
	// Pane names the widget mounted at this leaf, or — with the `page:` prefix
	// ([DrawerPageRefPrefix]) — another page whose layout is inlined here. The
	// reference is what composes a wide drawer out of pages that already exist:
	// see [DrawerPageRef].
	Pane  string  `yaml:"pane,omitempty" toml:"pane,omitempty" json:"pane,omitempty" jsonschema:"description=Pane name for a leaf node; the prefix page: instead inlines another page's layout here (page:sessions)"`
	Split string  `yaml:"split,omitempty" toml:"split,omitempty" json:"split,omitempty" jsonschema:"description=Split direction,enum=auto,enum=horizontal,enum=vertical"`
	Ratio float64 `yaml:"ratio,omitempty" toml:"ratio,omitempty" json:"ratio,omitempty" jsonschema:"description=Fraction allocated to the first child"`
	// MinWidth and MinHeight are the smallest extent this node is worth
	// compiling at, in cells. They are what makes a multi-column page
	// RESPONSIVE: a host compiles an explicit split only while both children
	// can still get their minimum along the split axis, and otherwise falls
	// back to the orientation's natural stacking rather than handing every pane
	// a strip too narrow to read.
	//
	// Unset means "use the host's built-in default"
	// ([DrawerMinWidthDefault] / [DrawerMinHeightDefault]), which is itself
	// outranked by a minimum the mounted widget declares for itself. Setting
	// one here is the last word: it is the only statement that knows what THIS
	// layout is for.
	MinWidth  int               `yaml:"min_width,omitempty" toml:"min_width,omitempty" json:"min_width,omitempty" jsonschema:"description=Smallest column count this node is worth compiling at; an explicit split that cannot give both children their minimum degrades to the drawer orientation's natural stacking,minimum=0"`
	MinHeight int               `yaml:"min_height,omitempty" toml:"min_height,omitempty" json:"min_height,omitempty" jsonschema:"description=Smallest row count this node is worth compiling at; an explicit split that cannot give both children their minimum degrades to the drawer orientation's natural stacking,minimum=0"`
	First     *DrawerNodeConfig `yaml:"first,omitempty" toml:"first,omitempty" json:"first,omitempty" jsonschema:"description=First split child"`
	Second    *DrawerNodeConfig `yaml:"second,omitempty" toml:"second,omitempty" json:"second,omitempty" jsonschema:"description=Second split child"`
}

// AgentPaneConfig controls how treemux hosts agent CLI panes (claude etc.).
// Added as a mitigation for Claude Code's stale-screen-model rendering bug:
// its renderer occasionally emits cell-merged frames, corrupting the pane.
// A SIGWINCH (full Ink repaint) heals the live region, so treemux can nudge
// the agent PTY automatically after output bursts and on pane focus.
type AgentPaneConfig struct {
	// Term overrides TERM for treemux-spawned agent PTYs (default
	// xterm-256color). Setting e.g. screen-256color makes renderers like
	// Ink take their conservative tmux render path (fuller line redraws),
	// which can avoid the stale-model frame merging at the source.
	Term string `yaml:"term,omitempty" toml:"term,omitempty" json:"term,omitempty" jsonschema:"description=TERM value for agent pane PTYs (e.g. screen-256color for the conservative tmux render path),default=xterm-256color"`

	// RepaintNudge enables automatic PTY winsize jiggles (SIGWINCH →
	// full repaint) after agent output bursts settle and on pane focus,
	// healing renderer corruption in the live region. Default: true.
	RepaintNudge *bool `yaml:"repaint_nudge,omitempty" toml:"repaint_nudge,omitempty" json:"repaint_nudge,omitempty" jsonschema:"description=Automatically SIGWINCH-nudge agent panes after output bursts to heal rendering corruption,default=true"`

	// AutoApprove configures answering agent permission prompts from the
	// rendered pane. Off unless enabled. See AgentAutoApproveConfig.
	AutoApprove *AgentAutoApproveConfig `yaml:"auto_approve,omitempty" toml:"auto_approve,omitempty" json:"auto_approve,omitempty" jsonschema:"description=Answer matching agent permission prompts by pressing Enter in the pane"`
}

// AgentAutoApproveConfig controls whether treemux answers an agent's
// permission prompt by pressing Enter in the pane.
//
// The prompts this exists for are the ones Claude Code raises purely because
// its static analyser could not model the command's shell syntax — `git log
// $BRANCH`, `ls {a,b}`. Those are refused BEFORE permissions.allow is
// consulted, so no allow rule can ever cover them, and the refusal reason is
// not visible to a hook or a setting. It is, however, printed in the dialog,
// so treemux reads it off the pane's own terminal grid and answers there.
//
// Nothing is auto-approved by default, and the guards are structural rather
// than pattern-based: only the sandboxed "Bash command" dialog is ever a
// candidate (the "(unsandboxed)" variant, file writes, fetches and tool use
// are not), and only while its highlighted option is exactly "Yes" — so the
// keystroke can never write a permission rule or answer something else.
//
// Example grove.toml:
//
//	[tui.agent.auto_approve]
//	enabled = true
//	deny_patterns = ["terraform apply"]
type AgentAutoApproveConfig struct {
	// Enabled turns auto-approval on for every agent pane. Default: false.
	Enabled *bool `yaml:"enabled,omitempty" toml:"enabled,omitempty" json:"enabled,omitempty" jsonschema:"description=Answer matching agent permission prompts automatically,default=false"`

	// ApprovePatterns are matched case-insensitively against the dialog text;
	// any match makes it a candidate. They describe the REASON class being
	// answered, not the command. Setting this REPLACES the built-in list
	// ("cannot be statically analyzed", "expansion obfuscation").
	ApprovePatterns []string `yaml:"approve_patterns,omitempty" toml:"approve_patterns,omitempty" json:"approve_patterns,omitempty" jsonschema:"description=Dialog text that makes a permission prompt eligible for auto-approval; replaces the built-in static-analysis-miss list"`

	// DenyPatterns veto a candidate, matched against the whole dialog
	// including the command itself. This EXTENDS the built-in floor (sandbox
	// escapes, rm -rf, sudo, git push) — it cannot shrink it.
	DenyPatterns []string `yaml:"deny_patterns,omitempty" toml:"deny_patterns,omitempty" json:"deny_patterns,omitempty" jsonschema:"description=Dialog text that vetoes auto-approval; extends the built-in deny floor rather than replacing it"`
}

// JobDetailConfig configures direct keybinds for the job detail tab wrapper.
// These only activate when the wrapper's active tab is NOT a PTY.
type JobDetailConfig struct {
	Editor string `yaml:"editor,omitempty" toml:"editor,omitempty" json:"editor,omitempty" jsonschema:"description=Key to jump to the editor tab,default=e"`
	Rules  string `yaml:"rules,omitempty" toml:"rules,omitempty" json:"rules,omitempty" jsonschema:"description=Key to jump to the cx rules tab,default=r"`
	Logs   string `yaml:"logs,omitempty" toml:"logs,omitempty" json:"logs,omitempty" jsonschema:"description=Key to jump to the logs tab,default=l"`
}

// FocusConfig controls how the focused BSP pane is visually distinguished.
type FocusConfig struct {
	// Style selects the focus indicator strategy: border (highlight
	// separator cells adjacent to focused pane), gutter (1-col colored
	// bar on left edge), or title (1-row colored header).
	Style string `yaml:"style,omitempty" toml:"style,omitempty" jsonschema:"description=Focus indicator style,enum=border,enum=gutter,enum=title,default=gutter"`
	// ActiveColor is the color used for the focused pane's indicator.
	// Named theme colors ("cyan", "accent", …) and hex literals are
	// accepted; the shipped default is "cyan".
	ActiveColor string `yaml:"active_color,omitempty" toml:"active_color,omitempty" jsonschema:"description=Color for focused pane indicator,default=cyan"`
	// InactiveColor is the color used for unfocused pane indicators.
	// "none" hides the unfocused indicator entirely.
	InactiveColor string `yaml:"inactive_color,omitempty" toml:"inactive_color,omitempty" jsonschema:"description=Color for unfocused pane indicator,default=none"`
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
	// Label is the human-readable name shown on the rail item and in the
	// panel's host-facing surfaces. Empty falls back to the map key, which is
	// constrained to a TOML bare key and therefore often not what a user would
	// choose to read ("break-timer" vs "Break timer").
	Label string `yaml:"label,omitempty" toml:"label,omitempty" jsonschema:"description=Display label for the rail item (defaults to the plugin's config key)"`
	// Position controls where the plugin appears. "rail" (the default, and
	// the only supported value) gives the plugin a persistent icon-rail pane.
	// "ephemeral" was declared here but never had a consumer; spawn-on-demand
	// panes are configured under [tui.panels.bindings], which also carries the
	// key chord such a pane needs in order to be reachable.
	Position string `yaml:"position,omitempty" toml:"position,omitempty" jsonschema:"description=Panel position: rail (persistent icon-rail pane). For spawn-on-demand panels use [tui.panels.bindings].,enum=rail,default=rail"`
	// Cwd is the working directory for the command.
	Cwd string `yaml:"cwd,omitempty" toml:"cwd,omitempty" jsonschema:"description=Working directory for the command"`
	// Env are extra environment variables (KEY=VALUE format).
	Env []string `yaml:"env,omitempty" toml:"env,omitempty" jsonschema:"description=Extra environment variables (KEY=VALUE)"`
	// Restart controls whether the plugin auto-restarts on exit.
	Restart bool `yaml:"restart,omitempty" toml:"restart,omitempty" jsonschema:"description=Auto-restart plugin on exit,default=false"`
	// Protocol opts the plugin into the embed-over-socket control plane.
	// Empty (the default) is a plain PTY plugin: the host spawns it and
	// renders its output, and nothing else. "embed/v1" additionally makes the
	// host create a per-panel unix socket, pass its path in
	// GROVE_PANEL_SOCKET, and exchange JSON-lines control messages over it
	// (focus, workspace scope, deep links, key claims). The rendering plane is
	// identical either way, so a protocol panel that never connects degrades
	// to a plain PTY plugin.
	Protocol string `yaml:"protocol,omitempty" toml:"protocol,omitempty" jsonschema:"description=Control-plane protocol: empty for a plain PTY plugin or embed/v1 for a socket-connected sidecar panel,enum=,enum=embed/v1"`
	// ProtocolTimeout bounds how long the host waits for a protocol panel to
	// connect and complete its handshake before reporting the pane degraded.
	// A Go duration string; empty means the host default (2s). The listener
	// stays open past the deadline, so a slow sidecar still connects — the
	// timeout governs the host's readiness reporting, not the socket's life.
	ProtocolTimeout string `yaml:"protocol_timeout,omitempty" toml:"protocol_timeout,omitempty" jsonschema:"description=Handshake deadline for protocol panels (Go duration; default 2s)"`
	// Settings is a free-form table the host hands to the panel verbatim —
	// [tui.plugins.<name>.settings] in grove.toml, delivered in the embed/v1
	// welcome frame and re-delivered live on config_reload. It is how a panel
	// gets configured at all: before it, a plugin's only knobs were `args` and
	// `env`, both fixed at spawn, so changing one meant restarting the process.
	//
	// It is DATA, and the schema deliberately leaves it unconstrained: grove
	// does not know what a third-party panel's options are, and a schema that
	// guessed would reject valid ones.
	//
	// It is not separately exec-gated, and that is a decision rather than an
	// omission. The gate quarantines `tui.plugins.*` — the WHOLE entry — so a
	// settings table from a repo-controlled layer is already stripped along with
	// the `command` that would read it, and a settings table that survives came
	// from a layer the user owns. Gating the subtree again would add a second
	// prompt about a value that cannot arrive without the first.
	//
	// What keeps that true is that nothing here is ever executed. The host does
	// not interpret settings; it forwards them. A panel that chooses to treat
	// one of its own settings as a command is making that decision in its own
	// code, with the authority it already had as a process the user configured —
	// exactly as it would if it read the same value from its own dotfile. If a
	// future host ever wants to act on a settings value itself, that key needs
	// its own RegisterExecField entry before it ships, not after.
	Settings map[string]interface{} `yaml:"settings,omitempty" toml:"settings,omitempty" jsonschema:"description=Free-form settings table delivered to the panel over the embed/v1 control plane (data only; never executed by the host)"`
	// Keys mirrors the host chords the panel declares it intends to claim —
	// `grove plugin install` copies them here from the manifest's
	// [[panel.keys]], which is the list the user read on the consent screen.
	//
	// It is a DECLARATION and grants nothing: a protocol panel's real claims
	// arrive in its handshake and are arbitrated there. It exists so the two
	// can be compared. Without it the host had no idea what a panel was
	// supposed to ask for, so a panel claiming chords the user never approved
	// looked exactly like one claiming the chords they did — and `treemux keys`
	// could only describe the compiled-in flow panel, because it was the only
	// hosted key reference reachable without a running handshake.
	Keys []PluginKey `yaml:"keys,omitempty" toml:"keys,omitempty" jsonschema:"description=Host chords the panel declares it intends to claim (declaration only; the host arbitrates the real claims at handshake time)"`
	// View names which of the PANEL's own layouts to draw, exactly as
	// [DrawerPaneConfig.View] does — see there for the whole contract, which is
	// the same contract in both places because it is a fact about the panel
	// rather than about where it is mounted.
	//
	// It is here for two reasons. The drawer's declaration projects onto this
	// type (a drawer sidecar and a rail sidecar are one implementation), so the
	// field has to exist here for the string to reach the wire at all; and a
	// rail entry has the same legitimate use for it, because a panel with a
	// `wide` layout and a `graph` layout can be asked for either from either
	// place. The host still reads it nowhere.
	View string `yaml:"view,omitempty" toml:"view,omitempty" jsonschema:"description=Which of the panel's own named layouts to draw; carried verbatim to the panel and never interpreted by the host. Empty means the panel's default."`
	// Views mirrors the panel's own view declaration, copied here from the
	// manifest's [panel.views.<name>] by `grove plugin install` — see
	// [DrawerPaneConfig.Views], which is the same declaration and the same
	// contract, because it is a fact about the panel rather than about where it
	// is mounted.
	//
	// Nothing reads it on the rail: a rail pane is as wide as the terminal, so
	// "is this view drawer-suitable" has no bearing there. It is written into the
	// installed fragment for the reason [Settings] is — the file the user edits
	// to configure a panel should state every view they can ask that panel for —
	// and it is where a drawer pane declaration is copied FROM.
	Views []PluginView `yaml:"views,omitempty" toml:"views,omitempty" jsonschema:"description=Views the panel declares it can draw, in declaration order (declaration only; nothing on the rail reads it)"`
	// Notebook mirrors the panel's declared notebook subtree — copied here from
	// the manifest's [panel.notebook] by `grove plugin install`, which is the
	// claim the user read on the consent screen.
	//
	// It is DECLARATIVE and host-ignored, exactly as [Keys] and [Views] are: no
	// host resolves the path, creates the directory, or fences the panel into
	// it. The panel writes with whatever authority it already has as a process
	// the user approved; this field exists so the file the user edits repeats
	// what that panel said it would do with their notebook. Because no host
	// ever acts on the value there is no RegisterExecField entry for it — the
	// same reasoning [Settings] records: the exec gate is for values a host
	// would act on, and this one is only ever rendered.
	Notebook *PluginNotebook `yaml:"notebook,omitempty" toml:"notebook,omitempty" jsonschema:"description=Notebook subtree the panel declares it writes into (declaration only; the host never resolves or enforces the path)"`
}

// PluginKey is one declared host chord in a plugin's [[tui.plugins.<name>.keys]].
type PluginKey struct {
	// Key is the chord in the host's key vocabulary (bubbletea's), e.g.
	// "ctrl+f".
	Key string `yaml:"key" toml:"key" jsonschema:"description=Key chord in the host's vocabulary (e.g. ctrl+f)"`
	// Description is the human-readable effect, shown wherever the host lists
	// the panel's keys.
	Description string `yaml:"description,omitempty" toml:"description,omitempty" jsonschema:"description=What the chord does, shown in the host's help surfaces"`
}

// PluginView is one view a panel declares it can draw — one entry of
// [[tui.plugins.<name>.views]] or [[tui.drawer.panes.<name>.views]], copied from
// the manifest's [panel.views.<name>] table by `grove plugin install`.
//
// An ARRAY here where the manifest has named tables, and that is the one thing
// the translation adds: order. The author's declaration order is their
// preference order (see [DrawerPaneConfig.EffectiveView]), and a TOML table of
// tables decodes into a map, which has none.
type PluginView struct {
	// Name is the view's name in the PANEL's vocabulary, exactly as
	// [DrawerPaneConfig.View] names one. The set is open and the host holds no
	// list of members; this entry is a claim by the author, not a registration.
	Name string `yaml:"name" toml:"name" json:"name" jsonschema:"description=The view's name in the panel's own vocabulary"`
	// Description is what the view is for, in the author's words. Read by people
	// — the install consent screen and anyone opening the fragment to choose a
	// view — and by no code.
	Description string `yaml:"description,omitempty" toml:"description,omitempty" json:"description,omitempty" jsonschema:"description=What this view shows, in the author's words"`
	// Drawer says whether the author means this view for a drawer pane: a column
	// tens of cells wide and a handful of rows tall.
	//
	// It is the ONE thing about a view the host reads, and it does exactly two
	// things with it — supply the default when the user named no view, and warn
	// when a view the author excluded is mounted in a drawer anyway. It is never
	// enforcement: a `drawer = false` view mounted in a drawer loads and runs, it
	// is just ugly, and the host says so rather than substituting a choice the
	// user did not make.
	Drawer bool `yaml:"drawer,omitempty" toml:"drawer,omitempty" json:"drawer,omitempty" jsonschema:"description=Whether the author means this view for a drawer pane; the host defaults to the first such view and warns when a view marked false is mounted in a drawer anyway"`
}

// PluginNotebook is the notebook subtree a panel declares it writes —
// [tui.plugins.<name>.notebook] or [tui.drawer.panes.<name>.notebook], copied
// from the manifest's [panel.notebook] by `grove plugin install`.
//
// Like [PluginKey] and [PluginView] it is a DECLARATION the host renders and
// never acts on: nothing resolves the path, nothing creates it, nothing fences
// the panel into it. It exists so the consent screen's claim about the user's
// notebook survives into the file they edit, instead of living only in a
// prompt that scrolled away.
type PluginNotebook struct {
	// Subtree is the notebook-relative path the panel says it writes under,
	// e.g. "hn/clippings". Opaque to every host: it is compared to nothing and
	// joined with nothing.
	Subtree string `yaml:"subtree" toml:"subtree" json:"subtree" jsonschema:"description=Notebook-relative subtree the panel declares it writes under (e.g. hn/clippings)"`
	// Description is what the panel saves there, in the author's words. Read
	// by people — the consent screen and anyone opening the fragment — and by
	// no code.
	Description string `yaml:"description,omitempty" toml:"description,omitempty" json:"description,omitempty" jsonschema:"description=What the panel saves there, in the author's words"`
}

// PanelConfig holds configuration for user-defined ephemeral panel
// keybindings. Command is the default binary; Bindings is a named
// map of keybindings that each spawn a panel.
type PanelConfig struct {
	// Command is the default binary to run. Defaults to $EDITOR or "vi".
	Command string `yaml:"command,omitempty" toml:"command,omitempty" jsonschema:"description=Default command binary (falls back to $EDITOR or vi)"`
	// Singleton is the default singleton setting applied to every binding
	// that does not set its own. Set at the [tui.panels] level to make all
	// panel keybindings focus-or-create a single reusable pane (e.g. ctrl+e).
	// A binding's own singleton=true still wins; this only supplies the default.
	Singleton bool                          `yaml:"singleton,omitempty" toml:"singleton,omitempty" jsonschema:"description=Default singleton setting for all bindings (focus a single reusable pane instead of spawning a new one)"`
	Bindings  map[string]PanelBindingConfig `yaml:"bindings,omitempty" toml:"bindings,omitempty" jsonschema:"description=Named panel keybindings"`
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
	// Singleton makes the binding focus-or-create a single reusable pane
	// (deterministic ID "editor-bound-<label>") instead of spawning a fresh
	// pane on every press. Use for scratchpad-style editors (e.g. ctrl+e).
	Singleton bool `yaml:"singleton,omitempty" toml:"singleton,omitempty" jsonschema:"description=Focus a single reusable pane instead of spawning a new one each press"`
}

// ContextConfig holds configuration for the grove-context (cx) tool.
type ContextConfig struct {
	ReposDir         *string `yaml:"repos_dir,omitempty" toml:"repos_dir,omitempty" jsonschema:"description=Directory where cx repo stores bare repositories (default: ~/.local/share/grove/cx)" jsonschema_extras:"x-layer=global,x-priority=80"`
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

// BuildConfig holds configuration for the daemon's machine-wide build queue.
type BuildConfig struct {
	MaxParallel int `yaml:"max_parallel,omitempty" toml:"max_parallel,omitempty" jsonschema:"description=Maximum number of build jobs running concurrently machine-wide (default: max(2\\, NumCPU/2))"`
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
	Commands         map[string]interface{} `yaml:"commands,omitempty" toml:"commands,omitempty" json:"commands,omitempty" jsonschema:"description=Named commands that run in the context of this environment. Each entry is either a shell-string (e.g. build = \"make build\") or a table with command/startup keys (startup=true auto-runs the command after env up)"`
	DisplayEndpoints []string               `yaml:"display_endpoints,omitempty" toml:"display_endpoints,omitempty" json:"display_endpoints,omitempty" jsonschema:"description=Env var names whose values should surface as endpoints in the TUI. If unset\\, any http(s) value is treated as an endpoint."`
	DisplayResources []string               `yaml:"display_resources,omitempty" toml:"display_resources,omitempty" json:"display_resources,omitempty" jsonschema:"description=Human-readable resource labels shown on the Shared Infra page (e.g. 'Cloud SQL (myproject:us-central1:db)'). Purely cosmetic; no schema constraint."`
	Shared           *bool                  `yaml:"shared,omitempty" toml:"shared,omitempty" json:"shared,omitempty" jsonschema:"description=Whether this profile represents shared ecosystem infrastructure consumed by other profiles via shared_env."`
}

// DaemonConfig holds configuration for the grove daemon (groved).
type DaemonConfig struct {
	GitInterval            string            `yaml:"git_interval,omitempty" toml:"git_interval,omitempty" jsonschema:"description=How often to poll git status (default: 10s)"`
	SessionInterval        string            `yaml:"session_interval,omitempty" toml:"session_interval,omitempty" jsonschema:"description=How often to poll sessions (default: 2s)"`
	WorkspaceInterval      string            `yaml:"workspace_interval,omitempty" toml:"workspace_interval,omitempty" jsonschema:"description=How often to refresh workspace discovery (default: 30s)"`
	PlanInterval           string            `yaml:"plan_interval,omitempty" toml:"plan_interval,omitempty" jsonschema:"description=How often to poll plan stats (default: 30s)"`
	NoteInterval           string            `yaml:"note_interval,omitempty" toml:"note_interval,omitempty" jsonschema:"description=How often to poll note counts (default: 60s)"`
	ConfigWatch            *bool             `yaml:"config_watch,omitempty" toml:"config_watch,omitempty" jsonschema:"description=Enable config watching (default: true)"`
	ConfigDebounceMs       int               `yaml:"config_debounce_ms,omitempty" toml:"config_debounce_ms,omitempty" jsonschema:"description=Debounce window for rapid config changes in milliseconds (default: 100)"`
	AutoSyncSkills         *bool             `yaml:"auto_sync_skills,omitempty" toml:"auto_sync_skills,omitempty" jsonschema:"description=Enable automatic syncing of skills on file change (default: true)"`
	AutoSyncClaudeSettings *bool             `yaml:"auto_sync_claude_settings,omitempty" toml:"auto_sync_claude_settings,omitempty" jsonschema:"description=Enable automatic syncing of .claude settings on file change (default: true)"`
	SkillSyncDebounceMs    int               `yaml:"skill_sync_debounce_ms,omitempty" toml:"skill_sync_debounce_ms,omitempty" jsonschema:"description=Debounce window for skill syncs in milliseconds (default: 1000)"`
	Hooks                  *DaemonHooks      `yaml:"hooks,omitempty" toml:"hooks,omitempty" jsonschema:"description=Daemon-specific hooks configuration"`
	Jobs                   *DaemonJobsConfig `yaml:"jobs,omitempty" toml:"jobs,omitempty" jsonschema:"description=Job runner configuration"`
	Build                  *BuildConfig      `yaml:"build,omitempty" toml:"build,omitempty" jsonschema:"description=Machine-wide build queue configuration"`
	SSH                    *DaemonSSHConfig  `yaml:"ssh,omitempty" toml:"ssh,omitempty" jsonschema:"description=Embedded SSH server configuration"`
	PairWithTreemux        *bool             `yaml:"pair_with_treemux,omitempty" toml:"pair_with_treemux,omitempty" jsonschema:"description=Opt-in to kill daemon when the parent treemux exits"`

	JobReconcile *DaemonJobReconcileConfig `yaml:"job_reconcile,omitempty" toml:"job_reconcile,omitempty" jsonschema:"description=Reconciliation of job files left claiming an active status by processes that died"`
}

// DaemonJobReconcileConfig controls the JobCollector's sweep for job
// files stuck on an active status ("running") with nothing alive behind
// them — the ghosts left when a daemon restart loses a job, or a
// process dies without anyone watching.
//
// It defaults to report-only. Rewriting somebody's job file on
// inference is the kind of change that should be observed in logs for a
// release before it is trusted to act, so `enabled` must be set
// explicitly to make the sweep write anything.
type DaemonJobReconcileConfig struct {
	Enabled   *bool  `yaml:"enabled,omitempty" toml:"enabled,omitempty" jsonschema:"description=Actually rewrite stuck job files. When false (the default) the sweep only logs what it would have changed."`
	QuietFor  string `yaml:"quiet_for,omitempty" toml:"quiet_for,omitempty" jsonschema:"description=How long a job file must be untouched before the sweep will reconcile it (default: 10m)"`
	MaxPerRun int    `yaml:"max_per_run,omitempty" toml:"max_per_run,omitempty" jsonschema:"description=Cap on files reconciled per sweep, so a bad inference can't rewrite a whole notebook at once (default: 25)"`
}

// DaemonSSHConfig holds configuration for the embedded SSH server.
type DaemonSSHConfig struct {
	Enabled     *bool  `yaml:"enabled,omitempty" toml:"enabled,omitempty" jsonschema:"description=Enable the embedded SSH server (default: false)"`
	Port        int    `yaml:"port,omitempty" toml:"port,omitempty" jsonschema:"description=Port to listen on (default: 2222)"`
	HostKeyPath string `yaml:"host_key_path,omitempty" toml:"host_key_path,omitempty" jsonschema:"description=Path to the SSH host key (default: ~/.local/state/grove/ssh_host_key)"`
	BindAddress string `yaml:"bind_address,omitempty" toml:"bind_address,omitempty" jsonschema:"description=Address to bind the SSH server to (default: 127.0.0.1)"`
}

// DaemonHooks defines hooks that are triggered by daemon events.
type DaemonHooks struct {
	OnSkillSync []HookCommand `yaml:"on_skill_sync,omitempty" toml:"on_skill_sync,omitempty" jsonschema:"description=Commands to run after skills are synced for a workspace"`
	OnEvent     []EventHook   `yaml:"on_event,omitempty" toml:"on_event,omitempty" jsonschema:"description=Commands to run when the daemon broadcasts a matching lifecycle event"`
}

// EventHook binds a set of daemon lifecycle events to a shell command.
//
//	[[daemon.hooks.on_event]]
//	events  = ["job_completed", "job_failed"]
//	filter  = "workspace=grove*"
//	command = "notify-send grove \"$GROVE_JOB_ID $GROVE_EVENT_TYPE\""
//	timeout = 30
//
// The daemon publishes a typed event vocabulary (jobs, sessions, workflows,
// builds, notes, plans, git…) over its SSE bus; this is the exec-side
// subscription to it. The event is delivered as JSON on the hook's stdin and
// as GROVE_* environment variables — the same two conventions grove-env-<name>
// sidecars and Claude Code hooks already use.
//
// The embedded HookCommand supplies the lifecycle semantics (timeout,
// cancel_previous, disable_env/enable_env); `run_if` is a skill-sync concept
// and is ignored here, since "did anything change" is what the event itself
// already asserts.
//
// SECURITY: this is exec-bearing config. It is registered in the
// exec-provenance gate (see execgate.go), so a definition arriving from a
// cloned repository's grove.toml is quarantined unless the user has trusted
// that file with `grove config trust`.
type EventHook struct {
	// HookCommand carries name/command plus the execution lifecycle. It is
	// embedded, so its keys appear at the same level as events/filter in TOML.
	HookCommand `yaml:",inline"`

	// Events lists the update types that trigger this hook. Glob patterns are
	// allowed, so `job_*` catches the whole job lifecycle. An empty list never
	// fires — the daemon logs that at startup rather than defaulting to "all",
	// because a hook that silently subscribes to the firehose is a footgun.
	Events []string `yaml:"events,omitempty" toml:"events,omitempty" jsonschema:"description=Daemon event types that trigger this hook (glob patterns allowed e.g. job_*)"`

	// Filter narrows matches by event field. It is deliberately NOT an
	// expression language: terms are `field=glob` pairs ANDed together, over
	// workspace, plan, job_id, status, source and origin. A bare term with no
	// `=` is a substring match against workspace, plan and job_id. An
	// expression language (CEL/expr) is a named follow-up.
	Filter string `yaml:"filter,omitempty" toml:"filter,omitempty" jsonschema:"description=Optional field filter e.g. workspace=grove* or plan=extensib*"`
}

// HookCommand defines a command to be executed for a hook.
type HookCommand struct {
	Name           string `yaml:"name" toml:"name" jsonschema:"description=Name of the hook command"`
	Command        string `yaml:"command" toml:"command" jsonschema:"description=Shell command to execute"`
	RunIf          string `yaml:"run_if,omitempty" toml:"run_if,omitempty" jsonschema:"enum=always,enum=changes,description=Condition to run the command (always or changes)"`
	Timeout        int    `yaml:"timeout,omitempty" toml:"timeout,omitempty" jsonschema:"description=Maximum run time in seconds before the hook is killed (default 600)"`
	CancelPrevious bool   `yaml:"cancel_previous,omitempty" toml:"cancel_previous,omitempty" jsonschema:"description=If true, SIGTERM any in-flight instance of the same hook when a new event fires"`
	DisableEnv     string `yaml:"disable_env,omitempty" toml:"disable_env,omitempty" jsonschema:"description=Skip this hook when the named environment variable is non-empty"`
	EnableEnv      string `yaml:"enable_env,omitempty" toml:"enable_env,omitempty" jsonschema:"description=Skip this hook unless the named environment variable is non-empty (opt-in gating)"`
}

// PostToolUseHook defines a reminder hook that emits additional context to
// the agent when a tool call matches a Claude Code permission-rule filter.
type PostToolUseHook struct {
	Name              string `yaml:"name" toml:"name" jsonschema:"description=Name of the reminder hook"`
	If                string `yaml:"if" toml:"if" jsonschema:"description=Claude Code permission-rule filter (e.g. 'Bash(git commit *)' or 'Edit(*.go)')"`
	AdditionalContext string `yaml:"additional_context" toml:"additional_context" jsonschema:"description=Reminder text emitted as hookSpecificOutput.additionalContext on match"`
}

// HooksConfig groups all hook-related settings.
type HooksConfig struct {
	OnStop      []HookCommand     `yaml:"on_stop,omitempty" toml:"on_stop,omitempty" jsonschema:"description=Commands to run when a session stops"`
	PostToolUse []PostToolUseHook `yaml:"post_tool_use,omitempty" toml:"post_tool_use,omitempty" jsonschema:"description=Reminder hooks that emit additional context after tool calls"`
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
	// Sync is tagged toml:"-" because the key accepts two shapes (the typed
	// SyncConfig table and the legacy provider list); TOML decoding happens
	// in postProcessTOMLNotebookSync, YAML via SyncConfig.UnmarshalYAML.
	Sync      *SyncConfig      `yaml:"sync,omitempty" toml:"-" jsonschema:"description=Synchronization configuration for this notebook"`
	Syncthing *SyncthingConfig `yaml:"syncthing,omitempty" toml:"syncthing,omitempty" jsonschema:"description=Syncthing automated setup configuration"`
	Obsidian  *ObsidianConfig  `yaml:"obsidian,omitempty" toml:"obsidian,omitempty" jsonschema:"description=Obsidian vault automated setup configuration"`
}

// WorktreeConfig holds settings for git worktrees.
type WorktreeConfig struct {
	// Layout selects where new worktrees are created: "xdg" (under the XDG
	// data dir) or "legacy" (in-repo under .grove-worktrees/).
	Layout string `yaml:"layout,omitempty" toml:"layout,omitempty" jsonschema:"description=Worktree layout: xdg (XDG data dir) or legacy (in-repo .grove-worktrees),enum=xdg,enum=legacy"`
}

// OnboardingConfig tracks the first-run onboarding flow's persistent state,
// written to the user-global layer (~/.config/grove/grove.toml). A nil
// section reads as not-completed (treemux boots into the setup takeover);
// all consumers must be nil-safe.
type OnboardingConfig struct {
	// Completed marks the flow finished; treemux stops entering the
	// takeover on startup (re-runnable via `treemux start --onboard`).
	Completed bool `yaml:"completed,omitempty" toml:"completed,omitempty" jsonschema:"description=First-run onboarding finished; treemux no longer enters the setup takeover on startup,default=false" jsonschema_extras:"x-layer=global,x-priority=90"`
	// LastStep is the resume marker for a mid-run quit; cleared when the
	// flow completes.
	LastStep string `yaml:"last_step,omitempty" toml:"last_step,omitempty" jsonschema:"description=Step ID the onboarding flow last persisted (resume marker; cleared on completion)" jsonschema_extras:"x-layer=global,x-priority=91"`
}

// TestScopeConfig defines a smart test triggering scope
type TestScopeConfig struct {
	Name      string   `yaml:"name" toml:"name" jsonschema:"description=Name of the test scope"`
	Rules     string   `yaml:"rules" toml:"rules" jsonschema:"description=Path to cx .rules file"`
	Scenarios []string `yaml:"scenarios" toml:"scenarios" jsonschema:"description=List of tend scenarios to trigger"`
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

	Commands   map[string]string `yaml:"commands,omitempty" toml:"commands,omitempty" jsonschema:"description=Command overrides per verb"`
	TestScopes []TestScopeConfig `yaml:"test_scopes,omitempty" toml:"test_scopes,omitempty" jsonschema:"description=Smart test triggering scopes"`

	Worktree *WorktreeConfig `yaml:"worktree,omitempty" toml:"worktree,omitempty" jsonschema:"description=Git worktree settings (layout)"`

	Onboarding *OnboardingConfig `yaml:"onboarding,omitempty" toml:"onboarding,omitempty" jsonschema:"description=First-run onboarding progress (completed marker + resume step)"`

	Security *SecurityConfig `yaml:"security,omitempty" toml:"security,omitempty" jsonschema:"description=Security policy (exec-bearing config trust gate)"`

	// Extensions captures all other top-level keys for extensibility.
	Extensions map[string]interface{} `yaml:",inline" toml:"-" jsonschema:"-"`

	// ExecGate reports what the exec-provenance gate did during this load:
	// which repo-controlled layer files carried exec-bearing config, whether
	// they are trusted, and what was quarantined. Populated by the loaders
	// (LoadFrom*/LoadLayered) after merging; nil for configs built any other
	// way. Never read from or written to a config file — see execgate.go.
	ExecGate *ExecGateReport `yaml:"-" toml:"-" json:"-" jsonschema:"-"`
}

// SecurityConfig holds grove's security policy. It is only ever honored from
// user-controlled layers (global config, ~/.config/grove fragments, global
// override, GROVE_CONFIG_OVERLAY) — a workspace grove.toml setting
// exec_trust = "off" would otherwise disable the gate that exists to contain
// that very file.
type SecurityConfig struct {
	// ExecTrust selects the exec-provenance enforcement policy:
	//
	//   default  quarantine implicit-exec values (hooks, plugin panels, panel
	//            keybindings, key-resolution commands) from untrusted layers;
	//            warn about user-invoked ones (build_cmd, commands, env)
	//   strict   quarantine every exec-bearing value from untrusted layers
	//   warn     never strip; report only
	//   off      disable the gate entirely
	//
	// Overridden by the GROVE_EXEC_TRUST environment variable.
	ExecTrust string `yaml:"exec_trust,omitempty" toml:"exec_trust,omitempty" jsonschema:"description=Exec-provenance gate policy for config from untrusted (repo-controlled) layers,enum=default,enum=strict,enum=warn,enum=off,default=default" jsonschema_extras:"x-layer=global,x-priority=95"`

	// InheritWorktreeTrust lets a worktree checkout inherit the trust decision
	// already made about the SAME repo's config in its owner checkout, but only
	// when the two files carry byte-identical exec values (same digest). A
	// branch that edits a hook gets a different digest and inherits nothing, so
	// this never runs a command the user has not reviewed — it only stops
	// asking twice about content they already reviewed once. See
	// exectrust_inherit.go.
	//
	// Unset means enabled; set false to require a fresh review per worktree.
	InheritWorktreeTrust *bool `yaml:"inherit_worktree_trust,omitempty" toml:"inherit_worktree_trust,omitempty" jsonschema:"description=When true (the default) a new worktree inherits its owner checkout's exec-trust decision for member repos whose config carries identical exec values; set false to require a fresh review in every worktree,default=true" jsonschema_extras:"x-layer=global,x-priority=94"`
}

// UnmarshalYAML implements custom YAML unmarshaling to handle backward compatibility
// for the old configuration formats.
func (c *Config) UnmarshalYAML(node *yaml.Node) error {
	// Create a temporary struct with all fields to capture the data, including legacy ones.
	type rawConfig struct {
		Name             string                        `yaml:"name,omitempty"`
		Version          string                        `yaml:"version"`
		Workspaces       []string                      `yaml:"workspaces,omitempty"`
		BuildCmd         string                        `yaml:"build_cmd,omitempty"`
		BuildAfter       []string                      `yaml:"build_after,omitempty"`
		Notebooks        *NotebooksConfig              `yaml:"notebooks,omitempty"`
		TUI              *TUIConfig                    `yaml:"tui,omitempty"`
		Context          *ContextConfig                `yaml:"context,omitempty"`
		Daemon           *DaemonConfig                 `yaml:"daemon,omitempty"`
		Environment      *EnvironmentConfig            `yaml:"environment,omitempty"`
		Environments     map[string]*EnvironmentConfig `yaml:"environments,omitempty"`
		Groves           map[string]GroveSourceConfig  `yaml:"groves,omitempty"`
		ExplicitProjects []ExplicitProject             `yaml:"explicit_projects,omitempty"`
		Commands         map[string]string             `yaml:"commands,omitempty"`
		TestScopes       []TestScopeConfig             `yaml:"test_scopes,omitempty"`
		Worktree         *WorktreeConfig               `yaml:"worktree,omitempty"`
		Onboarding       *OnboardingConfig             `yaml:"onboarding,omitempty"`
		Security         *SecurityConfig               `yaml:"security,omitempty"`
		Extensions       map[string]interface{}        `yaml:",inline"`

		// --- Legacy Fields for Backward Compatibility ---
		SearchPaths       map[string]SearchPathConfig `yaml:"search_paths,omitempty"`        // Old name for Groves
		LegacyNotebooks   map[string]*Notebook        `yaml:"-"`                             // To catch top-level notebooks map
		LegacyNotebook    *Notebook                   `yaml:"notebook,omitempty"`            // Very old single notebook
		DefaultNotebook   string                      `yaml:"default_notebook,omitempty"`    // Old top-level default
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
	c.Commands = raw.Commands
	c.TestScopes = raw.TestScopes
	c.Worktree = raw.Worktree
	c.Onboarding = raw.Onboarding
	c.Security = raw.Security
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
	return func(from, to reflect.Type, data interface{}) (interface{}, error) {
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
