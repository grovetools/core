package keymap

import (
	"reflect"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/grovetools/core/tui/hostedkeys"
)

// The bridge between a hosted app's two key declarations.
//
// A hosted application says the same thing twice. It publishes a
// hostedkeys.Reference so the host can arbitrate its chords — scope, action,
// keys, description — and it builds a SectionedKeyMap of bubbles key.Bindings
// so the standard help overlay can render them. Nothing connected the two, so
// every panel wrote its chord list and, worse, its DESCRIPTIONS twice.
//
// Duplicated descriptions drift, and a drifted one is invisible: the host
// quotes the declaration (Grant.Ref survives arbitration precisely so the help
// overlay, the which-key popup and the config keys page can describe a panel's
// keys in the panel's own words), while the panel's own overlay quotes the key
// map. Two strings for one fact, rendered a keystroke apart, with nothing
// comparing them.
//
// The bridge runs one way on purpose. The key map is the source: it is the
// declaration the panel already has to write, it is the one the compiler
// checks (key.Matches takes a key.Binding, not a chord string), and its struct
// FIELD NAMES are a better stable action identity than anything a hand-written
// wire declaration tends to invent. The Reference is derived from it. There is
// no reverse direction because a Reference decoded off the wire has no
// key.Bindings to match against — a host rendering a hosted app's keys reads
// Grant.Ref directly, which is what internal/app/keyhints.go does.

// HostedReference derives a hosted application's key declaration from the key
// map its help overlay already renders, so one declaration drives both.
//
// app is the name the host records for the panel (sidecar.Options.App for a
// sidecar). scope names the sub-application or mode the bindings belong to and
// is joined with each section name — scope "timer" and section "Actions"
// publish as "timer/Actions" — so a host reports "breaktimer's Actions binds
// this" rather than just "breaktimer does". An empty scope publishes the bare
// section name.
//
// claims narrows the declaration to the listed bindings; passing none declares
// the whole key map. Narrowing is the common case for a sidecar, because a
// plugin manifest's [[tui.plugins.<name>.keys]] is a consent list the host
// compares the handshake against: declaring a chord the manifest never
// mentioned is reported as a divergence from what the user approved. Declaring
// everything is right for an app whose whole key map the host should be able
// to describe (flow does this), and it is what SelfBindings exists to serve.
//
//	var keys = timerKeys{
//	    Jump: key.NewBinding(key.WithKeys("ctrl+e"), key.WithHelp("ctrl+e", "jump to notebook")),
//	    ...
//	}
//	var keyRef = keymap.HostedReference("breaktimer", "timer", keys, keys.Jump)
func HostedReference(app, scope string, km SectionedKeyMap, claims ...key.Binding) hostedkeys.Reference {
	return hostedkeys.Reference{
		SchemaVersion: hostedkeys.SchemaVersion,
		App:           app,
		Bindings:      HostedBindings(scope, km, claims...),
	}
}

// HostedBindings is HostedReference's per-scope half, for an application that
// publishes several key maps under one Reference: derive each scope's bindings
// and concatenate them.
//
// Bindings come out in section order, then in binding order within a section,
// which is the order the help overlay renders — a host's accepted and rejected
// lists are then diffable across successive declarations, and they read in the
// order the panel thinks about its own keys.
//
// A binding is skipped when it is disabled or binds no keys: a chord the panel
// will not act on has no business being claimed from the host. A binding that
// appears in two sections is published once, under the first.
func HostedBindings(scope string, km SectionedKeyMap, claims ...key.Binding) []hostedkeys.Binding {
	if km == nil {
		return nil
	}

	// Field names are the action identity, matched back by signature exactly
	// as MakeTUIInfo does it — same map, same first-field-wins tiebreak on a
	// signature collision, so a binding's Action here and its Name in the keys
	// registry are the same string.
	names := make(map[string]string)
	for _, f := range collectBindingFields(reflect.ValueOf(km), "") {
		sig := bindingSignature(f.Binding)
		if _, ok := names[sig]; !ok {
			names[sig] = f.Name
		}
	}

	selected := make(map[string]bool, len(claims))
	for _, c := range claims {
		selected[bindingSignature(c)] = true
	}
	all := len(claims) == 0

	var out []hostedkeys.Binding
	published := make(map[string]bool)
	for _, section := range km.Sections() {
		for _, b := range section.Bindings {
			sig := bindingSignature(b)
			if published[sig] || (!all && !selected[sig]) {
				continue
			}
			if !b.Enabled() || len(b.Keys()) == 0 {
				continue
			}
			published[sig] = true
			out = append(out, hostedBinding(joinHostedScope(scope, section.Name), b, names[sig]))
		}
	}

	// A claim the key map's sections do not contain is still published, under
	// the bare scope. It is very likely a bug in the caller's Sections() — the
	// panel is claiming a chord its own help overlay will not list — but
	// dropping it would turn that into a binding that mysteriously never
	// fires, which is the failure mode the whole arbitration contract exists
	// to prevent.
	for _, c := range claims {
		sig := bindingSignature(c)
		if published[sig] || !c.Enabled() || len(c.Keys()) == 0 {
			continue
		}
		published[sig] = true
		out = append(out, hostedBinding(scope, c, names[sig]))
	}
	return out
}

// hostedBinding converts one key.Binding, with field the struct field name it
// was matched to ("" when it was not).
//
// HostSwallowed and CollisionHints are left zero. Both are statements about
// the relationship between this app and its host that a key map cannot know —
// which layer of the declaring app consumes the key, and which host binding it
// is known to collide with — so an app that has something to say there sets it
// on the derived bindings itself.
func hostedBinding(scope string, b key.Binding, field string) hostedkeys.Binding {
	help := b.Help()
	action, configKey := field, ConfigKeyForField(field)
	if field == "" {
		// Not backed by a struct field (constructed inline in Sections()).
		// Fall back to the description exactly as MakeTUIInfo does, so the two
		// exports never disagree about a binding's identity.
		action = help.Desc
		configKey = sanitizeConfigKey(toSnakeCase(strings.ReplaceAll(help.Desc, " ", "_")))
	}
	return hostedkeys.Binding{
		Scope:       scope,
		Action:      action,
		Keys:        append([]string(nil), b.Keys()...),
		Description: help.Desc,
		ConfigKey:   configKey,
	}
}

// joinHostedScope composes a binding's published scope from the app-level
// scope and its section, tolerating either being empty.
func joinHostedScope(scope, section string) string {
	switch {
	case scope == "":
		return section
	case section == "":
		return scope
	default:
		return scope + "/" + section
	}
}
