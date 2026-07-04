package schema

import (
	"strings"
	"testing"
)

// inlineConflict reproduces the shape that makes yaml.Marshal panic: an inline
// map carrying a key that conflicts with a sibling struct field. This is
// exactly what a core Config looks like when a struct-owned key (e.g.
// "test_scopes") leaks into the inline Extensions map.
type inlineConflict struct {
	Name       string                 `yaml:"name"`
	Extensions map[string]interface{} `yaml:",inline"`
}

// TestValidateRecoversYAMLMarshalPanic locks in the panic-proofing contract:
// Validate is used for warn-only advisory validation, so a yaml.Marshal panic
// (which yaml.v3 raises instead of returning an error for inline-map key
// conflicts) must come back as a returned error, never crash the process.
func TestValidateRecoversYAMLMarshalPanic(t *testing.T) {
	v, err := NewValidator()
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}

	err = v.Validate(inlineConflict{
		Name:       "x",
		Extensions: map[string]interface{}{"name": "boom"},
	})
	if err == nil {
		t.Fatal("expected an error from the recovered marshal panic, got nil")
	}
	if !strings.Contains(err.Error(), "panicked") {
		t.Errorf("error should identify the recovered panic, got: %v", err)
	}
}
