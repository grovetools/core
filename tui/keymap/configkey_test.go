package keymap

import "testing"

// TestConfigKeyForField pins the acronym-aware CamelCase->snake_case conversion
// that hand-authored TUIInfo exports (treemux/tuimux) rely on. This is the same
// table Phase F's converter unification must satisfy, so it doubles as the F pin.
func TestConfigKeyForField(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"ViewJSON", "view_json"},
		{"ScrollTUILeft", "scroll_tui_left"},
		{"Tab1", "tab1"},
		{"PageUp", "page_up"},
		{"already_snake", "already_snake"},
	}
	for _, c := range cases {
		if got := ConfigKeyForField(c.name); got != c.want {
			t.Errorf("ConfigKeyForField(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}
