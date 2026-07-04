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

	t.Run("schema violation is caught for an unknown nested key", func(t *testing.T) {
		// TUIConfig is additionalProperties:false in the schema, so a key that
		// exists in no version of the struct must trip validation — proving the
		// validator compares real snake_case property names.
		drift := map[string]interface{}{
			"version": "1.0",
			"tui":     map[string]interface{}{"not_a_real_tui_key": "x"},
		}
		if err := v.Validate(drift); err == nil {
			t.Fatal("expected a schema violation for tui.not_a_real_tui_key, got nil")
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
	// The unknown tui key violates the schema (see above) but the file is
	// parseable. Loading must succeed and preserve the sibling value.
	data := []byte("version = \"1.0\"\n[tui]\nleader_key = \"space\"\nnot_a_real_tui_key = \"x\"\n")
	cfg, err := LoadFromTOMLBytes(data)
	if err != nil {
		t.Fatalf("schema violation must not fail LoadFromTOMLBytes, got: %v", err)
	}
	if cfg.TUI == nil || cfg.TUI.LeaderKey != "space" {
		t.Fatalf("expected tui.leader_key to load despite the violation, got: %+v", cfg.TUI)
	}
}

// TestLoadFromTOMLBytes_CoreKeysNotLeakedToExtensions locks in the
// coreConfigKeys derivation fix: "commands" and "test_scopes" ARE core Config
// struct fields, but the hand-maintained key list omitted them, so the TOML
// extension-capture loop copied them into the inline Extensions map — and
// yaml.Marshal then PANICKED during warn-only validation ("cannot have key
// test_scopes in inlined map: conflicts with struct field"), crashing any
// binary that loaded such a grove.toml. This load previously panicked.
func TestLoadFromTOMLBytes_CoreKeysNotLeakedToExtensions(t *testing.T) {
	data := []byte(`version = "1.0"

[commands]
build = "make build"

[[test_scopes]]
name = "orchestration"
rules = ".grove/rules.d/orchestration.rules"
scenarios = ["coordinator-workflow"]
`)

	cfg, err := LoadFromTOMLBytes(data)
	if err != nil {
		t.Fatalf("LoadFromTOMLBytes with [commands]/[[test_scopes]] must load cleanly, got: %v", err)
	}

	// Struct fields must carry the values...
	if cfg.Commands["build"] != "make build" {
		t.Errorf("Commands not parsed into struct field: %+v", cfg.Commands)
	}
	if len(cfg.TestScopes) != 1 || cfg.TestScopes[0].Name != "orchestration" {
		t.Errorf("TestScopes not parsed into struct field: %+v", cfg.TestScopes)
	}

	// ...and the keys must NOT be duplicated into Extensions (the duplication
	// is what made yaml.Marshal panic on the inline map).
	if _, ok := cfg.Extensions["commands"]; ok {
		t.Error("core key 'commands' leaked into Extensions")
	}
	if _, ok := cfg.Extensions["test_scopes"]; ok {
		t.Error("core key 'test_scopes' leaked into Extensions")
	}
}

// TestCoreConfigKeysTrackStructTags guards the derivation itself: every
// top-level Config tag name must be in the set, including the two that
// historically drifted.
func TestCoreConfigKeysTrackStructTags(t *testing.T) {
	for _, key := range []string{"name", "version", "workspaces", "commands", "test_scopes", "groves", "worktree", "_grove"} {
		if !coreConfigKeys[key] {
			t.Errorf("coreConfigKeys missing %q", key)
		}
	}
}
