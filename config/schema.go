package config

import (
	"encoding/json"

	"github.com/invopop/jsonschema"
)

// GenerateSchema generates the base JSON Schema for the core Grove configuration.
// It reflects the Config struct from types.go but excludes the 'Extensions' field,
// which will be handled by schema composition.
//
// The jsonschema_extras tag is used to add custom x-* properties for UI metadata:
//   - x-layer: Recommended config layer ("global", "ecosystem", "project")
//   - x-priority: Sort order (lower = higher in list). 1-10 for wizard, 50+ common, 100+ advanced
//   - x-important: true if field is a key/important configuration option
//   - x-sensitive: true for API keys - mask display
//   - x-hint: Additional guidance shown in edit dialog
func GenerateSchema() ([]byte, error) {
	return GenerateSchemaWithThemeNames(nil)
}

// GenerateSchemaWithThemeNames is GenerateSchema with a closed enum for
// tui.theme injected from the given names. The theme roster lives in the
// tui/theme registry (embedded TOML palettes), which imports this package —
// so the names are passed in by the schema-generator tool instead of being
// imported here (that would be an import cycle) or hardcoded in struct tags
// (that caused roster drift). Passing nil leaves tui.theme an open string.
func GenerateSchemaWithThemeNames(themeNames []string) ([]byte, error) {
	r := &jsonschema.Reflector{
		// Do not allow unknown fields, extensions will be added explicitly during composition.
		AllowAdditionalProperties: false,
		// Expand struct references instead of using $ref for cleaner base schema.
		ExpandedStruct: true,
		// Use YAML field names for property names
		FieldNameTag: "yaml",
	}

	// Create a temporary struct that omits the Extensions field
	// so it's not included in the base schema.
	// UI metadata (x-layer, x-priority, x-important) is added via jsonschema_extras.

	// The mirrors below track logging.Config and its sub-structs, which this
	// package cannot import (core/logging imports core/config). Every field
	// must carry ,omitempty so the reflector marks nothing required: configs
	// are merged from partial fragments, and the load-path validation
	// (config.validateAndWarn) checks each fragment against this schema —
	// a required field would flag every fragment that doesn't set it.

	// FileSinkSchemaConfig mirrors logging.FileSinkConfig.
	type FileSinkSchemaConfig struct {
		Enabled       bool   `yaml:"enabled,omitempty" jsonschema:"description=Enable file logging,default=true"`
		Path          string `yaml:"path,omitempty" jsonschema:"description=Full path to the log file"`
		Format        string `yaml:"format,omitempty" jsonschema:"description=File log format: text or json,default=json,enum=text,enum=json"`
		Level         string `yaml:"level,omitempty" jsonschema:"description=Minimum log level for the file sink only (defaults to the console level; GROVE_LOG_LEVEL overrides both),enum=debug,enum=info,enum=warn,enum=error"`
		RetentionDays int    `yaml:"retention_days,omitempty" jsonschema:"description=Days of dated log files to keep before the daemon sweeps them (0 = default of 14),default=14"`
	}

	// FormatSchemaConfig mirrors logging.FormatConfig.
	type FormatSchemaConfig struct {
		Preset             string `yaml:"preset,omitempty" jsonschema:"description=Log format preset: default (rich)/simple/json,enum=default,enum=simple,enum=json"`
		DisableTimestamp   bool   `yaml:"disable_timestamp,omitempty" jsonschema:"description=Disable timestamp in log output,default=false"`
		DisableComponent   bool   `yaml:"disable_component,omitempty" jsonschema:"description=Disable component name in log output,default=false"`
		StructuredToStderr string `yaml:"structured_to_stderr,omitempty" jsonschema:"description=When to send structured logs to stderr,enum=auto,enum=always,enum=never,default=auto"`
	}

	// ComponentFilteringSchemaConfig mirrors logging.ComponentFilteringConfig.
	type ComponentFilteringSchemaConfig struct {
		Only []string `yaml:"only,omitempty" jsonschema:"description=Strict whitelist of components/groups to show (ignores show/hide)"`
		Show []string `yaml:"show,omitempty" jsonschema:"description=Components/groups to always show (overrides hide)"`
		Hide []string `yaml:"hide,omitempty" jsonschema:"description=Components/groups to hide from log output"`
	}

	// LoggingSchemaConfig mirrors logging.Config.
	type LoggingSchemaConfig struct {
		Level              string                          `yaml:"level,omitempty" jsonschema:"description=Minimum log level (debug/info/warn/error),default=info,enum=debug,enum=info,enum=warn,enum=error"`
		SystemLevel        string                          `yaml:"system_level,omitempty" jsonschema:"description=Minimum log level for system/daemon logs (debug/info/warn/error),enum=debug,enum=info,enum=warn,enum=error"`
		ReportCaller       bool                            `yaml:"report_caller,omitempty" jsonschema:"description=Include file/line/function in output,default=true"`
		LogStartup         bool                            `yaml:"log_startup,omitempty" jsonschema:"description=Log 'Grove binary started' on first init"`
		File               *FileSinkSchemaConfig           `yaml:"file,omitempty" jsonschema:"description=File logging sink configuration"`
		Format             *FormatSchemaConfig             `yaml:"format,omitempty" jsonschema:"description=Log output format settings"`
		Groups             map[string][]string             `yaml:"groups,omitempty" jsonschema:"description=Named collections of component loggers for filtering"`
		ComponentFiltering *ComponentFilteringSchemaConfig `yaml:"component_filtering,omitempty" jsonschema:"description=Rules for filtering logs by component"`
		ShowCurrentProject *bool                           `yaml:"show_current_project,omitempty" jsonschema:"description=Always show logs from current project regardless of filters"`
	}

	type BaseConfig struct {
		Name             string                        `yaml:"name,omitempty" jsonschema:"description=Name of the project or ecosystem" jsonschema_extras:"x-layer=ecosystem,x-priority=10"`
		Version          string                        `yaml:"version,omitempty" jsonschema:"description=Configuration version (e.g. 1.0)" jsonschema_extras:"x-layer=global,x-priority=100"`
		Workspaces       []string                      `yaml:"workspaces,omitempty" jsonschema:"description=Glob patterns for workspace directories in this ecosystem" jsonschema_extras:"x-layer=ecosystem,x-priority=11"`
		BuildCmd         string                        `yaml:"build_cmd,omitempty" jsonschema:"description=Custom build command (default: make build)" jsonschema_extras:"x-layer=project,x-priority=20"`
		BuildAfter       []string                      `yaml:"build_after,omitempty" jsonschema:"description=Projects that must be built before this one" jsonschema_extras:"x-layer=project,x-priority=21"`
		Notebooks        *NotebooksConfig              `yaml:"notebooks,omitempty" jsonschema:"description=Notebook configuration" jsonschema_extras:"x-layer=global,x-priority=2,x-important=true"`
		Logging          *LoggingSchemaConfig          `yaml:"logging,omitempty" jsonschema:"description=Logging configuration" jsonschema_extras:"x-layer=global,x-priority=60"`
		TUI              *TUIConfig                    `yaml:"tui,omitempty" jsonschema:"description=TUI appearance and behavior settings" jsonschema_extras:"x-layer=global,x-priority=50"`
		Context          *ContextConfig                `yaml:"context,omitempty" jsonschema:"description=Configuration for the cx (context) tool" jsonschema_extras:"x-layer=global,x-priority=80"`
		Environment      *EnvironmentConfig            `yaml:"environment,omitempty" jsonschema:"description=Default environment provider configuration" jsonschema_extras:"x-layer=project,x-priority=25"`
		Environments     map[string]*EnvironmentConfig `yaml:"environments,omitempty" jsonschema:"description=Named environment profiles selected via --env flag" jsonschema_extras:"x-layer=project,x-priority=26"`
		Groves           map[string]GroveSourceConfig  `yaml:"groves,omitempty" jsonschema:"description=Root directories to search for projects and ecosystems" jsonschema_extras:"x-layer=global,x-priority=1,x-important=true"`
		SearchPaths      map[string]SearchPathConfig   `yaml:"search_paths,omitempty" jsonschema:"description=DEPRECATED: Use groves instead,deprecated=true" jsonschema_extras:"x-layer=global,x-priority=1000,x-deprecated=true,x-deprecated-message=Use 'groves' for project discovery,x-deprecated-replacement=groves,x-deprecated-version=v0.5.0,x-deprecated-removal=v1.0.0"`
		ExplicitProjects []ExplicitProject             `yaml:"explicit_projects,omitempty" jsonschema:"description=Specific projects to include without discovery" jsonschema_extras:"x-layer=global,x-priority=5"`
		Commands         map[string]string             `yaml:"commands,omitempty" jsonschema:"description=Command overrides per verb (e.g. build check fmt lint)" jsonschema_extras:"x-layer=project,x-priority=22"`
		TestScopes       []TestScopeConfig             `yaml:"test_scopes,omitempty" jsonschema:"description=Smart test triggering scopes" jsonschema_extras:"x-layer=project,x-priority=23"`
		Onboarding       *OnboardingConfig             `yaml:"onboarding,omitempty" jsonschema:"description=First-run onboarding progress (completed marker + resume step)" jsonschema_extras:"x-layer=global,x-priority=90"`
	}

	schema := r.Reflect(&BaseConfig{})
	schema.Title = "Grove Core Configuration"
	schema.Description = "Base schema for core grove.yml properties."
	schema.Version = "http://json-schema.org/draft-07/schema#"

	// Post-process via JSON injection to ensure x-status fields appear.
	// The jsonschema library's custom marshaler ignores manual Extras modifications on nested structs,
	// so we marshal to JSON first, then inject fields directly into the JSON structure.
	jsonBytes, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}

	var rawSchema map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &rawSchema); err != nil {
		return nil, err
	}

	// Helper to safely navigate the map
	getMap := func(m map[string]interface{}, key string) map[string]interface{} {
		if val, ok := m[key]; ok {
			if casted, ok := val.(map[string]interface{}); ok {
				return casted
			}
		}
		return nil
	}

	// 1. Inject alpha status into TUIConfig.nvim_embed property
	// Path: $defs -> TUIConfig -> properties -> nvim_embed
	if defs := getMap(rawSchema, "$defs"); defs != nil {
		if tui := getMap(defs, "TUIConfig"); tui != nil {
			if props := getMap(tui, "properties"); props != nil {
				if nvimEmbed := getMap(props, "nvim_embed"); nvimEmbed != nil {
					nvimEmbed["x-status"] = "alpha"
					nvimEmbed["x-status-message"] = "Experimental Neovim embedding"
					nvimEmbed["x-status-since"] = "v0.6.0"
					nvimEmbed["x-status-target"] = "v1.0"
				}
			}
		}

		// Also inject alpha status into the NvimEmbedConfig definition itself
		if nvimConfig := getMap(defs, "NvimEmbedConfig"); nvimConfig != nil {
			nvimConfig["x-status"] = "alpha"
			nvimConfig["x-status-message"] = "Experimental Neovim embedding"
		}
	}

	// 2. Inject deprecation status into SearchPaths (top-level property)
	// Path: properties -> search_paths
	if props := getMap(rawSchema, "properties"); props != nil {
		if searchPaths := getMap(props, "search_paths"); searchPaths != nil {
			searchPaths["x-status"] = "deprecated"
			searchPaths["x-status-message"] = "Use 'groves' for project discovery"
			searchPaths["x-status-replaced-by"] = "groves"
			searchPaths["x-status-since"] = "v0.5.0"
			searchPaths["x-status-target"] = "v1.0.0"
		}
	}

	// 2b. Inject the closed theme enum from the theme registry.
	// Path: $defs -> TUIConfig -> properties -> theme
	if len(themeNames) > 0 {
		if defs := getMap(rawSchema, "$defs"); defs != nil {
			if tui := getMap(defs, "TUIConfig"); tui != nil {
				if props := getMap(tui, "properties"); props != nil {
					if themeField := getMap(props, "theme"); themeField != nil {
						enum := make([]interface{}, len(themeNames))
						for i, n := range themeNames {
							enum[i] = n
						}
						themeField["enum"] = enum
					}
				}
			}
		}
	}

	// 3. Inject x-important into GroveSourceConfig fields
	// Path: $defs -> GroveSourceConfig -> properties -> *
	if defs := getMap(rawSchema, "$defs"); defs != nil {
		if groveConfig := getMap(defs, "GroveSourceConfig"); groveConfig != nil {
			if props := getMap(groveConfig, "properties"); props != nil {
				for _, fieldName := range []string{"path", "enabled", "description", "notebook"} {
					if field := getMap(props, fieldName); field != nil {
						field["x-important"] = true
					}
				}
			}
		}

		// 4. Inject x-important into Notebook fields
		// Path: $defs -> Notebook -> properties -> root_dir
		if notebook := getMap(defs, "Notebook"); notebook != nil {
			if props := getMap(notebook, "properties"); props != nil {
				if rootDir := getMap(props, "root_dir"); rootDir != nil {
					rootDir["x-important"] = true
				}
			}
		}

		// 5. Inject x-important into NotebookRules fields
		// Path: $defs -> NotebookRules -> properties -> default, global
		if notebookRules := getMap(defs, "NotebookRules"); notebookRules != nil {
			if props := getMap(notebookRules, "properties"); props != nil {
				for _, fieldName := range []string{"default", "global"} {
					if field := getMap(props, fieldName); field != nil {
						field["x-important"] = true
					}
				}
			}
		}

		// 6. Inject x-important into GlobalNotebookConfig fields
		// Path: $defs -> GlobalNotebookConfig -> properties -> root_dir
		if globalNotebook := getMap(defs, "GlobalNotebookConfig"); globalNotebook != nil {
			if props := getMap(globalNotebook, "properties"); props != nil {
				if rootDir := getMap(props, "root_dir"); rootDir != nil {
					rootDir["x-important"] = true
				}
			}
		}
	}

	return json.MarshalIndent(rawSchema, "", "  ")
}
