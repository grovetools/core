package schema

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

//go:embed grove.embedded.schema.json
var embeddedSchemaData []byte

// Validator validates configuration against the embedded JSON Schema.
type Validator struct {
	schema *jsonschema.Schema
}

// NewValidator creates a new schema validator, loading the embedded schema.
func NewValidator() (*Validator, error) {
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("grove.json", strings.NewReader(string(embeddedSchemaData))); err != nil {
		return nil, fmt.Errorf("failed to add embedded schema resource: %w", err)
	}

	schema, err := compiler.Compile("grove.json")
	if err != nil {
		return nil, fmt.Errorf("failed to compile embedded schema: %w", err)
	}

	return &Validator{schema: schema}, nil
}

// Validate validates configuration data against the schema.
// It expects the configData to be any struct that can be marshaled to JSON.
func (v *Validator) Validate(configData interface{}) error {
	// Convert the Go struct to a generic map[string]interface{} for validation.
	// This is necessary because the schema expects plain JSON-like objects.
	jsonData, err := json.Marshal(configData)
	if err != nil {
		return fmt.Errorf("failed to marshal config to JSON for validation: %w", err)
	}

	var dataToValidate interface{}
	if err := json.Unmarshal(jsonData, &dataToValidate); err != nil {
		return fmt.Errorf("failed to unmarshal JSON for validation: %w", err)
	}

	if err := v.schema.Validate(dataToValidate); err != nil {
		// Format the validation error to be more user-friendly.
		if validationErr, ok := err.(*jsonschema.ValidationError); ok {
			var errorMessages []string
			collectErrors(validationErr, &errorMessages)
			return fmt.Errorf("schema validation failed:\n%s", strings.Join(errorMessages, "\n"))
		}
		return fmt.Errorf("schema validation failed: %w", err)
	}

	return nil
}

// collectErrors recursively collects all validation errors into a slice
func collectErrors(err *jsonschema.ValidationError, messages *[]string) {
	if err.InstanceLocation != "" {
		*messages = append(*messages, fmt.Sprintf("- %s: %s", err.InstanceLocation, err.Message))
	}
	for _, cause := range err.Causes {
		collectErrors(cause, messages)
	}
}
