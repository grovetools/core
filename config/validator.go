package config

import (
	"github.com/grovetools/core/schema"
)

// SchemaValidator validates configuration against the embedded JSON Schema.
// This is a wrapper around schema.Validator to maintain backward compatibility
// with existing code that uses config.SchemaValidator.
type SchemaValidator struct {
	validator *schema.Validator
}

// NewSchemaValidator creates a new schema validator, loading the embedded schema.
func NewSchemaValidator() (*SchemaValidator, error) {
	validator, err := schema.NewValidator()
	if err != nil {
		return nil, err
	}
	return &SchemaValidator{validator: validator}, nil
}

// Validate validates configuration data against the schema.
func (v *SchemaValidator) Validate(configData interface{}) error {
	return v.validator.Validate(configData)
}

