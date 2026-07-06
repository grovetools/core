package keymap

import (
	"testing"

	"github.com/charmbracelet/bubbles/key"

	"github.com/grovetools/core/config"
)

// acronymKeyMap has an acronym-run field to exercise the wrapper's override path.
type acronymKeyMap struct {
	Base
	ViewJSON key.Binding
}

// TestApplyOverrides_AcronymRoundTrip proves the export identity equals the
// override identity for acronym-run fields: the registry advertises
// "view_json" (acronym-aware), so ApplyOverrides keyed by "view_json" must
// rebind ViewJSON. Before the wrapper, the override path derived
// "view_j_s_o_n" and this would silently fail to rebind.
func TestApplyOverrides_AcronymRoundTrip(t *testing.T) {
	km := acronymKeyMap{
		Base:     NewBase(),
		ViewJSON: key.NewBinding(key.WithKeys("j"), key.WithHelp("j", "view json")),
	}

	ApplyOverrides(&km, config.KeybindingSectionConfig{"view_json": []string{"J"}})

	if keys := km.ViewJSON.Keys(); len(keys) != 1 || keys[0] != "J" {
		t.Errorf("ViewJSON keys = %v, want [J] (override key %q must match exported ConfigKey)", keys, "view_json")
	}
}

// TestSnakeConverterEquivalence pins the Phase-F1 wrapper: camelToSnake now
// delegates to toSnakeCase, so both must produce the same acronym-aware
// output. The reflection export path (MakeTUIInfo) and the override-key
// derivation (applyOverridesRecursive) both go through camelToSnake; the
// advertised config handle must match what ConfigKeyForField/toSnakeCase emit.
func TestSnakeConverterEquivalence(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ViewJSON", "view_json"},
		{"ExportJSON", "export_json"},
		{"ScrollTUILeft", "scroll_tui_left"},
		{"Tab1", "tab1"},
		{"PageUp", "page_up"},
		{"already_snake", "already_snake"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := toSnakeCase(tt.input); got != tt.expected {
				t.Errorf("toSnakeCase(%q) = %q, want %q", tt.input, got, tt.expected)
			}
			if got := camelToSnake(tt.input); got != tt.expected {
				t.Errorf("camelToSnake(%q) = %q, want %q (wrapper must equal toSnakeCase)", tt.input, got, tt.expected)
			}
		})
	}
}
