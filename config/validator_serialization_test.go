package config

import "testing"

// TestSchemaValidatorSnakeCaseSerialization locks in the fix for the tag
// mismatch: the validator serializes via YAML (snake_case, matching the schema
// property names) instead of json.Marshal of the tagless Config struct, which
// used to emit Go field names that never matched — making validation vacuous.
func TestSchemaValidatorSnakeCaseSerialization(t *testing.T) {
	v, err := NewSchemaValidator()
	if err != nil {
		t.Fatalf("NewSchemaValidator: %v", err)
	}

	t.Run("valid struct passes", func(t *testing.T) {
		cfg := &Config{Version: "1.0", Workspaces: []string{"*"}}
		if err := v.Validate(cfg); err != nil {
			t.Fatalf("expected valid config to pass, got: %v", err)
		}
	})

	t.Run("wrong-typed known field is now caught", func(t *testing.T) {
		// version is typed string in the schema. Before the fix this passed
		// vacuously because the marshaled key was "Version", not "version".
		bad := map[string]interface{}{"version": 123}
		if err := v.Validate(bad); err == nil {
			t.Fatal("expected a schema violation for version:int, got nil")
		}
	})

	t.Run("schema violation is caught for a real drift field", func(t *testing.T) {
		// tui.leader_key is a real TUIConfig field the schema shadow struct
		// omits (TUISchemaConfig is additionalProperties:false), so it is a
		// genuine drift case the corrected validator now flags.
		drift := map[string]interface{}{
			"version": "1.0",
			"tui":     map[string]interface{}{"leader_key": "space"},
		}
		if err := v.Validate(drift); err == nil {
			t.Fatal("expected a schema violation for tui.leader_key, got nil")
		}
	})

	t.Run("extension namespaces do not warn", func(t *testing.T) {
		// Top-level extension keys serialize inline; the schema permits
		// additional properties there, so legitimate namespaces must not trip
		// validation.
		cfg := &Config{
			Version:    "1.0",
			Extensions: map[string]interface{}{"flow": map[string]interface{}{"oneshot_model": "x"}},
		}
		if err := v.Validate(cfg); err != nil {
			t.Fatalf("extension namespace should not fail validation, got: %v", err)
		}
	})
}

// TestLoadFromTOMLBytesValidationIsNonFatal guards the ecosystem-wide contract:
// now that validation actually compares snake_case keys, a schema violation on
// the single-file load path (config.Load, used across every repo) must warn,
// not fail — forward-compat and drift keys have to keep loading.
func TestLoadFromTOMLBytesValidationIsNonFatal(t *testing.T) {
	// tui.leader_key violates the schema (see above) but is a real, parseable
	// field. Loading must succeed and preserve the value.
	data := []byte("version = \"1.0\"\n[tui]\nleader_key = \"space\"\n")
	cfg, err := LoadFromTOMLBytes(data)
	if err != nil {
		t.Fatalf("schema violation must not fail LoadFromTOMLBytes, got: %v", err)
	}
	if cfg.TUI == nil || cfg.TUI.LeaderKey != "space" {
		t.Fatalf("expected tui.leader_key to load despite the violation, got: %+v", cfg.TUI)
	}
}
