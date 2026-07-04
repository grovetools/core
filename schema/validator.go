package schema

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"gopkg.in/yaml.v3"
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
//
// configData may be a *config.Config struct or an already-decoded config
// document (map[string]interface{}). Serialization goes through YAML rather
// than JSON on purpose: the core Config struct carries yaml/toml tags but no
// json tags, so json.Marshal would emit Go field names ("Groves",
// "SearchPaths", …) that can never match the schema's snake_case properties —
// making validation silently vacuous. YAML marshaling honors the same field
// names the schema was generated from (the reflector uses FieldNameTag:
// "yaml"). Values that are already generic maps pass through unchanged.
func (v *Validator) Validate(configData interface{}) error {
	yamlData, err := yaml.Marshal(configData)
	if err != nil {
		return fmt.Errorf("failed to marshal config to YAML for validation: %w", err)
	}

	var dataToValidate interface{}
	if err := yaml.Unmarshal(yamlData, &dataToValidate); err != nil {
		return fmt.Errorf("failed to unmarshal config for validation: %w", err)
	}

	// Normalize to plain JSON value types (float64 numbers, string-keyed maps)
	// so the JSON Schema validator sees the shapes it expects, regardless of
	// whether the value originated from a struct, TOML, or YAML.
	dataToValidate, err = toJSONValue(dataToValidate)
	if err != nil {
		return fmt.Errorf("failed to normalize config for validation: %w", err)
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

// toJSONValue round-trips a value through encoding/json so downstream
// consumers see canonical JSON types. YAML decoding yields int for whole
// numbers and TOML yields int64/time.Time; the schema validator is happiest
// with the float64/string/bool/[]interface{}/map[string]interface{} shapes
// that json.Unmarshal into interface{} produces.
func toJSONValue(v interface{}) (interface{}, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out interface{}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
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
