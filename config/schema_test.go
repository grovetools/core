package config

import (
	"encoding/json"
	"testing"
)

func TestGenerateSchema(t *testing.T) {
	schema := GenerateSchema()

	// Test basic structure
	if schema.Schema != "http://json-schema.org/draft-07/schema#" {
		t.Errorf("expected JSON Schema draft-07, got %s", schema.Schema)
	}

	if schema.Type != "object" {
		t.Errorf("expected root type to be object, got %s", schema.Type)
	}

	// Test required fields
	if len(schema.Required) == 0 {
		t.Error("expected required fields")
	}

	found := false
	for _, req := range schema.Required {
		if req == "services" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'services' to be required")
	}

	// Test properties exist
	if schema.Properties == nil {
		t.Fatal("expected properties to be defined")
	}

	// Test services property
	if services, ok := schema.Properties["services"]; ok {
		if services.Type != "object" {
			t.Errorf("expected services type to be object, got %s", services.Type)
		}
	} else {
		t.Error("expected services property")
	}

	// Test settings property
	if settings, ok := schema.Properties["settings"]; ok {
		if settings.Properties == nil {
			t.Error("expected settings to have properties")
		}

		// Check specific settings properties
		if projectName, ok := settings.Properties["project_name"]; ok {
			if projectName.Pattern == "" {
				t.Error("expected project_name to have pattern validation")
			}
		}
	} else {
		t.Error("expected settings property")
	}
}

func TestExportSchema(t *testing.T) {
	data, err := ExportSchema()
	if err != nil {
		t.Fatal(err)
	}

	// Verify it's valid JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Errorf("exported schema is not valid JSON: %v", err)
	}

	// Verify it contains expected top-level keys
	expectedKeys := []string{"$schema", "type", "title", "description", "properties", "required"}
	for _, key := range expectedKeys {
		if _, ok := parsed[key]; !ok {
			t.Errorf("expected key '%s' in schema", key)
		}
	}
}

func TestSchemaServiceProperties(t *testing.T) {
	schema := GenerateSchema()

	services := schema.Properties["services"]
	if services == nil {
		t.Fatal("services property not found")
	}

	// Get the additional properties schema for services
	if services.AdditionalProperties == nil {
		t.Fatal("expected services to have additionalProperties")
	}

	serviceSchema, ok := services.AdditionalProperties.(*JSONSchema)
	if !ok {
		t.Fatal("services additionalProperties should be a JSONSchema")
	}

	// Check service properties
	expectedProps := []string{"build", "image", "ports", "volumes", "environment", "depends_on", "labels", "command", "profiles"}
	for _, prop := range expectedProps {
		if _, ok := serviceSchema.Properties[prop]; !ok {
			t.Errorf("expected service property '%s'", prop)
		}
	}

	// Test port validation
	if ports, ok := serviceSchema.Properties["ports"]; ok {
		if ports.Type != "array" {
			t.Errorf("expected ports to be array, got %s", ports.Type)
		}
		if ports.Items == nil {
			t.Error("expected ports to have items schema")
		} else if ports.Items.Pattern == "" {
			t.Error("expected ports items to have pattern validation")
		}
	}
}

func TestSchemaProfileProperties(t *testing.T) {
	schema := GenerateSchema()

	profiles := schema.Properties["profiles"]
	if profiles == nil {
		t.Fatal("profiles property not found")
	}

	if profiles.AdditionalProperties == nil {
		t.Fatal("expected profiles to have additionalProperties")
	}

	profileSchema, ok := profiles.AdditionalProperties.(*JSONSchema)
	if !ok {
		t.Fatal("profiles additionalProperties should be a JSONSchema")
	}

	// Check profile properties
	if services, ok := profileSchema.Properties["services"]; ok {
		if services.Type != "array" {
			t.Errorf("expected profile services to be array, got %s", services.Type)
		}
	} else {
		t.Error("expected profile to have services property")
	}
}
