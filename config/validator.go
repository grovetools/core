package config

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// SchemaValidator validates configuration against JSON Schema
type SchemaValidator struct {
	schema *JSONSchema
}

// NewSchemaValidator creates a new schema validator
func NewSchemaValidator() (*SchemaValidator, error) {
	schema := GenerateSchema()
	return &SchemaValidator{schema: schema}, nil
}

// Validate validates configuration data against the schema
func (v *SchemaValidator) Validate(configData interface{}) error {
	// Convert config to JSON for validation
	jsonData, err := json.Marshal(configData)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Convert back to map for validation
	var data map[string]interface{}
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Transform capitalized field names to lowercase
	transformed := make(map[string]interface{})
	fieldMapping := map[string]string{
		"Version": "version",
		"Agent":   "agent",
	}

	for key, value := range data {
		if mappedKey, ok := fieldMapping[key]; ok {
			transformed[mappedKey] = value
		} else {
			transformed[key] = value
		}
	}

	// Validate against schema
	errors := v.validateObject(transformed, v.schema, "")
	if len(errors) > 0 {
		return fmt.Errorf("configuration validation failed:\n%s", strings.Join(errors, "\n"))
	}

	return nil
}

// validateObject validates an object against a schema
func (v *SchemaValidator) validateObject(data interface{}, schema *JSONSchema, path string) []string {
	var errors []string

	if schema == nil {
		return errors
	}

	// Type validation
	if schema.Type != "" {
		if !v.checkType(data, schema.Type) {
			errors = append(errors, fmt.Sprintf("- %s: expected type %s", path, schema.Type))
			return errors
		}
	}

	// Object-specific validation
	if schema.Type == "object" && data != nil {
		obj, ok := data.(map[string]interface{})
		if !ok {
			errors = append(errors, fmt.Sprintf("- %s: expected object", path))
			return errors
		}

		// Required fields
		for _, required := range schema.Required {
			if _, exists := obj[required]; !exists {
				errors = append(errors, fmt.Sprintf("- %s: missing required field '%s'", path, required))
			}
		}

		// Validate properties
		if schema.Properties != nil {
			for key, value := range obj {
				if propSchema, exists := schema.Properties[key]; exists {
					propPath := path
					if propPath == "" {
						propPath = key
					} else {
						propPath = fmt.Sprintf("%s.%s", path, key)
					}
					errors = append(errors, v.validateObject(value, propSchema, propPath)...)
				}
			}
		}

		// Additional properties validation
		if schema.AdditionalProperties != nil {
			if additionalSchema, ok := schema.AdditionalProperties.(*JSONSchema); ok {
				for key, value := range obj {
					if schema.Properties == nil || schema.Properties[key] == nil {
						propPath := fmt.Sprintf("%s.%s", path, key)
						errors = append(errors, v.validateObject(value, additionalSchema, propPath)...)
					}
				}
			}
		}
	}

	// Array validation
	if schema.Type == "array" && data != nil {
		arr, ok := data.([]interface{})
		if !ok {
			errors = append(errors, fmt.Sprintf("- %s: expected array", path))
			return errors
		}

		if schema.Items != nil {
			for i, item := range arr {
				itemPath := fmt.Sprintf("%s[%d]", path, i)
				errors = append(errors, v.validateObject(item, schema.Items, itemPath)...)
			}
		}
	}

	// String validation
	if schema.Type == "string" && data != nil {
		str, ok := data.(string)
		if !ok {
			errors = append(errors, fmt.Sprintf("- %s: expected string", path))
			return errors
		}

		// Pattern validation
		if schema.Pattern != "" {
			if matched, err := regexp.MatchString(schema.Pattern, str); err != nil || !matched {
				errors = append(errors, fmt.Sprintf("- %s: value '%s' does not match pattern %s", path, str, schema.Pattern))
			}
		}

		// Length validation
		if schema.MinLength != nil && len(str) < *schema.MinLength {
			errors = append(errors, fmt.Sprintf("- %s: value length %d is less than minimum %d", path, len(str), *schema.MinLength))
		}
		if schema.MaxLength != nil && len(str) > *schema.MaxLength {
			errors = append(errors, fmt.Sprintf("- %s: value length %d exceeds maximum %d", path, len(str), *schema.MaxLength))
		}
	}

	// Enum validation
	if len(schema.Enum) > 0 {
		found := false
		for _, enumVal := range schema.Enum {
			if v.equal(data, enumVal) {
				found = true
				break
			}
		}
		if !found {
			errors = append(errors, fmt.Sprintf("- %s: value must be one of %v", path, schema.Enum))
		}
	}

	return errors
}

// checkType checks if data matches the expected type
func (v *SchemaValidator) checkType(data interface{}, expectedType string) bool {
	if data == nil {
		return true // null is valid for any type unless required
	}

	switch expectedType {
	case "object":
		_, ok := data.(map[string]interface{})
		return ok
	case "array":
		_, ok := data.([]interface{})
		return ok
	case "string":
		_, ok := data.(string)
		return ok
	case "number":
		switch data.(type) {
		case float64, float32, int, int32, int64:
			return true
		}
		return false
	case "boolean":
		_, ok := data.(bool)
		return ok
	}

	return false
}

// equal checks if two values are equal
func (v *SchemaValidator) equal(a, b interface{}) bool {
	// Simple equality check
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

