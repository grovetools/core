package keymap

import (
	"testing"

	"github.com/charmbracelet/bubbles/key"
	"github.com/grovetools/core/config"
)

func TestCamelToSnake(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ViewLogs", "view_logs"},
		{"GoToTop", "go_to_top"},
		{"HTTPServer", "h_t_t_p_server"}, // Edge case with consecutive caps
		{"Up", "up"},
		{"PageUp", "page_up"},
		{"ToggleFullscreen", "toggle_fullscreen"},
		{"A", "a"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := camelToSnake(tt.input)
			if result != tt.expected {
				t.Errorf("camelToSnake(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestKeyMap is a sample keymap for testing
type TestKeyMap struct {
	Base
	ViewLogs    key.Binding
	RunJob      key.Binding
	GoToTop     key.Binding
	unexported  key.Binding // Should be skipped
	NotABinding string      // Should be skipped
}

func TestApplyOverrides(t *testing.T) {
	km := TestKeyMap{
		Base: NewBase(),
		ViewLogs: key.NewBinding(
			key.WithKeys("l"),
			key.WithHelp("l", "view logs"),
		),
		RunJob: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "run job"),
		),
		GoToTop: key.NewBinding(
			key.WithKeys("g"),
			key.WithHelp("gg", "go to top"),
		),
		NotABinding: "not a binding",
	}

	overrides := config.KeybindingSectionConfig{
		"view_logs": []string{"L"},           // Change l to L
		"run_job":   []string{"R", "enter"},  // Change r to R with alt
		"not_a_binding": []string{"x"},       // Should be ignored (not a key.Binding)
	}

	ApplyOverrides(&km, overrides)

	// Check ViewLogs was updated
	if keys := km.ViewLogs.Keys(); len(keys) != 1 || keys[0] != "L" {
		t.Errorf("ViewLogs keys = %v, want [L]", keys)
	}
	if help := km.ViewLogs.Help().Desc; help != "view logs" {
		t.Errorf("ViewLogs help = %q, want %q", help, "view logs")
	}

	// Check RunJob was updated with multiple keys
	if keys := km.RunJob.Keys(); len(keys) != 2 || keys[0] != "R" || keys[1] != "enter" {
		t.Errorf("RunJob keys = %v, want [R enter]", keys)
	}

	// Check GoToTop was NOT updated (no override provided)
	if keys := km.GoToTop.Keys(); len(keys) != 1 || keys[0] != "g" {
		t.Errorf("GoToTop keys = %v, want [g]", keys)
	}

	// Check NotABinding was NOT modified
	if km.NotABinding != "not a binding" {
		t.Errorf("NotABinding = %q, want %q", km.NotABinding, "not a binding")
	}
}

func TestApplyOverrides_NilOverrides(t *testing.T) {
	km := TestKeyMap{
		ViewLogs: key.NewBinding(
			key.WithKeys("l"),
			key.WithHelp("l", "view logs"),
		),
	}

	// Should not panic with nil overrides
	ApplyOverrides(&km, nil)

	// Should remain unchanged
	if keys := km.ViewLogs.Keys(); len(keys) != 1 || keys[0] != "l" {
		t.Errorf("ViewLogs keys = %v, want [l]", keys)
	}
}

func TestApplyOverrides_NonPointer(t *testing.T) {
	km := TestKeyMap{
		ViewLogs: key.NewBinding(
			key.WithKeys("l"),
			key.WithHelp("l", "view logs"),
		),
	}

	overrides := config.KeybindingSectionConfig{
		"view_logs": []string{"L"},
	}

	// Should not panic when passed non-pointer (but won't modify)
	ApplyOverrides(km, overrides)

	// Should remain unchanged since we passed by value
	if keys := km.ViewLogs.Keys(); len(keys) != 1 || keys[0] != "l" {
		t.Errorf("ViewLogs keys = %v, want [l]", keys)
	}
}

func TestApplyOverrides_EmbeddedStruct(t *testing.T) {
	km := TestKeyMap{
		Base: NewBase(),
		ViewLogs: key.NewBinding(
			key.WithKeys("l"),
			key.WithHelp("l", "view logs"),
		),
		RunJob: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "run job"),
		),
	}

	overrides := config.KeybindingSectionConfig{
		"view_logs": []string{"L"},      // Top-level field
		"up":        []string{"w"},      // From embedded Base struct
		"quit":      []string{"Q", "x"}, // From embedded Base struct
	}

	ApplyOverrides(&km, overrides)

	// Check ViewLogs was updated (top-level field)
	if keys := km.ViewLogs.Keys(); len(keys) != 1 || keys[0] != "L" {
		t.Errorf("ViewLogs keys = %v, want [L]", keys)
	}

	// Check Up was updated (embedded Base field)
	if keys := km.Base.Up.Keys(); len(keys) != 1 || keys[0] != "w" {
		t.Errorf("Base.Up keys = %v, want [w]", keys)
	}

	// Check Quit was updated with multiple keys (embedded Base field)
	if keys := km.Base.Quit.Keys(); len(keys) != 2 || keys[0] != "Q" || keys[1] != "x" {
		t.Errorf("Base.Quit keys = %v, want [Q x]", keys)
	}

	// Check RunJob was NOT updated (no override provided)
	if keys := km.RunJob.Keys(); len(keys) != 1 || keys[0] != "r" {
		t.Errorf("RunJob keys = %v, want [r]", keys)
	}

	// Check Down was NOT updated (no override provided, should remain default)
	defaultDownKeys := NewBase().Down.Keys()
	if keys := km.Base.Down.Keys(); len(keys) != len(defaultDownKeys) || keys[0] != defaultDownKeys[0] {
		t.Errorf("Base.Down keys = %v, want %v", keys, defaultDownKeys)
	}
}
