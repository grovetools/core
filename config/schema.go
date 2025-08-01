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
		Schema:      "http://json-schema.org/draft-07/schema#",
		Type:        "object",
		Title:       "Grove Configuration Schema",
		Description: "Schema for grove.yml configuration files",
		AdditionalProperties: true, // Allow extensions for grove ecosystem tools
		Properties: map[string]*JSONSchema{
			"version": {
				Type:        "string",
				Description: "Configuration version (e.g., '3.8')",
				Pattern:     "^(\\d+\\.\\d+)?$", // Allow empty string
			},
			"services": {
				Type:        "object",
				Description: "Service definitions for your workspace",
				AdditionalProperties: &JSONSchema{
					Type: "object",
					Properties: map[string]*JSONSchema{
						"build": {
							Description: "Build configuration (string path or object)",
						},
						"image": {
							Type:        "string",
							Description: "Docker image to use",
						},
						"ports": {
							Type:        "array",
							Description: "Port mappings (host:container)",
							Items: &JSONSchema{
								Type:    "string",
								Pattern: "^\\d+:\\d+$",
							},
						},
						"volumes": {
							Type:        "array",
							Description: "Volume mounts",
							Items: &JSONSchema{
								Type: "string",
							},
						},
						"environment": {
							Type:                 "object",
							Description:          "Environment variables",
							AdditionalProperties: &JSONSchema{Type: "string"},
						},
						"depends_on": {
							Type:        "array",
							Description: "Service dependencies",
							Items: &JSONSchema{
								Type: "string",
							},
						},
						"labels": {
							Type:                 "object",
							Description:          "Container labels",
							AdditionalProperties: &JSONSchema{Type: "string"},
						},
						"command": {
							Type:        "string",
							Description: "Override default command",
						},
						"profiles": {
							Type:        "array",
							Description: "Profiles this service belongs to",
							Items: &JSONSchema{
								Type: "string",
							},
						},
					},
				},
			},
			"networks": {
				Type:        "object",
				Description: "Network definitions",
				AdditionalProperties: &JSONSchema{
					Type: "object",
					Properties: map[string]*JSONSchema{
						"driver": {
							Type:        "string",
							Description: "Network driver",
							Enum:        []interface{}{"bridge", "host", "overlay", "macvlan", "none"},
						},
						"external": {
							Type:        "boolean",
							Description: "Use external network",
						},
					},
				},
			},
			"volumes": {
				Type:        "object",
				Description: "Volume definitions",
				AdditionalProperties: &JSONSchema{
					Type: "object",
					Properties: map[string]*JSONSchema{
						"driver": {
							Type:        "string",
							Description: "Volume driver",
						},
						"external": {
							Type:        "boolean",
							Description: "Use external volume",
						},
					},
				},
			},
			"profiles": {
				Type:        "object",
				Description: "Service profiles for different environments",
				AdditionalProperties: &JSONSchema{
					Type: "object",
					Properties: map[string]*JSONSchema{
						"services": {
							Type:        "array",
							Description: "Services in this profile",
							Items: &JSONSchema{
								Type: "string",
							},
						},
						"env_files": {
							Type:        "array",
							Description: "Environment files for this profile",
							Items: &JSONSchema{
								Type: "string",
							},
						},
					},
				},
			},
			"settings": {
				Type:        "object",
				Description: "Grove-specific settings",
				Properties: map[string]*JSONSchema{
					"project_name": {
						Type:        "string",
						Description: "Override the Docker Compose project name",
						Pattern:     "^[a-z][a-z0-9_-]*$",
					},
					"default_profile": {
						Type:        "string",
						Description: "Default profile to use",
					},
					"traefik_enabled": {
						Type:        "boolean",
						Description: "Enable Traefik reverse proxy",
					},
					"network_name": {
						Type:        "string",
						Description: "Custom network name",
						Pattern:     "^[a-z][a-z0-9_-]*$",
					},
					"domain_suffix": {
						Type:        "string",
						Description: "Domain suffix for services",
						Pattern:     "^[a-z][a-z0-9.-]*$",
					},
					"auto_infer": {
						Type:        "boolean",
						Description: "Enable automatic configuration inference",
					},
					"mcp_port": {
						Type:        "number",
						Description: "Port for MCP server",
						Minimum:     ptr(float64(1024)),
						Maximum:     ptr(float64(65535)),
					},
				},
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
