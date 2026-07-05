package jsontree

import (
	"testing"

	"github.com/charmbracelet/bubbles/key"

	"github.com/grovetools/core/tui/keymap"
)

// TestKeyMapAuditCoverage asserts every enabled binding in the jsontree
// KeyMap appears in a section and that no help label contradicts its keys.
func TestKeyMapAuditCoverage(t *testing.T) {
	if gaps := keymap.AuditCoverage(DefaultKeyMap()); len(gaps) != 0 {
		for _, g := range gaps {
			t.Errorf("audit gap: field=%s kind=%s detail=%s", g.Field, g.Kind, g.Detail)
		}
	}
}

// TestChordKeysMatchLabels pins the label-lie fix independently of the audit's
// label heuristics: the chord bindings must key exactly what their help claims.
func TestChordKeysMatchLabels(t *testing.T) {
	km := DefaultKeyMap()
	cases := []struct {
		name    string
		binding key.Binding
	}{
		{"GotoTop", km.GotoTop},
		{"ExpandAll", km.ExpandAll},
		{"CollapseAll", km.CollapseAll},
	}
	for _, c := range cases {
		keys := c.binding.Keys()
		if len(keys) == 0 {
			t.Fatalf("%s has no keys", c.name)
		}
		if got, want := keys[0], c.binding.Help().Key; got != want {
			t.Errorf("%s: Keys()[0]=%q but Help().Key=%q", c.name, got, want)
		}
	}
}
