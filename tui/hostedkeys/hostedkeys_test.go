package hostedkeys

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestReferenceJSONIsStable pins the wire shape. Three declarations join on
// these field names — flow's in-process one, treemux's host-side filter, and
// any sidecar speaking embed/v1 in a language without a typed decoder — so a
// rename here is a compatibility break, not a refactor.
func TestReferenceJSONIsStable(t *testing.T) {
	b, err := json.Marshal(Reference{
		SchemaVersion: SchemaVersion,
		App:           "probe",
		Bindings: []Binding{{
			Scope: "s", Action: "a", Keys: []string{"ctrl+f"},
			Description: "d", ConfigKey: "c", HostSwallowed: true,
			CollisionHints: []string{"nav"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"schema_version":1,"app":"probe","bindings":[{"scope":"s","action":"a",` +
		`"keys":["ctrl+f"],"description":"d","config_key":"c","host_swallowed":true,` +
		`"collision_hints":["nav"]}]}`
	if string(b) != want {
		t.Errorf("reference JSON drifted:\n got %s\nwant %s", b, want)
	}
}

func TestRejectionJSONIsStable(t *testing.T) {
	b, err := json.Marshal(Rejection{Key: "ctrl+c", Reason: ReasonNonDeferrable})
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"key":"ctrl+c","reason":"non_deferrable"}`; string(b) != want {
		t.Errorf("rejection JSON drifted:\n got %s\nwant %s", b, want)
	}
}

func TestDeclaredKeysDedupesAndKeepsOrder(t *testing.T) {
	ref := Reference{Bindings: []Binding{
		{Keys: []string{"ctrl+f", ""}},
		{Keys: []string{"g", "ctrl+f"}},
	}}
	if got := ref.DeclaredKeys(); !reflect.DeepEqual(got, []string{"ctrl+f", "g"}) {
		t.Errorf("DeclaredKeys() = %v, want [ctrl+f g]", got)
	}
}

// A binding that omits the optional fields must not emit them: a sidecar
// decoding into a struct with required fields would otherwise see empty
// strings where the host meant "absent".
func TestOptionalFieldsAreOmitted(t *testing.T) {
	b, err := json.Marshal(Binding{Keys: []string{"g"}})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"scope":"","action":"","keys":["g"],"description":"","host_swallowed":false}`
	if string(b) != want {
		t.Errorf("binding JSON drifted:\n got %s\nwant %s", b, want)
	}
}

// A binding whose chords were only PARTLY granted must report only the
// granted ones. Rendering Binding.Keys next to the description would
// advertise a deferral the user does not actually have.
func TestGrantSplitsBindingsByWhatWasGranted(t *testing.T) {
	ref := Reference{
		App: "probe",
		Bindings: []Binding{
			{Action: "mixed", Keys: []string{"ctrl+f", "ctrl+q"}, Description: "partly granted"},
			{Action: "own", Keys: []string{"j", "k"}, Description: "the app's own keys"},
			{Action: "all", Keys: []string{"ctrl+n"}, Description: "fully granted"},
		},
	}
	g := Grant{Ref: ref, Claims: map[string]bool{"ctrl+f": true, "ctrl+n": true}}

	if g.App() != "probe" {
		t.Errorf("App() = %q, want probe", g.App())
	}

	granted := g.GrantedBindings()
	wantGranted := []BoundKeys{
		{Binding: ref.Bindings[0], Keys: []string{"ctrl+f"}},
		{Binding: ref.Bindings[2], Keys: []string{"ctrl+n"}},
	}
	if !reflect.DeepEqual(granted, wantGranted) {
		t.Errorf("GrantedBindings() = %+v\nwant %+v", granted, wantGranted)
	}

	// The partly-granted binding appears on BOTH sides: ctrl+q still reaches
	// the app without arbitration, and saying so is the honest rendering.
	self := g.SelfBindings()
	wantSelf := []BoundKeys{
		{Binding: ref.Bindings[0], Keys: []string{"ctrl+q"}},
		{Binding: ref.Bindings[1], Keys: []string{"j", "k"}},
	}
	if !reflect.DeepEqual(self, wantSelf) {
		t.Errorf("SelfBindings() = %+v\nwant %+v", self, wantSelf)
	}
}

// The zero Grant must not panic — a panel with no control connection returns
// one, and the help overlay walks it on every render.
func TestZeroGrantIsEmpty(t *testing.T) {
	var g Grant
	if g.App() != "" || len(g.GrantedBindings()) != 0 || len(g.SelfBindings()) != 0 {
		t.Errorf("zero Grant is not empty: %+v", g)
	}
}
