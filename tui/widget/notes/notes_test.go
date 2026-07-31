package notes

import (
	"strings"
	"testing"

	"github.com/grovetools/core/tui/theme"
	"github.com/grovetools/tuimux"
)

// A notebook with nothing in it is a real state, and the pane says so in one
// muted ⓘ line — the same sentence [Spec] hands a host that wants to explain
// the pane without mounting it.
func TestEmptySummaryExplainsItself(t *testing.T) {
	p := New(func() []GroupRow { return nil })
	p.Resize(50, 12)

	view := p.View()
	if !strings.Contains(view, theme.IconInfo) || !strings.Contains(view, emptyReason) {
		t.Fatalf("empty notes pane = %q, want the ⓘ line %q", view, emptyReason)
	}
	if got := Spec(func() []GroupRow { return nil }).Reason(); got != emptyReason {
		t.Fatalf("spec reason = %q, want the sentence the pane draws", got)
	}
}

// And it collapses to exactly those lines, so the rows it cannot use go to a
// sibling that can. With any group at all it is flexible: a list wants every
// row it is given, and a hint that moved per note would move the layout with it.
func TestEmptySummaryCollapsesAndContentDoesNot(t *testing.T) {
	groups := []GroupRow{}
	p := New(func() []GroupRow { return groups })
	p.Resize(50, 12)

	rows, flexible := p.PreferredHeightHint()
	if flexible || rows != 3 {
		t.Fatalf("empty pane hint = (%d, flexible=%v), want a bounded 3 rows", rows, flexible)
	}

	// Without the in-body heading the collapse is two lines shorter.
	p.SetShowHeading(false)
	if rows, _ = p.PreferredHeightHint(); rows != 1 {
		t.Fatalf("heading-less empty pane hint = %d rows, want 1", rows)
	}
	p.SetShowHeading(true)

	groups = []GroupRow{{Workspace: "grovetools", Group: "inbox", Count: 2}}
	if rows, flexible = p.PreferredHeightHint(); !flexible {
		t.Fatalf("a summarized group hinted %d bounded rows, want no opinion", rows)
	}

	// The hint must never follow the height the pane was handed.
	groups = nil
	p.Resize(50, 40)
	if rows, _ = p.PreferredHeightHint(); rows != 3 {
		t.Fatalf("hint moved with the pane height: %d rows at 40 high", rows)
	}
}

var _ tuimux.SizeHintProvider = (*Panel)(nil)
