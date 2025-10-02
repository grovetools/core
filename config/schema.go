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
