package keymap

import (
	"testing"

	"github.com/grovetools/core/config"
)

func TestDefaultVim(t *testing.T) {
	km := DefaultVim()

	// Test navigation keys
	if keys := km.Up.Keys(); len(keys) < 1 || keys[0] != "k" {
		t.Errorf("Expected Up to have 'k' as first key, got %v", keys)
	}
	if keys := km.Down.Keys(); len(keys) < 1 || keys[0] != "j" {
		t.Errorf("Expected Down to have 'j' as first key, got %v", keys)
	}

	// Test fold keys
	if keys := km.FoldOpen.Keys(); len(keys) < 1 || keys[0] != "zo" {
		t.Errorf("Expected FoldOpen to have 'zo' as key, got %v", keys)
	}
	if keys := km.FoldClose.Keys(); len(keys) < 1 || keys[0] != "zc" {
		t.Errorf("Expected FoldClose to have 'zc' as key, got %v", keys)
	}

	// Test sequence keys
	if keys := km.Top.Keys(); len(keys) < 1 || keys[0] != "gg" {
		t.Errorf("Expected Top to have 'gg' as key, got %v", keys)
	}
	if keys := km.Delete.Keys(); len(keys) < 1 || keys[0] != "dd" {
		t.Errorf("Expected Delete to have 'dd' as key, got %v", keys)
	}
}

func TestDefaultEmacs(t *testing.T) {
	km := DefaultEmacs()

	// Test emacs navigation overrides
	if keys := km.Up.Keys(); len(keys) < 1 || keys[0] != "ctrl+p" {
		t.Errorf("Expected Up to have 'ctrl+p' as first key, got %v", keys)
	}
	if keys := km.Down.Keys(); len(keys) < 1 || keys[0] != "ctrl+n" {
		t.Errorf("Expected Down to have 'ctrl+n' as first key, got %v", keys)
	}
	if keys := km.Search.Keys(); len(keys) < 1 || keys[0] != "ctrl+s" {
		t.Errorf("Expected Search to have 'ctrl+s' as first key, got %v", keys)
	}
}

func TestDefaultArrows(t *testing.T) {
	km := DefaultArrows()

	// Test arrow navigation
	if keys := km.Up.Keys(); len(keys) < 1 || keys[0] != "up" {
		t.Errorf("Expected Up to have 'up' as first key, got %v", keys)
	}
	if keys := km.Down.Keys(); len(keys) < 1 || keys[0] != "down" {
		t.Errorf("Expected Down to have 'down' as first key, got %v", keys)
	}

	// Test simplified actions
	if keys := km.Delete.Keys(); len(keys) < 1 || keys[0] != "delete" {
		t.Errorf("Expected Delete to have 'delete' as first key, got %v", keys)
	}
}

func TestLoad_NilConfig(t *testing.T) {
	km := Load(nil, "")

	// Should return vim defaults
	if keys := km.Up.Keys(); len(keys) < 1 || keys[0] != "k" {
		t.Errorf("Expected vim-style Up key, got %v", keys)
	}
}

func TestLoad_PresetSelection(t *testing.T) {
	tests := []struct {
		preset   string
		expected string // Expected first key for Up
	}{
		{"vim", "k"},
		{"emacs", "ctrl+p"},
		{"arrows", "up"},
		{"", "k"},          // Default
		{"unknown", "k"},   // Unknown falls back to vim
	}

	for _, tt := range tests {
		t.Run(tt.preset, func(t *testing.T) {
			cfg := &config.Config{
				TUI: &config.TUIConfig{
					Preset: tt.preset,
				},
			}
			km := Load(cfg, "")

			keys := km.Up.Keys()
			if len(keys) < 1 || keys[0] != tt.expected {
				t.Errorf("Preset %q: expected Up=%q, got %v", tt.preset, tt.expected, keys)
			}
		})
	}
}

func TestLoad_GlobalOverrides(t *testing.T) {
	cfg := &config.Config{
		TUI: &config.TUIConfig{
			Preset: "vim",
			Keybindings: &config.KeybindingsConfig{
				Navigation: config.KeybindingSectionConfig{
					"up":   {"w"},
					"down": {"s"},
				},
				Actions: config.KeybindingSectionConfig{
					"delete": {"x"},
				},
			},
		},
	}

	km := Load(cfg, "")

	// Check navigation overrides
	if keys := km.Up.Keys(); len(keys) < 1 || keys[0] != "w" {
		t.Errorf("Expected Up='w', got %v", keys)
	}
	if keys := km.Down.Keys(); len(keys) < 1 || keys[0] != "s" {
		t.Errorf("Expected Down='s', got %v", keys)
	}

	// Check action overrides
	if keys := km.Delete.Keys(); len(keys) < 1 || keys[0] != "x" {
		t.Errorf("Expected Delete='x', got %v", keys)
	}

	// Check unchanged keys
	if keys := km.Left.Keys(); len(keys) < 1 || keys[0] != "h" {
		t.Errorf("Expected Left='h' (unchanged), got %v", keys)
	}
}

