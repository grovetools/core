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
//   - x-wizard: true if field appears in setup wizard
//   - x-sensitive: true for API keys - mask display
//   - x-hint: Additional guidance shown in edit dialog
func GenerateSchema() ([]byte, error) {
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
	// UI metadata (x-layer, x-priority, x-wizard) is added via jsonschema_extras.
	type BaseConfig struct {
		Name             string                       `yaml:"name,omitempty" jsonschema:"description=Name of the project or ecosystem" jsonschema_extras:"x-layer=ecosystem,x-priority=10"`
		Version          string                       `yaml:"version,omitempty" jsonschema:"description=Configuration version (e.g. 1.0)" jsonschema_extras:"x-layer=global,x-priority=100"`
		Workspaces       []string                     `yaml:"workspaces,omitempty" jsonschema:"description=Glob patterns for workspace directories in this ecosystem" jsonschema_extras:"x-layer=ecosystem,x-priority=11"`
		BuildCmd         string                       `yaml:"build_cmd,omitempty" jsonschema:"description=Custom build command (default: make build)" jsonschema_extras:"x-layer=project,x-priority=20"`
		BuildAfter       []string                     `yaml:"build_after,omitempty" jsonschema:"description=Projects that must be built before this one" jsonschema_extras:"x-layer=project,x-priority=21"`
		Notebooks        *NotebooksConfig             `yaml:"notebooks,omitempty" jsonschema:"description=Notebook configuration" jsonschema_extras:"x-layer=global,x-priority=2,x-wizard=true"`
		TUI              *TUIConfig                   `yaml:"tui,omitempty" jsonschema:"description=TUI appearance and behavior settings" jsonschema_extras:"x-layer=global,x-priority=50"`
		Context          *ContextConfig               `yaml:"context,omitempty" jsonschema:"description=Configuration for the cx (context) tool" jsonschema_extras:"x-layer=global,x-priority=80"`
		Groves           map[string]GroveSourceConfig `yaml:"groves,omitempty" jsonschema:"description=Root directories to search for projects and ecosystems" jsonschema_extras:"x-layer=global,x-priority=1,x-wizard=true"`
		SearchPaths      map[string]SearchPathConfig  `yaml:"search_paths,omitempty" jsonschema:"description=DEPRECATED: Use groves instead,deprecated=true" jsonschema_extras:"x-layer=global,x-priority=1000,x-deprecated=true,x-deprecated-message=Use 'groves' for project discovery,x-deprecated-replacement=groves,x-deprecated-version=v0.5.0,x-deprecated-removal=v1.0.0"`
		ExplicitProjects []ExplicitProject            `yaml:"explicit_projects,omitempty" jsonschema:"description=Specific projects to include without discovery" jsonschema_extras:"x-layer=global,x-priority=5"`
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

	// 3. Inject x-wizard into GroveSourceConfig fields
	// Path: $defs -> GroveSourceConfig -> properties -> *
	if defs := getMap(rawSchema, "$defs"); defs != nil {
		if groveConfig := getMap(defs, "GroveSourceConfig"); groveConfig != nil {
			if props := getMap(groveConfig, "properties"); props != nil {
				for _, fieldName := range []string{"path", "enabled", "description", "notebook"} {
					if field := getMap(props, fieldName); field != nil {
						field["x-wizard"] = true
					}
				}
			}
		}
	}

	return json.MarshalIndent(rawSchema, "", "  ")
}
