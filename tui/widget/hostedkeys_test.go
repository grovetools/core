package widget

import (
	"reflect"
	"testing"

	"github.com/grovetools/core/tui/hostedkeys"
)

// The projection's whole point is that a host can resolve a drawer widget's
// keys and a hosted panel's keys into ONE shape. These tests pin the parts of
// that mapping a renderer depends on.

func TestWidgetBindingsProjectOntoTheWireShape(t *testing.T) {
	got := HostedBindings("files", []KeyBinding{
		{Key: "q/esc", Desc: "leave the drawer"},
		{Key: "y", Desc: "copy path", When: "on a file row", Active: func() bool { return false }},
	})

	want := []hostedkeys.Binding{
		{Scope: "files", Action: "leave the drawer", Keys: []string{"q", "esc"}, Description: "leave the drawer"},
		{Scope: "files", Action: "copy path", Keys: []string{"y"}, Description: "copy path", When: "on a file row"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("projection =\n%+v\nwant\n%+v", got, want)
	}
}

// The When LABEL crosses; the live answer does not, and comes back as "cannot
// say" rather than "false". A renderer that dimmed a reconstructed row would be
// asserting something it never learned — the widget it describes is on the far
// side of a socket and was never asked.
func TestReconstructionKeepsTheLabelAndDropsThePredicate(t *testing.T) {
	round := BindingsFromHosted(HostedBindings("files", []KeyBinding{
		{Key: "y", Desc: "copy path", When: "on a file row", Active: func() bool { return false }},
	}))

	if len(round) != 1 {
		t.Fatalf("round trip produced %d bindings, want 1", len(round))
	}
	if round[0].Key != "y" || round[0].Desc != "copy path" || round[0].When != "on a file row" {
		t.Errorf("round trip = %+v, want the key, description and When label intact", round[0])
	}
	if round[0].Active != nil {
		t.Error("a predicate came back across the wire — nothing there could have evaluated it")
	}
	if !round[0].Live() {
		t.Error("a reconstructed binding renders as not-live; absent Active means 'cannot say', not 'no'")
	}
}

// A binding whose declaration carries no help text still gets a row: knowing
// the key belongs to this component beats not listing it.
func TestReconstructionFallsBackToTheActionForItsDescription(t *testing.T) {
	got := BindingsFromHosted([]hostedkeys.Binding{
		{Scope: "flow/browser", Action: "OpenPlan", Keys: []string{"enter"}},
	})
	if len(got) != 1 || got[0].Desc != "OpenPlan" {
		t.Errorf("bindings = %+v, want the action standing in for a missing description", got)
	}
}

// "/" is a legal chord on its own — it is the search key in half the TUIs in
// this ecosystem — so the alias separator must not eat it.
func TestSlashSurvivesBeingItsOwnAliasSeparator(t *testing.T) {
	if got := SplitKeyAliases("/"); !reflect.DeepEqual(got, []string{"/"}) {
		t.Errorf("SplitKeyAliases(%q) = %v, want the key itself", "/", got)
	}
	if got := SplitKeyAliases(""); got != nil {
		t.Errorf("SplitKeyAliases(\"\") = %v, want nil", got)
	}
}
