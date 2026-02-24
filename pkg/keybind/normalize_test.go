package keybind

import (
	"testing"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		source   string
		expected string
	}{
		// Fish notation
		{"fish ctrl-t", "\\ct", "fish", "C-T"},
		{"fish ctrl-p", "\\cp", "fish", "C-P"},
		{"fish meta-f", "\\ef", "fish", "M-F"},
		{"fish meta-b", "\\eb", "fish", "M-B"},
		{"fish ctrl-meta-x", "\\e\\cx", "fish", "C-M-X"}, // Normalize always puts C- before M-
		{"fish escape", "\\e", "fish", "Escape"},
		{"fish tab", "\\t", "fish", "Tab"},
		{"fish enter", "\\r", "fish", "Enter"},

		// Bash notation
		{"bash ctrl-t", "\\C-t", "bash", "C-T"},
		{"bash ctrl-p", "\\C-p", "bash", "C-P"},
		{"bash meta-f", "\\M-f", "bash", "M-F"},
		{"bash meta-b", "\\M-b", "bash", "M-B"},
		{"bash ctrl-meta-x", "\\C-\\M-x", "bash", "C-M-X"},
		{"bash meta-ctrl-x", "\\M-\\C-x", "bash", "C-M-X"},
		{"bash escape", "\\e", "bash", "Escape"},
		{"bash quoted", "\"\\C-t\"", "bash", "C-T"},

		// Zsh notation
		{"zsh ctrl-t", "^T", "zsh", "C-T"},
		{"zsh ctrl-p", "^P", "zsh", "C-P"},
		{"zsh meta-f", "^[f", "zsh", "M-F"},
		{"zsh meta-b", "^[b", "zsh", "M-B"},
		{"zsh meta-ctrl-x", "^[^X", "zsh", "M-C-X"},
		{"zsh escape", "^[", "zsh", "Escape"},
		{"zsh quoted", "\"^T\"", "zsh", "C-T"},

		// Tmux notation (already standard, normalize case)
		{"tmux ctrl-t", "C-t", "tmux", "C-T"},
		{"tmux ctrl-p", "C-p", "tmux", "C-P"},
		{"tmux meta-f", "M-f", "tmux", "M-F"},
		{"tmux ctrl-meta-x", "C-M-x", "tmux", "C-M-X"},
		{"tmux shift-tab", "S-Tab", "tmux", "S-Tab"},

		// Grove notation (same as tmux)
		{"grove ctrl-g", "C-g", "grove", "C-G"},
		{"grove meta-w", "M-w", "grove", "M-W"},

		// Auto-detection
		{"auto fish ctrl", "\\cp", "", "C-P"},
		{"auto bash ctrl", "\\C-p", "", "C-P"},
		{"auto zsh ctrl", "^P", "", "C-P"},
		{"auto tmux ctrl", "C-p", "", "C-P"},

		// Special keys
		{"enter", "enter", "", "Enter"},
		{"return", "return", "", "Enter"},
		{"escape", "escape", "", "Escape"},
		{"esc", "esc", "", "Escape"},
		{"tab", "tab", "", "Tab"},
		{"space", "space", "", "Space"},
		{"backspace", "backspace", "", "Backspace"},
		{"delete", "delete", "", "Delete"},
		{"up", "up", "", "Up"},
		{"down", "down", "", "Down"},
		{"left", "left", "", "Left"},
		{"right", "right", "", "Right"},
		{"home", "home", "", "Home"},
		{"end", "end", "", "End"},
		{"pageup", "pageup", "", "PageUp"},
		{"pagedown", "pagedown", "", "PageDown"},
		{"f1", "f1", "", "F1"},
		{"F12", "F12", "", "F12"},

		// Single characters
		{"single a", "a", "", "A"},
		{"single z", "z", "", "Z"},
		{"single 1", "1", "", "1"},

		// Empty input
		{"empty", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Normalize(tt.key, tt.source)
			if result != tt.expected {
				t.Errorf("Normalize(%q, %q) = %q, want %q", tt.key, tt.source, result, tt.expected)
			}
		})
	}
}

func TestParseKeySequence(t *testing.T) {
	tests := []struct {
		name     string
		seq      string
		expected []string
	}{
		{"single key", "C-g", []string{"C-G"}},
		{"two keys", "C-g p", []string{"C-G", "P"}},
		{"three keys", "C-b g w", []string{"C-B", "G", "W"}},
		{"mixed notation", "C-g M-f", []string{"C-G", "M-F"}},
		{"empty", "", []string{}},
		{"extra spaces", "C-g   p", []string{"C-G", "P"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseKeySequence(tt.seq)
			if len(result) != len(tt.expected) {
				t.Errorf("ParseKeySequence(%q) = %v (len %d), want %v (len %d)",
					tt.seq, result, len(result), tt.expected, len(tt.expected))
				return
			}
			for i, k := range result {
				if k != tt.expected[i] {
					t.Errorf("ParseKeySequence(%q)[%d] = %q, want %q",
						tt.seq, i, k, tt.expected[i])
				}
			}
		})
	}
}

func TestKeysEqual(t *testing.T) {
	tests := []struct {
		name     string
		a        string
		b        string
		expected bool
	}{
		{"same tmux", "C-p", "C-p", true},
		{"case difference", "C-p", "C-P", true},
		{"fish vs tmux", "\\cp", "C-p", true},
		{"bash vs tmux", "\\C-p", "C-p", true},
		{"zsh vs tmux", "^P", "C-p", true},
		{"all formats", "\\cp", "^P", true},
		{"different keys", "C-p", "C-n", false},
		{"different modifiers", "C-p", "M-p", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := KeysEqual(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("KeysEqual(%q, %q) = %v, want %v", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}
