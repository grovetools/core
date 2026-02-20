package keymap

import (
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

func TestSequenceState_Update(t *testing.T) {
	s := NewSequenceState()

	// First key
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}}
	buf := s.Update(msg)
	if buf != "g" {
		t.Errorf("Expected buffer='g', got %q", buf)
	}

	// Second key
	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}}
	buf = s.Update(msg)
	if buf != "gg" {
		t.Errorf("Expected buffer='gg', got %q", buf)
	}
}

func TestSequenceState_UpdateKey(t *testing.T) {
	s := NewSequenceState()

	buf := s.UpdateKey("z")
	if buf != "z" {
		t.Errorf("Expected buffer='z', got %q", buf)
	}

	buf = s.UpdateKey("o")
	if buf != "zo" {
		t.Errorf("Expected buffer='zo', got %q", buf)
	}
}

func TestSequenceState_Clear(t *testing.T) {
	s := NewSequenceState()

	s.UpdateKey("g")
	s.UpdateKey("g")

	if !s.IsPending() {
		t.Error("Expected IsPending=true before Clear")
	}

	s.Clear()

	if s.IsPending() {
		t.Error("Expected IsPending=false after Clear")
	}
	if s.Buffer() != "" {
		t.Errorf("Expected empty buffer after Clear, got %q", s.Buffer())
	}
}

func TestSequenceState_Timeout(t *testing.T) {
	s := NewSequenceStateWithTimeout(50 * time.Millisecond)

	s.UpdateKey("g")
	if s.Buffer() != "g" {
		t.Errorf("Expected buffer='g', got %q", s.Buffer())
	}

	// Wait for timeout
	time.Sleep(100 * time.Millisecond)

	// Next key should clear buffer first
	buf := s.UpdateKey("x")
	if buf != "x" {
		t.Errorf("Expected buffer='x' after timeout, got %q", buf)
	}
}

func TestMatches(t *testing.T) {
	binding := key.NewBinding(key.WithKeys("gg", "G"))

	tests := []struct {
		buffer   string
		expected bool
	}{
		{"gg", true},
		{"G", true},
		{"g", false},
		{"ggg", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.buffer, func(t *testing.T) {
			if got := Matches(tt.buffer, binding); got != tt.expected {
				t.Errorf("Matches(%q)=%v, want %v", tt.buffer, got, tt.expected)
			}
		})
	}
}

func TestMatchesAny(t *testing.T) {
	top := key.NewBinding(key.WithKeys("gg"))
	bottom := key.NewBinding(key.WithKeys("G"))
	delete := key.NewBinding(key.WithKeys("dd"))

	tests := []struct {
		buffer      string
		expectedIdx int
		expectedOk  bool
	}{
		{"gg", 0, true},
		{"G", 1, true},
		{"dd", 2, true},
		{"g", -1, false},
		{"x", -1, false},
	}

	for _, tt := range tests {
		t.Run(tt.buffer, func(t *testing.T) {
			idx, ok := MatchesAny(tt.buffer, top, bottom, delete)
			if idx != tt.expectedIdx || ok != tt.expectedOk {
				t.Errorf("MatchesAny(%q)=(%d,%v), want (%d,%v)",
					tt.buffer, idx, ok, tt.expectedIdx, tt.expectedOk)
			}
		})
	}
}

func TestIsPrefix(t *testing.T) {
	binding := key.NewBinding(key.WithKeys("zo", "zc", "za"))

	tests := []struct {
		buffer   string
		expected bool
	}{
		{"z", true},   // Prefix of zo, zc, za
		{"zo", false}, // Exact match, not prefix
		{"zx", false}, // Not a prefix
		{"", false},   // Empty not a prefix
		{"a", false},  // Not a prefix
	}

	for _, tt := range tests {
		t.Run(tt.buffer, func(t *testing.T) {
			if got := IsPrefix(tt.buffer, binding); got != tt.expected {
				t.Errorf("IsPrefix(%q)=%v, want %v", tt.buffer, got, tt.expected)
			}
		})
	}
}