func TestLoad_TUISpecificOverrides(t *testing.T) {
	cfg := &config.Config{
		TUI: &config.TUIConfig{
			Preset: "vim",
			Keybindings: &config.KeybindingsConfig{
				Navigation: config.KeybindingSectionConfig{
					"up": {"w"}, // Global override
				},
				Overrides: map[string]config.KeybindingSectionConfig{
					"nb.browser": {
						"up": {"i"}, // TUI-specific override
					},
				},
			},
		},
	}

	// Without TUI name, should use global override
	km := Load(cfg, "")
	if keys := km.Up.Keys(); len(keys) < 1 || keys[0] != "w" {
		t.Errorf("Without TUI: expected Up='w', got %v", keys)
	}

	// With TUI name, should use TUI-specific override
	km = Load(cfg, "nb.browser")
	if keys := km.Up.Keys(); len(keys) < 1 || keys[0] != "i" {
		t.Errorf("With TUI nb.browser: expected Up='i', got %v", keys)
	}

	// With different TUI name, should use global override
	km = Load(cfg, "flow.status")
	if keys := km.Up.Keys(); len(keys) < 1 || keys[0] != "w" {
		t.Errorf("With TUI flow.status: expected Up='w', got %v", keys)
	}
}

func TestLoad_FoldOverrides(t *testing.T) {
	cfg := &config.Config{
		TUI: &config.TUIConfig{
			Keybindings: &config.KeybindingsConfig{
				Fold: config.KeybindingSectionConfig{
					"open":      {"o"},
					"close":     {"c"},
					"toggle":    {"t"},
					"open_all":  {"O"},
					"close_all": {"C"},
				},
			},
		},
	}

	km := Load(cfg, "")

	if keys := km.FoldOpen.Keys(); len(keys) < 1 || keys[0] != "o" {
		t.Errorf("Expected FoldOpen='o', got %v", keys)
	}
	if keys := km.FoldClose.Keys(); len(keys) < 1 || keys[0] != "c" {
		t.Errorf("Expected FoldClose='c', got %v", keys)
	}
	if keys := km.FoldToggle.Keys(); len(keys) < 1 || keys[0] != "t" {
		t.Errorf("Expected FoldToggle='t', got %v", keys)
	}
	if keys := km.FoldOpenAll.Keys(); len(keys) < 1 || keys[0] != "O" {
		t.Errorf("Expected FoldOpenAll='O', got %v", keys)
	}
	if keys := km.FoldCloseAll.Keys(); len(keys) < 1 || keys[0] != "C" {
		t.Errorf("Expected FoldCloseAll='C', got %v", keys)
	}
}

func TestLoad_MultipleKeys(t *testing.T) {
	cfg := &config.Config{
		TUI: &config.TUIConfig{
			Keybindings: &config.KeybindingsConfig{
				Navigation: config.KeybindingSectionConfig{
					"up": {"w", "k", "up"}, // Multiple keys
				},
			},
		},
	}

	km := Load(cfg, "")

	keys := km.Up.Keys()
	if len(keys) != 3 {
		t.Errorf("Expected 3 keys for Up, got %d: %v", len(keys), keys)
	}
	if keys[0] != "w" || keys[1] != "k" || keys[2] != "up" {
		t.Errorf("Expected ['w', 'k', 'up'], got %v", keys)
	}
}

func TestNewBase(t *testing.T) {
	km := NewBase()

	// NewBase should return vim defaults
	if keys := km.Up.Keys(); len(keys) < 1 || keys[0] != "k" {
		t.Errorf("NewBase should return vim defaults, got Up=%v", keys)
	}
}

func TestFullHelp(t *testing.T) {
	km := DefaultVim()
	help := km.FullHelp()

	// Should have multiple groups
	if len(help) < 5 {
		t.Errorf("Expected at least 5 help groups, got %d", len(help))
	}

	// Check that all groups have bindings
	for i, group := range help {
		if len(group) == 0 {
			t.Errorf("Help group %d is empty", i)
		}
	}
}

func TestShortHelp(t *testing.T) {
	km := DefaultVim()
	help := km.ShortHelp()

	// Short help should include quit
	if len(help) < 1 {
		t.Errorf("Expected at least 1 binding in short help")
	}

	// First should be quit
	if help[0].Keys()[0] != km.Quit.Keys()[0] {
		t.Errorf("Expected Quit in short help")
	}
}
