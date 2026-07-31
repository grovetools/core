package keymap

import (
	"reflect"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	"github.com/grovetools/core/tui/hostedkeys"
)

type hostedTestKeys struct {
	Up       key.Binding
	PageDown key.Binding
	Jump     key.Binding
	Quit     key.Binding
	Disabled key.Binding
	NoKeys   key.Binding
}

func newHostedTestKeys() hostedTestKeys {
	disabled := key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "never"))
	disabled.SetEnabled(false)
	return hostedTestKeys{
		Up:       key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "move up")),
		PageDown: key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgdown", "page down")),
		Jump:     key.NewBinding(key.WithKeys("ctrl+e"), key.WithHelp("ctrl+e", "jump to notebook")),
		Quit:     key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
		Disabled: disabled,
		NoKeys:   key.NewBinding(key.WithHelp("", "no chords")),
	}
}

func (k hostedTestKeys) Sections() []Section {
	return []Section{
		NavigationSection(k.Up, k.PageDown),
		ActionsSection(k.Jump, k.Disabled, k.NoKeys),
		SystemSection(k.Quit),
	}
}

// The whole point of the bridge: the Reference is DERIVED, so the description
// on the wire and the description the help overlay renders are the same string
// by construction rather than by discipline.
func TestHostedReferenceDerivesTheWholeKeyMap(t *testing.T) {
	keys := newHostedTestKeys()
	ref := HostedReference("timerapp", "timer", keys)

	if ref.SchemaVersion != hostedkeys.SchemaVersion || ref.App != "timerapp" {
		t.Fatalf("reference envelope = %d/%q, want %d/%q",
			ref.SchemaVersion, ref.App, hostedkeys.SchemaVersion, "timerapp")
	}

	want := []hostedkeys.Binding{
		// Section order, then declaration order within a section — the order
		// the help overlay renders.
		{Scope: "timer/Navigation", Action: "Up", Keys: []string{"k", "up"}, Description: "move up", ConfigKey: "up"},
		{Scope: "timer/Navigation", Action: "PageDown", Keys: []string{"pgdown"}, Description: "page down", ConfigKey: "page_down"},
		{Scope: "timer/Actions", Action: "Jump", Keys: []string{"ctrl+e"}, Description: "jump to notebook", ConfigKey: "jump"},
		{Scope: "timer/System", Action: "Quit", Keys: []string{"q"}, Description: "quit", ConfigKey: "quit"},
	}
	if !reflect.DeepEqual(ref.Bindings, want) {
		t.Errorf("bindings =\n%+v\nwant\n%+v", ref.Bindings, want)
	}
}

// A disabled binding and a binding with no chords are both dropped: a chord
// the app will not act on has no business being claimed from the host, and a
// binding with no chord has nothing to claim.
func TestHostedBindingsSkipsUnclaimable(t *testing.T) {
	keys := newHostedTestKeys()
	for _, b := range HostedBindings("timer", keys) {
		if b.Action == "Disabled" || b.Action == "NoKeys" {
			t.Errorf("published an unclaimable binding: %+v", b)
		}
	}
}

// A sidecar narrows to the chords its manifest asked the user to approve; the
// descriptions still come from the one key map.
func TestHostedReferenceNarrowsToClaims(t *testing.T) {
	keys := newHostedTestKeys()
	ref := HostedReference("timerapp", "timer", keys, keys.Jump)

	want := []hostedkeys.Binding{
		{Scope: "timer/Actions", Action: "Jump", Keys: []string{"ctrl+e"}, Description: "jump to notebook", ConfigKey: "jump"},
	}
	if !reflect.DeepEqual(ref.Bindings, want) {
		t.Errorf("bindings =\n%+v\nwant\n%+v", ref.Bindings, want)
	}
}

