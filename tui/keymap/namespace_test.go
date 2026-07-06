package keymap

import (
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/key"
)

// viewNamespace builds a "View" namespace with three completions (vl/vf/vv)
// used across the arming and filtering tests.
func viewNamespace() Namespace {
	return Namespace{
		Prefix: "v",
		Label:  "View",
		Bindings: []key.Binding{
			key.NewBinding(key.WithKeys("vl"), key.WithHelp("vl", "logs")),
			key.NewBinding(key.WithKeys("vf"), key.WithHelp("vf", "frontmatter")),
			key.NewBinding(key.WithKeys("vv"), key.WithHelp("vv", "preview")),
		},
	}
}

func TestNamespaceArmed(t *testing.T) {
	ns := viewNamespace()

	if !ns.Armed("v") {
		t.Errorf("expected %q to arm the view namespace", "v")
	}
	if ns.Armed("x") {
		t.Errorf("expected %q not to arm the view namespace", "x")
	}
	if ns.Armed("") {
		t.Errorf("expected empty buffer not to arm the view namespace")
	}

	// A disabled member must not keep the prefix armed on its own.
	off := Namespace{
		Prefix: "v",
		Label:  "View",
		Bindings: []key.Binding{
			key.NewBinding(key.WithKeys("vl"), key.WithHelp("vl", "logs"), key.WithDisabled()),
		},
	}
	if off.Armed("v") {
		t.Errorf("expected a namespace with only disabled members not to arm")
	}
}

func TestPendingRowsFiltering(t *testing.T) {
	ns := viewNamespace()

	rows := ns.PendingRows("v")
	if len(rows) != 3 {
		t.Fatalf("buffer %q: expected 3 rows, got %d (%v)", "v", len(rows), rows)
	}
	// Remainders are the key with the buffer trimmed off.
	want := map[string]bool{"l": true, "f": true, "v": true}
	for _, r := range rows {
		if !want[r.Keys] {
			t.Errorf("unexpected row key %q in %v", r.Keys, rows)
		}
	}

	if rows := ns.PendingRows("vf"); len(rows) != 1 {
		t.Errorf("buffer %q: expected 1 row, got %d (%v)", "vf", len(rows), rows)
	}
	if rows := ns.PendingRows("vz"); len(rows) != 0 {
		t.Errorf("buffer %q: expected 0 rows, got %d (%v)", "vz", len(rows), rows)
	}
}

func TestPendingHintFlatChords(t *testing.T) {
	bindings := CommonSequenceBindings(DefaultVim())

	if hint := PendingHint("g", bindings...); hint == "" {
		t.Errorf("buffer %q: expected a non-empty flat-chord hint", "g")
	}
	if hint := PendingHint("z", bindings...); hint == "" {
		t.Errorf("buffer %q: expected a non-empty flat-chord hint", "z")
	}
	if hint := PendingHint("q", bindings...); hint != "" {
		t.Errorf("buffer %q: expected empty hint, got %q", "q", hint)
	}
	if hint := PendingHint("", bindings...); hint != "" {
		t.Errorf("empty buffer: expected empty hint, got %q", hint)
	}
}

func TestResolvePendingPrecedence(t *testing.T) {
	namespaces := []Namespace{viewNamespace()}
	flat := CommonSequenceBindings(DefaultVim())

	// An armed namespace wins: popup group, no flat hint.
	group, hint := ResolvePending("v", namespaces, flat...)
	if group == nil {
		t.Fatalf("buffer %q: expected an armed namespace group", "v")
	}
	if hint != "" {
		t.Errorf("buffer %q: expected no flat hint when a namespace is armed, got %q", "v", hint)
	}
	if len(group.Rows) != 3 {
		t.Errorf("buffer %q: expected 3 popup rows, got %d", "v", len(group.Rows))
	}

	// No namespace armed → flat hint path.
	group, hint = ResolvePending("g", namespaces, flat...)
	if group != nil {
		t.Errorf("buffer %q: expected no namespace group, got %+v", "g", group)
	}
	if hint == "" {
		t.Errorf("buffer %q: expected a flat-chord hint", "g")
	}
}

// TestPrefixVsExactPrecedence drives a real SequenceState to document why a
// namespace prefix must stay unbound as a flat key: with only chords present,
// the prefix key is pending; adding a flat binding for the same key makes it
// fire immediately and the chord can never arm.
func TestPrefixVsExactPrecedence(t *testing.T) {
	vv := key.NewBinding(key.WithKeys("vv"), key.WithHelp("vv", "preview"))
	vl := key.NewBinding(key.WithKeys("vl"), key.WithHelp("vl", "logs"))

	s := NewSequenceState()
	if res, _ := s.ProcessKey("v", vv, vl); res != SequencePending {
		t.Fatalf("first %q with only chords: expected SequencePending, got %v", "v", res)
	}
	res, idx := s.ProcessKey("v", vv, vl)
	if res != SequenceMatch {
		t.Fatalf("second %q: expected SequenceMatch, got %v", "v", res)
	}
	if idx != 0 {
		t.Errorf("expected match on vv (index 0), got index %d", idx)
	}

	// With a flat "v" binding present, "v" matches exactly on the first press —
	// the chord is shadowed and never arms.
	flatV := key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "flat"))
	s.Clear()
	res, idx = s.ProcessKey("v", flatV, vv, vl)
	if res != SequenceMatch {
		t.Fatalf("flat %q present: expected immediate SequenceMatch, got %v", "v", res)
	}
	if idx != 0 {
		t.Errorf("expected match on the flat v binding (index 0), got index %d", idx)
	}
}

// TestSequenceTimeoutClearsArm confirms an armed prefix expires: after the
// timeout elapses the next key starts a fresh buffer rather than extending the
// stale one.
func TestSequenceTimeoutClearsArm(t *testing.T) {
	s := NewSequenceStateWithTimeout(10 * time.Millisecond)

	if buf := s.UpdateKey("v"); buf != "v" {
		t.Fatalf("expected buffer %q after arming, got %q", "v", buf)
	}
	time.Sleep(20 * time.Millisecond)

	if buf := s.UpdateKey("l"); buf != "l" {
		t.Errorf("expected the buffer to reset to %q after timeout, got %q", "l", buf)
	}
}
