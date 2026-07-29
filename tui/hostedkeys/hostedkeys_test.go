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
