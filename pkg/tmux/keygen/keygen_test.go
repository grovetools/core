package keygen

import (
	"strings"
	"testing"
)

func TestMode(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		expected PrefixMode
	}{
		{"empty prefix", "", ModeDirectRoot},
		{"native prefix", "<prefix>", ModeDirectPrefix},
		{"sub-table under prefix", "<prefix> g", ModeSubTablePrefix},
		{"root key", "C-g", ModeSubTableRoot},
		{"alt root key", "M-w", ModeSubTableRoot},
		{"grove direct", "<grove>", ModeGroveDirect},
		{"grove sub-table", "<grove> n", ModeGroveSubTable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{Prefix: tt.prefix, TableName: "test-table"}
			if got := cfg.Mode(); got != tt.expected {
				t.Errorf("Mode() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGenerateEntryPoint(t *testing.T) {
	tests := []struct {
		name          string
		prefix        string
		tableName     string
		expectEmpty   bool
		expectContent string
	}{
		{
			name:        "direct root has no entry point",
			prefix:      "",
			tableName:   "test-table",
			expectEmpty: true,
		},
		{
			name:          "direct prefix table mode",
			prefix:        "<prefix>",
			tableName:     "test-table",
			expectContent: "Direct Prefix Table Mode",
		},
		{
			name:          "sub-table under prefix",
			prefix:        "<prefix> g",
			tableName:     "grove-popups",
			expectContent: "bind-key g switch-client -T grove-popups",
		},
		{
			name:          "root key sub-table",
			prefix:        "C-g",
			tableName:     "grove-popups",
			expectContent: "bind-key -n C-g switch-client -T grove-popups",
		},
		{
			name:          "grove direct mode",
			prefix:        "<grove>",
			tableName:     "nav-workspaces",
			expectContent: "Grove Popups Table Mode",
		},
		{
			name:          "grove sub-table",
			prefix:        "<grove> n",
			tableName:     "nav-workspaces",
			expectContent: "bind-key -T grove-popups n switch-client -T nav-workspaces",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{Prefix: tt.prefix, TableName: tt.tableName}
			lines := cfg.GenerateEntryPoint()

			if tt.expectEmpty && len(lines) > 0 {
				t.Errorf("Expected empty entry point, got %v", lines)
			}

			if tt.expectContent != "" {
				found := false
				for _, line := range lines {
					if strings.Contains(line, tt.expectContent) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected to find %q in lines: %v", tt.expectContent, lines)
				}
			}
		})
	}
}

func TestGenerateEscapeHatches(t *testing.T) {
	tests := []struct {
		name        string
		prefix      string
		tableName   string
		helpCmd     string
		expectEmpty bool
		expectItems []string
	}{
		{
			name:        "direct root has no escape hatches",
			prefix:      "",
			tableName:   "test-table",
			helpCmd:     "help",
			expectEmpty: true,
		},
		{
			name:        "direct prefix has no escape hatches",
			prefix:      "<prefix>",
			tableName:   "test-table",
			helpCmd:     "help",
			expectEmpty: true,
		},
		{
			name:      "sub-table under prefix",
			prefix:    "<prefix> g",
			tableName: "grove-popups",
			helpCmd:   "grove keys list",
			expectItems: []string{
				"bind-key -T grove-popups Escape switch-client -T root",
				"bind-key -T grove-popups C-c switch-client -T root",
				"bind-key -T grove-popups q switch-client -T root",
				"bind-key -T grove-popups ? display-popup",
			},
		},
		{
			name:      "root key sub-table includes passthrough",
			prefix:    "C-g",
			tableName: "grove-popups",
			helpCmd:   "grove keys list",
			expectItems: []string{
				"bind-key -T grove-popups Escape switch-client -T root",
				"bind-key -T grove-popups C-g send-keys C-g",
			},
		},
		{
			name:        "grove direct has no escape hatches",
			prefix:      "<grove>",
			tableName:   "nav-workspaces",
			helpCmd:     "nav key list",
			expectEmpty: true,
		},
		{
			name:      "grove sub-table escapes to grove-popups",
			prefix:    "<grove> n",
			tableName: "nav-workspaces",
			helpCmd:   "nav key list",
			expectItems: []string{
				"bind-key -T nav-workspaces Escape switch-client -T grove-popups",
				"bind-key -T nav-workspaces C-c switch-client -T grove-popups",
				"bind-key -T nav-workspaces q switch-client -T grove-popups",
				"bind-key -T nav-workspaces ? display-popup",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{Prefix: tt.prefix, TableName: tt.tableName}
			lines := cfg.GenerateEscapeHatches(tt.helpCmd)

			if tt.expectEmpty {
				if len(lines) > 0 {
					t.Errorf("Expected empty escape hatches, got %v", lines)
				}
				return
			}

			combined := strings.Join(lines, "\n")
			for _, expected := range tt.expectItems {
				if !strings.Contains(combined, expected) {
					t.Errorf("Expected to find %q in:\n%s", expected, combined)
				}
			}
		})
	}
}

func TestBindTarget(t *testing.T) {
	tests := []struct {
		name      string
		prefix    string
		tableName string
		expected  string
	}{
		{"direct root", "", "test", "-n"},
		{"direct prefix", "<prefix>", "test", ""},
		{"sub-table prefix", "<prefix> g", "grove-popups", "-T grove-popups"},
		{"root key", "C-g", "grove-popups", "-T grove-popups"},
		{"grove direct", "<grove>", "nav-workspaces", "-T grove-popups"},
		{"grove sub-table", "<grove> n", "nav-workspaces", "-T nav-workspaces"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{Prefix: tt.prefix, TableName: tt.tableName}
			if got := cfg.BindTarget(); got != tt.expected {
				t.Errorf("BindTarget() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestFormatBindKey(t *testing.T) {
	tests := []struct {
		name       string
		prefix     string
		tableName  string
		key        string
		action     string
		extraFlags []string
		expected   string
	}{
		{
			name:      "direct root",
			prefix:    "",
			tableName: "test",
			key:       "M-f",
			action:    "run-shell \"cmd\"",
			expected:  "bind-key -n M-f run-shell \"cmd\"",
		},
		{
			name:      "direct prefix",
			prefix:    "<prefix>",
			tableName: "test",
			key:       "f",
			action:    "run-shell \"cmd\"",
			expected:  "bind-key f run-shell \"cmd\"",
		},
		{
			name:       "with repeat flag",
			prefix:     "<prefix>",
			tableName:  "test",
			key:        "w",
			action:     "run-shell \"sessionize\"",
			extraFlags: []string{"-r"},
			expected:   "bind-key -r w run-shell \"sessionize\"",
		},
		{
			name:      "sub-table",
			prefix:    "C-g",
			tableName: "grove-popups",
			key:       "p",
			action:    "display-popup",
			expected:  "bind-key -T grove-popups p display-popup",
		},
		{
			name:      "grove direct binds to grove-popups",
			prefix:    "<grove>",
			tableName: "nav-workspaces",
			key:       "w",
			action:    "run-shell \"nav sessionize\"",
			expected:  "bind-key -T grove-popups w run-shell \"nav sessionize\"",
		},
		{
			name:       "grove sub-table",
			prefix:     "<grove> n",
			tableName:  "nav-workspaces",
			key:        "w",
			action:     "run-shell \"nav sessionize\"",
			extraFlags: []string{"-r"},
			expected:   "bind-key -r -T nav-workspaces w run-shell \"nav sessionize\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{Prefix: tt.prefix, TableName: tt.tableName}
			got := cfg.FormatBindKey(tt.key, tt.action, tt.extraFlags...)
			if got != tt.expected {
				t.Errorf("FormatBindKey() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestEscapeCommand(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"with \"quotes\"", "with \\\"quotes\\\""},
		{"with $var", "with \\$var"},
		{"with \\backslash", "with \\\\backslash"},
		{"combo \"$test\\\"", "combo \\\"\\$test\\\\\\\""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := EscapeCommand(tt.input); got != tt.expected {
				t.Errorf("EscapeCommand(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestEscapeKey(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"C-g", "C-g"},
		{"M-f", "M-f"},
		{"\\", "\\\\"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := EscapeKey(tt.input); got != tt.expected {
				t.Errorf("EscapeKey(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
