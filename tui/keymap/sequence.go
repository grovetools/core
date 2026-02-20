package keymap

import (
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// SequenceState manages state for multi-key sequences (e.g., gg, dd, zo).
// It tracks the current key buffer and handles timeout-based clearing.
type SequenceState struct {
	buffer     string
	lastUpdate time.Time
	timeout    time.Duration
}

// NewSequenceState creates a new sequence state handler with a 1 second timeout.
func NewSequenceState() *SequenceState {
	return NewSequenceStateWithTimeout(time.Second)
}

// NewSequenceStateWithTimeout creates a new sequence state handler with a custom timeout.
func NewSequenceStateWithTimeout(timeout time.Duration) *SequenceState {
	return &SequenceState{
		timeout: timeout,
	}
}

// Update processes a key message and returns the current buffer.
// If the timeout has elapsed since the last key, the buffer is cleared first.
func (s *SequenceState) Update(msg tea.KeyMsg) string {
	// Clear buffer if timeout elapsed
	if s.timeout > 0 && time.Since(s.lastUpdate) > s.timeout {
		s.buffer = ""
	}
	s.lastUpdate = time.Now()

	// Append the key to buffer
	s.buffer += msg.String()
	return s.buffer
}

// UpdateKey processes a key string (instead of tea.KeyMsg) and returns the current buffer.
// This is useful when you already have the key string.
func (s *SequenceState) UpdateKey(keyStr string) string {
	// Clear buffer if timeout elapsed
	if s.timeout > 0 && time.Since(s.lastUpdate) > s.timeout {
		s.buffer = ""
	}
	s.lastUpdate = time.Now()

	// Append the key to buffer
	s.buffer += keyStr
	return s.buffer
}

// Clear resets the sequence buffer. Call this after a successful match.
func (s *SequenceState) Clear() {
	s.buffer = ""
}

// Buffer returns the current buffer contents.
func (s *SequenceState) Buffer() string {
	return s.buffer
}

// IsPending returns true if there is content in the buffer.
func (s *SequenceState) IsPending() bool {
	return len(s.buffer) > 0
}

// Matches checks if the current buffer matches the binding.
// It returns true if any of the binding's keys exactly equals the buffer.
func Matches(buffer string, binding key.Binding) bool {
	for _, k := range binding.Keys() {
		if k == buffer {
			return true
		}
	}
	return false
}

// MatchesAny checks if the buffer matches any of the provided bindings.
// Returns the index of the first matching binding and true, or -1 and false.
func MatchesAny(buffer string, bindings ...key.Binding) (int, bool) {
	for i, binding := range bindings {
		if Matches(buffer, binding) {
			return i, true
		}
	}
	return -1, false
}

// IsPrefix checks if the buffer is a prefix of any of the binding's keys.
// This is useful for knowing whether to wait for more input.
// For example, "z" is a prefix of "zo", "zc", "za".
func IsPrefix(buffer string, binding key.Binding) bool {
	if buffer == "" {
		return false
	}
	for _, k := range binding.Keys() {
		if len(buffer) < len(k) && k[:len(buffer)] == buffer {
			return true
		}
	}
	return false
}

// IsPrefixOfAny checks if the buffer is a prefix of any key in any of the bindings.
func IsPrefixOfAny(buffer string, bindings ...key.Binding) bool {
	for _, binding := range bindings {
		if IsPrefix(buffer, binding) {
			return true
		}
	}
	return false
}

// SequenceResult represents the result of processing a key in a sequence context.
type SequenceResult int

const (
	// SequenceNone indicates no match and no potential match.
	SequenceNone SequenceResult = iota
	// SequencePending indicates the buffer is a prefix of a valid sequence.
	SequencePending
	// SequenceMatch indicates a complete sequence match.
	SequenceMatch
)

// Process handles a key message and returns the result and matching binding index.
// It updates the sequence state, checks for matches, and indicates whether to wait.
//
// Usage:
//
//	result, idx := seq.Process(msg, m.keys.Top, m.keys.FoldOpen, m.keys.FoldClose)
//	switch result {
//	case keymap.SequenceMatch:
//	    seq.Clear()
//	    // Handle binding at idx
//	case keymap.SequencePending:
//	    // Wait for more input
//	case keymap.SequenceNone:
//	    seq.Clear()
//	    // Handle single key or unknown
//	}
func (s *SequenceState) Process(msg tea.KeyMsg, bindings ...key.Binding) (SequenceResult, int) {
	buffer := s.Update(msg)

	// Check for exact match first
	if idx, ok := MatchesAny(buffer, bindings...); ok {
		return SequenceMatch, idx
	}

	// Check if it's a prefix of any binding
	if IsPrefixOfAny(buffer, bindings...) {
		return SequencePending, -1
	}

	return SequenceNone, -1
}

// ProcessKey is like Process but takes a key string instead of tea.KeyMsg.
func (s *SequenceState) ProcessKey(keyStr string, bindings ...key.Binding) (SequenceResult, int) {
	buffer := s.UpdateKey(keyStr)

	// Check for exact match first
	if idx, ok := MatchesAny(buffer, bindings...); ok {
		return SequenceMatch, idx
	}

	// Check if it's a prefix of any binding
	if IsPrefixOfAny(buffer, bindings...) {
		return SequencePending, -1
	}

	return SequenceNone, -1
}

// CommonSequenceBindings returns the standard sequence bindings used in Grove TUIs.
// This is a convenience function for TUIs that want to use the standard sequences.
func CommonSequenceBindings(base Base) []key.Binding {
	return []key.Binding{
		base.Top,          // gg
		base.Delete,       // dd
		base.Yank,         // yy
		base.FoldOpen,     // zo
		base.FoldClose,    // zc
		base.FoldToggle,   // za
		base.FoldOpenAll,  // zR
		base.FoldCloseAll, // zM
	}
}