// A claim the key map's Sections() forgot is still published. Dropping it
// would leave the panel with a binding that mysteriously never fires — the
// exact failure arbitration exists to make visible.
func TestHostedBindingsPublishesClaimMissingFromSections(t *testing.T) {
	keys := newHostedTestKeys()
	orphan := key.NewBinding(key.WithKeys("ctrl+t"), key.WithHelp("ctrl+t", "orphaned"))
	got := HostedBindings("timer", keys, keys.Jump, orphan)

	if len(got) != 2 {
		t.Fatalf("got %d bindings, want 2: %+v", len(got), got)
	}
	last := got[1]
	// No struct field backs it, so identity falls back to the description
	// exactly as MakeTUIInfo's export does.
	want := hostedkeys.Binding{
		Scope: "timer", Action: "orphaned", Keys: []string{"ctrl+t"},
		Description: "orphaned", ConfigKey: "orphaned",
	}
	if !reflect.DeepEqual(last, want) {
		t.Errorf("orphan binding = %+v, want %+v", last, want)
	}
}

type twoSectionKeys struct {
	Shared key.Binding
}

func (k twoSectionKeys) Sections() []Section {
	return []Section{
		NavigationSection(k.Shared),
		ActionsSection(k.Shared),
	}
}

// A binding rendered in two sections is one binding, published under the
// first. Two rows for one chord would make the host's accepted list disagree
// with its own arithmetic.
func TestHostedBindingsDeduplicatesAcrossSections(t *testing.T) {
	keys := twoSectionKeys{Shared: key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "shared"))}
	got := HostedBindings("app", keys)
	if len(got) != 1 || got[0].Scope != "app/Navigation" {
		t.Fatalf("got %+v, want one binding scoped app/Navigation", got)
	}
}

// The scope join tolerates either half being absent, so an app with no
// sub-scope publishes bare section names rather than "/Navigation".
func TestHostedBindingsScopeJoin(t *testing.T) {
	keys := newHostedTestKeys()
	got := HostedBindings("", keys, keys.Quit)
	if len(got) != 1 || got[0].Scope != "System" {
		t.Fatalf("got %+v, want one binding scoped System", got)
	}
	if joined := joinHostedScope("app", ""); joined != "app" {
		t.Errorf("joinHostedScope(app, \"\") = %q, want %q", joined, "app")
	}
}

// The derived Action and ConfigKey must be the same strings the keys registry
// advertises for the same binding, or a hosted app's wire identity and its
// registry identity would diverge.
func TestHostedBindingsAgreeWithMakeTUIInfo(t *testing.T) {
	keys := newHostedTestKeys()
	info := MakeTUIInfo("timerapp", "timer", "test", keys)

	registry := make(map[string][2]string) // description -> {Name, ConfigKey}
	for _, s := range info.Sections {
		for _, b := range s.Bindings {
			registry[b.Description] = [2]string{b.Name, b.ConfigKey}
		}
	}
	for _, b := range HostedBindings("timer", keys) {
		want, ok := registry[b.Description]
		if !ok {
			t.Errorf("%q is on the wire but not in the registry export", b.Description)
			continue
		}
		if b.Action != want[0] || b.ConfigKey != want[1] {
			t.Errorf("%q: wire identity %s/%s, registry identity %s/%s",
				b.Description, b.Action, b.ConfigKey, want[0], want[1])
		}
	}
}

// Mutating the derived Keys must not reach back into the key map.
func TestHostedBindingsCopiesKeys(t *testing.T) {
	keys := newHostedTestKeys()
	got := HostedBindings("timer", keys, keys.Up)
	got[0].Keys[0] = "clobbered"
	if keys.Up.Keys()[0] != "k" {
		t.Fatalf("derived binding aliased the key map's chords: %v", keys.Up.Keys())
	}
}

func TestHostedBindingsNilKeyMap(t *testing.T) {
	if got := HostedBindings("timer", nil); got != nil {
		t.Fatalf("HostedBindings(nil) = %+v, want nil", got)
	}
}
