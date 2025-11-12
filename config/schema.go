package config

import (
	"encoding/json"

	"github.com/invopop/jsonschema"
)

// GenerateSchema generates the base JSON Schema for the core Grove configuration.
// It reflects the Config struct from types.go but excludes the 'Extensions' field,
// which will be handled by schema composition.
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
	type BaseConfig struct {
		Name             string                       `yaml:"name,omitempty" jsonschema:"description=Name of the configuration"`
		Version          string                       `yaml:"version" jsonschema:"required,description=Configuration version (e.g. '1.0')"`
		Workspaces       []string                     `yaml:"workspaces,omitempty" jsonschema:"description=List of workspace directories in this ecosystem"`
		Notebooks        *NotebooksConfig             `yaml:"notebooks,omitempty" jsonschema:"description=Configuration for notebooks"`
		Groves           map[string]GroveSourceConfig `yaml:"groves,omitempty" jsonschema:"description=Root directories to search for projects and ecosystems"`
		ExplicitProjects []ExplicitProject            `yaml:"explicit_projects,omitempty" jsonschema:"description=Specific projects to include without discovery"`
	}

	schema := r.Reflect(&BaseConfig{})
	schema.Title = "Grove Core Configuration"
	schema.Description = "Base schema for core grove.yml properties."
	schema.Version = "http://json-schema.org/draft-07/schema#"

	return json.MarshalIndent(schema, "", "  ")
}
