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

		// Neovim notation
		{"nvim ctrl-g", "<C-g>", "nvim", "C-G"},
		{"nvim ctrl-p", "<C-P>", "nvim", "C-P"},
		{"nvim meta-f", "<M-f>", "nvim", "M-F"},
		{"nvim alt-f", "<A-f>", "nvim", "M-F"},
		{"nvim ctrl-meta-x", "<C-M-x>", "nvim", "C-M-X"},
		{"nvim enter", "<CR>", "nvim", "Enter"},
		{"nvim tab", "<Tab>", "nvim", "Tab"},
		{"nvim escape", "<Esc>", "nvim", "Escape"},
		{"nvim space", "<Space>", "nvim", "Space"},
		{"nvim backspace", "<BS>", "nvim", "Backspace"},
		{"nvim leader", "<leader>", "nvim", "Leader"},
		{"nvim F1", "<F1>", "nvim", "F1"},
		{"nvim up", "<Up>", "nvim", "Up"},

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

func TestDenormalize(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		target   string
		expected string
	}{
		// Fish denormalization
		{"fish ctrl-g", "C-G", "fish", "\\cg"},
		{"fish ctrl-p", "C-P", "fish", "\\cp"},
		{"fish meta-f", "M-F", "fish", "\\ef"},
		{"fish meta-b", "M-B", "fish", "\\eb"},
		{"fish ctrl-meta-x", "C-M-X", "fish", "\\e\\cx"},
		{"fish enter", "Enter", "fish", "\\r"},
		{"fish tab", "Tab", "fish", "\\t"},
		{"fish escape", "Escape", "fish", "\\e"},
		{"fish single char", "G", "fish", "g"},

		// Bash denormalization
		{"bash ctrl-g", "C-G", "bash", "\\C-g"},
		{"bash ctrl-p", "C-P", "bash", "\\C-p"},
		{"bash meta-f", "M-F", "bash", "\\M-f"},
		{"bash meta-b", "M-B", "bash", "\\M-b"},
		{"bash ctrl-meta-x", "C-M-X", "bash", "\\C-\\M-x"},
		{"bash enter", "Enter", "bash", "\\C-m"},
		{"bash tab", "Tab", "bash", "\\C-i"},
		{"bash escape", "Escape", "bash", "\\e"},
		{"bash single char", "G", "bash", "g"},

		// Zsh denormalization
		{"zsh ctrl-g", "C-G", "zsh", "^g"},
		{"zsh ctrl-p", "C-P", "zsh", "^p"},
		{"zsh meta-f", "M-F", "zsh", "^[f"},
		{"zsh meta-b", "M-B", "zsh", "^[b"},
		{"zsh ctrl-meta-x", "C-M-X", "zsh", "^[^x"},
		{"zsh enter", "Enter", "zsh", "^m"},
		{"zsh tab", "Tab", "zsh", "^i"},
		{"zsh escape", "Escape", "zsh", "^["},
		{"zsh single char", "G", "zsh", "g"},

		// Tmux (unchanged)
		{"tmux ctrl-g", "C-G", "tmux", "C-G"},
		{"tmux meta-f", "M-F", "tmux", "M-F"},

		// Neovim denormalization
		{"nvim ctrl-g", "C-G", "nvim", "<C-g>"},
		{"nvim ctrl-p", "C-P", "nvim", "<C-p>"},
		{"nvim meta-f", "M-F", "nvim", "<M-f>"},
		{"nvim meta-b", "M-B", "nvim", "<M-b>"},
		{"nvim ctrl-meta-x", "C-M-X", "nvim", "<C-M-x>"},
		{"nvim enter", "Enter", "nvim", "<CR>"},
		{"nvim tab", "Tab", "nvim", "<Tab>"},
		{"nvim escape", "Escape", "nvim", "<Esc>"},
		{"nvim space", "Space", "nvim", "<Space>"},
		{"nvim backspace", "Backspace", "nvim", "<BS>"},
		{"nvim F1", "F1", "nvim", "<F1>"},
		{"nvim up", "Up", "nvim", "<Up>"},
		{"nvim single char", "G", "nvim", "G"},

		// Empty and unknown target
		{"empty key", "", "fish", ""},
		{"unknown target", "C-G", "unknown", "C-G"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Denormalize(tt.key, tt.target)
			if result != tt.expected {
				t.Errorf("Denormalize(%q, %q) = %q, want %q", tt.key, tt.target, result, tt.expected)
			}
		})
	}
}

func TestDenormalizeRoundTrip(t *testing.T) {
	// Test that Normalize(Denormalize(key)) == key for various keys
	// Note: C-M-X and M-C-X are semantically equivalent
	keys := []string{"C-G", "C-P", "M-F", "M-B", "C-M-X"}

	// Helper to check if two keys are equivalent (C-M-X == M-C-X)
	keysEquivalent := func(a, b string) bool {
		if a == b {
			return true
		}
		// C-M-X and M-C-X are equivalent
		if (a == "C-M-X" && b == "M-C-X") || (a == "M-C-X" && b == "C-M-X") {
			return true
		}
		return false
	}

	for _, key := range keys {
		t.Run("fish "+key, func(t *testing.T) {
			denorm := Denormalize(key, "fish")
			norm := Normalize(denorm, "fish")
			if !keysEquivalent(norm, key) {
				t.Errorf("Round trip failed: %q -> %q -> %q", key, denorm, norm)
			}
		})

		t.Run("bash "+key, func(t *testing.T) {
			denorm := Denormalize(key, "bash")
			norm := Normalize(denorm, "bash")
			if !keysEquivalent(norm, key) {
				t.Errorf("Round trip failed: %q -> %q -> %q", key, denorm, norm)
			}
		})

		t.Run("zsh "+key, func(t *testing.T) {
			denorm := Denormalize(key, "zsh")
			norm := Normalize(denorm, "zsh")
			if !keysEquivalent(norm, key) {
				t.Errorf("Round trip failed: %q -> %q -> %q", key, denorm, norm)
			}
		})

		t.Run("nvim "+key, func(t *testing.T) {
			denorm := Denormalize(key, "nvim")
			norm := Normalize(denorm, "nvim")
			if !keysEquivalent(norm, key) {
				t.Errorf("Round trip failed: %q -> %q -> %q", key, denorm, norm)
			}
		})
	}
}
