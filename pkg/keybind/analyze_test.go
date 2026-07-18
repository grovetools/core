package keybind

import "testing"

// suggestionStack builds a stack with the given keys bound at the shell
// layer (one of the default layers isKeyBound consults).
func suggestionStack(keys ...string) *Stack {
	s := NewStack()
	for _, k := range keys {
		s.AddBinding(Binding{
			Key:        k,
			Layer:      LayerShell,
			Source:     "bash",
			Action:     "test-action",
			Provenance: ProvenanceDefault,
		})
	}
	return s
}

// assertAllModified fails if any suggestion lacks a modifier prefix.
func assertAllModified(t *testing.T, alts []string) {
	t.Helper()
	for _, a := range alts {
		if _, mods := parseModifiers(a); mods == "" {
			t.Errorf("unmodified suggestion %q — chord alternatives must carry a modifier", a)
		}
	}
}

// TestSuggestAlternativesNeverBare: asking about a modified key (the leader
// path: C-B is taken by readline's backward-char) must never suggest the
// bare base letter — only other modifier combinations.
func TestSuggestAlternativesNeverBare(t *testing.T) {
	s := suggestionStack("C-B")

	alts := s.SuggestAlternatives("C-B", 3)
	if len(alts) == 0 {
		t.Fatal("expected suggestions for C-B")
	}
	assertAllModified(t, alts)
	for _, a := range alts {
		if a == "B" {
			t.Errorf("bare B suggested as a leader alternative: %v", alts)
		}
	}
}

// TestSuggestAlternativesNearbyFallbackModified: when every modifier combo
// of the base is bound, the nearby-letter fallback kicks in — its candidates
// must keep the input's modifiers rather than emitting bare letters.
func TestSuggestAlternativesNearbyFallbackModified(t *testing.T) {
	s := suggestionStack("C-B", "M-B", "C-M-B", "S-B")

	alts := s.SuggestAlternatives("C-B", 3)
	if len(alts) == 0 {
		t.Fatal("expected nearby-letter suggestions")
	}
	assertAllModified(t, alts)
}

// TestSuggestAlternativesBareInputModified: even for a bare input key, the
// suggestions (including the nearby-letter fallback, which substitutes C-
// for the missing modifiers) are all modified.
func TestSuggestAlternativesBareInputModified(t *testing.T) {
	s := suggestionStack("C-B", "M-B", "C-M-B", "S-B")

	alts := s.SuggestAlternatives("B", 5)
	if len(alts) == 0 {
		t.Fatal("expected suggestions for bare B")
	}
	assertAllModified(t, alts)
}