func TestIsPrefixOfAny(t *testing.T) {
	top := key.NewBinding(key.WithKeys("gg"))
	foldOpen := key.NewBinding(key.WithKeys("zo"))
	foldClose := key.NewBinding(key.WithKeys("zc"))

	tests := []struct {
		buffer   string
		expected bool
	}{
		{"g", true},  // Prefix of gg
		{"z", true},  // Prefix of zo, zc
		{"x", false}, // Not a prefix of any
		{"gg", false},// Exact match, not prefix
		{"zo", false},// Exact match, not prefix
	}

	for _, tt := range tests {
		t.Run(tt.buffer, func(t *testing.T) {
			if got := IsPrefixOfAny(tt.buffer, top, foldOpen, foldClose); got != tt.expected {
				t.Errorf("IsPrefixOfAny(%q)=%v, want %v", tt.buffer, got, tt.expected)
			}
		})
	}
}

func TestSequenceState_Process(t *testing.T) {
	top := key.NewBinding(key.WithKeys("gg"))
	foldOpen := key.NewBinding(key.WithKeys("zo"))

	tests := []struct {
		name     string
		keys     []string
		expected SequenceResult
		idx      int
	}{
		{
			name:     "single g is pending",
			keys:     []string{"g"},
			expected: SequencePending,
			idx:      -1,
		},
		{
			name:     "gg is match",
			keys:     []string{"g", "g"},
			expected: SequenceMatch,
			idx:      0,
		},
		{
			name:     "single z is pending",
			keys:     []string{"z"},
			expected: SequencePending,
			idx:      -1,
		},
		{
			name:     "zo is match",
			keys:     []string{"z", "o"},
			expected: SequenceMatch,
			idx:      1,
		},
		{
			name:     "x is none",
			keys:     []string{"x"},
			expected: SequenceNone,
			idx:      -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSequenceState()
			var result SequenceResult
			var idx int

			for _, k := range tt.keys {
				result, idx = s.ProcessKey(k, top, foldOpen)
			}

			if result != tt.expected || idx != tt.idx {
				t.Errorf("ProcessKey(%v)=(%v,%d), want (%v,%d)",
					tt.keys, result, idx, tt.expected, tt.idx)
			}
		})
	}
}

func TestSequenceState_ProcessWithClear(t *testing.T) {
	top := key.NewBinding(key.WithKeys("gg"))
	s := NewSequenceState()

	// First g - pending
	result, _ := s.ProcessKey("g", top)
	if result != SequencePending {
		t.Errorf("First g should be pending, got %v", result)
	}

	// Second g - match
	result, idx := s.ProcessKey("g", top)
	if result != SequenceMatch || idx != 0 {
		t.Errorf("Second g should match, got result=%v idx=%d", result, idx)
	}

	// Clear and start again
	s.Clear()
	result, _ = s.ProcessKey("g", top)
	if result != SequencePending {
		t.Errorf("After clear, g should be pending, got %v", result)
	}
}

func TestCommonSequenceBindings(t *testing.T) {
	base := DefaultVim()
	bindings := CommonSequenceBindings(base)

	if len(bindings) < 8 {
		t.Errorf("Expected at least 8 common sequence bindings, got %d", len(bindings))
	}

	// Check that the bindings are correct
	expectedKeys := []string{"gg", "dd", "yy", "zo", "zc", "za", "zR", "zM"}
	for i, expectedKey := range expectedKeys {
		keys := bindings[i].Keys()
		if len(keys) < 1 || keys[0] != expectedKey {
			t.Errorf("Binding %d: expected key %q, got %v", i, expectedKey, keys)
		}
	}
}

func TestSequenceResult_String(t *testing.T) {
	// Ensure the constants are distinct
	results := []SequenceResult{SequenceNone, SequencePending, SequenceMatch}
	seen := make(map[SequenceResult]bool)
	for _, r := range results {
		if seen[r] {
			t.Errorf("Duplicate SequenceResult: %v", r)
		}
		seen[r] = true
	}
}
