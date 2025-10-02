package config

import (
	"encoding/json"
)

// JSONSchema represents a JSON Schema structure
type JSONSchema struct {
	Schema               string                 `json:"$schema"`
	Type                 string                 `json:"type"`
	Title                string                 `json:"title,omitempty"`
	Description          string                 `json:"description,omitempty"`
	Properties           map[string]*JSONSchema `json:"properties,omitempty"`
	Items                *JSONSchema            `json:"items,omitempty"`
	Required             []string               `json:"required,omitempty"`
	Pattern              string                 `json:"pattern,omitempty"`
	MinLength            *int                   `json:"minLength,omitempty"`
	MaxLength            *int                   `json:"maxLength,omitempty"`
	Minimum              *float64               `json:"minimum,omitempty"`
	Maximum              *float64               `json:"maximum,omitempty"`
	Enum                 []interface{}          `json:"enum,omitempty"`
	AdditionalProperties interface{}            `json:"additionalProperties,omitempty"`
	PatternProperties    map[string]*JSONSchema `json:"patternProperties,omitempty"`
}

// GenerateSchema generates JSON Schema for Grove configuration
func GenerateSchema() *JSONSchema {
	return &JSONSchema{
		Schema:               "http://json-schema.org/draft-07/schema#",
		Type:                 "object",
		Title:                "Grove Configuration Schema",
		Description:          "Schema for grove.yml configuration files",
		AdditionalProperties: true, // Allow extensions for grove ecosystem tools
		Properties: map[string]*JSONSchema{
			"version": {
				Type:        "string",
				Description: "Configuration version (e.g., '1.0')",
				Pattern:     "^(\\d+\\.\\d+)?$", // Allow empty string
			},
			"agent": {
				Type:        "object",
				Description: "Grove agent configuration",
				Properties: map[string]*JSONSchema{
					"enabled": {
						Type:        "boolean",
						Description: "Enable the Grove agent",
					},
					"image": {
						Type:        "string",
						Description: "Docker image for the agent",
					},
					"logs_path": {
						Type:        "string",
						Description: "Path to mount for Claude project logs",
					},
					"extra_volumes": {
						Type:        "array",
						Description: "Additional volume mounts",
						Items: &JSONSchema{
							Type:    "string",
							Pattern: "^[^:]+:[^:]+$",
						},
					},
					"args": {
						Type:        "array",
						Description: "Additional arguments for claude command",
						Items: &JSONSchema{
							Type: "string",
						},
					},
					"output_format": {
						Type:        "string",
						Description: "Output format for piped input: text (default), json, or stream-json",
						Enum:        []interface{}{"text", "json", "stream-json"},
					},
					"notes_dir": {
						Type:        "string",
						Description: "Notes directory to mount for agent to read/write",
					},
					"mount_workspace_at_host_path": {
						Type:        "boolean",
						Description: "Mount the host's git repository root to the same path inside the container. Useful for local Go module development.",
					},
					"use_superproject_root": {
						Type:        "boolean",
						Description: "Use the superproject (parent repository) root when in a git submodule. Useful for monorepo development.",
					},
				},
			},
		},
	}
}

// ExportSchema exports the schema as JSON
func ExportSchema() ([]byte, error) {
	schema := GenerateSchema()
	return json.MarshalIndent(schema, "", "  ")
}

// ptr returns a pointer to the given value
func ptr[T any](v T) *T {
	return &v
}
