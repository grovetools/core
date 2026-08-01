package panelproto_test

import (
	"encoding/json"
	"testing"

	"github.com/grovetools/core/panelkit/panelproto"
)

// A digest arrives from a panel the host did not compile, so every optional
// field has to survive being absent — the required line alone is a complete
// frame, and that is the shape a one-line panel will actually send.
func TestDigestDecodesWithEveryOptionalFieldAbsent(t *testing.T) {
	var d panelproto.Digest
	if err := json.Unmarshal([]byte(`{"line":"WORK 0:40"}`), &d); err != nil {
		t.Fatalf("decoding a line-only digest: %v", err)
	}
	if d.Line != "WORK 0:40" {
		t.Errorf("Line = %q, want %q", d.Line, "WORK 0:40")
	}
	if d.Detail != "" || d.Icon != "" || d.State != "" {
		t.Errorf("absent optionals decoded to %#v, want all empty", d)
	}
	if d.NormalizedState() != "" {
		t.Errorf("NormalizedState with no state = %q, want empty", d.NormalizedState())
	}
	if d.Empty() {
		t.Error("a digest with a line reported Empty")
	}
}

func TestDigestDecodesEveryField(t *testing.T) {
	var d panelproto.Digest
	raw := `{"line":"WORK 0:40","detail":"focus until the bell","state":"attention","icon":"clock"}`
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatalf("decoding a full digest: %v", err)
	}
	want := panelproto.Digest{
		Line: "WORK 0:40", Detail: "focus until the bell",
		State: panelproto.DigestStateAttention, Icon: "clock",
	}
	if d != want {
		t.Errorf("decoded %#v, want %#v", d, want)
	}
}

// The enum is closed and host-owned, so a value from outside it is a tint this
// host does not have — never a reason to lose the line underneath it.
func TestDigestUnknownStateReadsAsUnstyledRatherThanFailing(t *testing.T) {
	for _, state := range []string{"work", "URGENT", "Ok", " ok", "", "0"} {
		var d panelproto.Digest
		raw, err := json.Marshal(map[string]string{"line": "still here", "state": state})
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &d); err != nil {
			t.Fatalf("state %q: decoding failed, want a decode that keeps the line: %v", state, err)
		}
		if d.Line != "still here" {
			t.Errorf("state %q: Line = %q, want the line to survive", state, d.Line)
		}
		if got := d.NormalizedState(); got != "" {
			t.Errorf("state %q: NormalizedState = %q, want empty", state, got)
		}
	}
}

func TestDigestKnownStatesNormalizeToThemselves(t *testing.T) {
	for _, state := range []string{
		panelproto.DigestStateOK,
		panelproto.DigestStateAttention,
		panelproto.DigestStateIdle,
	} {
		d := panelproto.Digest{Line: "x", State: state}
		if got := d.NormalizedState(); got != state {
			t.Errorf("NormalizedState(%q) = %q, want %q", state, got, state)
		}
	}
}

// A frame with no line is how a panel says "nothing to project", and it must be
// distinguishable from one that merely dropped its detail — otherwise a stale
// line stays on the glass under a fresh second row.
func TestDigestWithoutALineIsEmptyWhateverElseItCarries(t *testing.T) {
	d := panelproto.Digest{Detail: "orphaned", State: panelproto.DigestStateOK, Icon: "clock"}
	if !d.Empty() {
		t.Errorf("%#v reported non-empty; a digest with no line has nothing to draw", d)
	}
}

// Optional fields are omitted on the wire, so a one-line digest is a small
// frame — this is the payload a per-second publisher actually sends.
func TestDigestOmitsEmptyOptionalsOnTheWire(t *testing.T) {
	b, err := json.Marshal(panelproto.Digest{Line: "WORK 0:40"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(b), `{"line":"WORK 0:40"}`; got != want {
		t.Errorf("marshalled %s, want %s", got, want)
	}
}
