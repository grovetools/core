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
		"Version":  "version",
		"Networks": "networks",
		"Services": "services",
		"Volumes":  "volumes",
		"Profiles": "profiles",
		"Agent":    "agent",
		"Settings": "settings",
	}

	for key, value := range data {
		if mappedKey, ok := fieldMapping[key]; ok {
			transformed[mappedKey] = value
		} else {
			transformed[key] = value
		}
	}

	// Remove empty maps that are marshaled as null
	if services, ok := transformed["services"]; ok && services == nil {
		delete(transformed, "services")
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

// ValidateSemantics performs semantic validation beyond schema
func (c *Config) ValidateSemantics() error {
	// Check for circular dependencies
	if err := c.checkCircularDependencies(); err != nil {
		return err
	}

	// Check for port conflicts
	if err := c.checkPortConflicts(); err != nil {
		return err
	}

	// Validate service references
	if err := c.validateServiceReferences(); err != nil {
		return err
	}

	return nil
}

// checkCircularDependencies checks for circular service dependencies
func (c *Config) checkCircularDependencies() error {
	// Build dependency graph
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var hasCycle func(string) bool
	hasCycle = func(service string) bool {
		visited[service] = true
		recStack[service] = true

		if svc, ok := c.Services[service]; ok {
			for _, dep := range svc.DependsOn {
				if !visited[dep] {
					if hasCycle(dep) {
						return true
					}
				} else if recStack[dep] {
					return true
				}
			}
		}

		recStack[service] = false
		return false
	}

	for service := range c.Services {
		if !visited[service] {
			if hasCycle(service) {
				return fmt.Errorf("circular dependency detected involving service: %s", service)
			}
		}
	}

	return nil
}

// checkPortConflicts checks for port binding conflicts
func (c *Config) checkPortConflicts() error {
	portMap := make(map[string]string) // port -> service name

	for name, service := range c.Services {
		for _, portMapping := range service.Ports {
			// Extract host port from mapping (e.g., "8080:80" -> "8080")
			parts := strings.Split(portMapping, ":")
			if len(parts) < 2 {
				continue
			}

			hostPort := parts[0]
			if existingService, exists := portMap[hostPort]; exists {
				return fmt.Errorf("port %s is used by both '%s' and '%s'", hostPort, existingService, name)
			}
			portMap[hostPort] = name
		}
	}

	return nil
}

// validateServiceReferences validates that all service references exist
func (c *Config) validateServiceReferences() error {
	// Check depends_on references
	for name, service := range c.Services {
		for _, dep := range service.DependsOn {
			if _, exists := c.Services[dep]; !exists {
				return fmt.Errorf("service '%s' depends on non-existent service '%s'", name, dep)
			}
		}
	}

	// Check profile references
	for profileName, profile := range c.Profiles {
		for _, serviceName := range profile.Services {
			if _, exists := c.Services[serviceName]; !exists {
				return fmt.Errorf("profile '%s' references non-existent service '%s'", profileName, serviceName)
			}
		}
	}

	return nil
}
