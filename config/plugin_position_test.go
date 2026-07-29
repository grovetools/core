package config

import "testing"

// TestPluginPositionEnum pins the [tui.plugins] position contract. "ephemeral"
// was in the enum for the schema's whole life but no code ever consumed it —
// a pane declared that way silently never appeared. It is now a schema
// violation (warn-only on every load path, like every other violation), which
// is the signal that points users at [tui.panels.bindings] instead.
func TestPluginPositionEnum(t *testing.T) {
	v, err := NewSchemaValidator()
	if err != nil {
		t.Fatalf("NewSchemaValidator: %v", err)
	}

	pluginCfg := func(position string) map[string]interface{} {
		return map[string]interface{}{
			"version": "1.0",
			"tui": map[string]interface{}{
				"plugins": map[string]interface{}{
					"btop": map[string]interface{}{
						"command":  "btop",
						"position": position,
					},
				},
			},
		}
	}

	if err := v.Validate(pluginCfg("rail")); err != nil {
		t.Errorf(`position = "rail" must validate, got: %v`, err)
	}
	if err := v.Validate(pluginCfg("ephemeral")); err == nil {
		t.Error(`expected a schema violation for the retired position = "ephemeral", got nil`)
	}
}

// TestPluginPositionViolationIsNonFatal guards the load contract: a config
// still carrying the retired value must warn, not fail — the rest of the
// plugin (and the rest of grove.toml) has to keep loading.
func TestPluginPositionViolationIsNonFatal(t *testing.T) {
	data := []byte("version = \"1.0\"\n[tui.plugins.btop]\ncommand = \"btop\"\nposition = \"ephemeral\"\n")
	cfg, err := LoadFromTOMLBytes(data)
	if err != nil {
		t.Fatalf("a retired position value must not fail the load, got: %v", err)
	}
	if cfg.TUI == nil || cfg.TUI.Plugins["btop"] == nil || cfg.TUI.Plugins["btop"].Command != "btop" {
		t.Fatalf("expected the plugin entry to load despite the violation, got: %+v", cfg.TUI)
	}
}
