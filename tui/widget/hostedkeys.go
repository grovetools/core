package widget

import (
	"strings"

	"github.com/grovetools/core/tui/hostedkeys"
)

// The projection between a drawer widget's key declaration and the ecosystem's
// wire shape for one.
//
// Two vocabularies had grown up for the same statement — "a key this component
// responds to, its description, and the condition under which it applies". A
// drawer widget declares [KeyBinding]{Key, Desc, When, Active} and the host's
// help overlay renders it; a hosted application declares
// hostedkeys.Binding{Scope, Action, Keys, Description} and the host arbitrates
// it. Same job, no bridge, and a host that wanted to describe both surfaces
// uniformly had to carry two row types all the way to the renderer.
//
// hostedkeys.Binding is the one that survives, because it is the only one that
// can cross a process boundary: versioned, plain JSON, emitted by sidecars
// written in other languages. This file makes KeyBinding a PROJECTION of it
// rather than a second dialect — [HostedBindings] one way, [BindingsFromHosted]
// the other — so a host resolves every key surface it renders into one shape.
// The same move keymap.HostedReference already made for bubbles key.Bindings.
//
// Neither direction is lossy in the way that matters, and the one thing that
// does not survive is the one that cannot: [KeyBinding.Active] is a live
// predicate over the mounted widget's state, and there is no wire
// representation of a function. Dropping it lands on hostedkeys.Binding's
// documented meaning for an absent predicate — the When LABEL travels, the
// answer does not — which is exactly what a renderer with no live component to
// ask should show.

// HostedBindings projects a widget's declared keys onto the wire shape, under
// the given scope.
//
// scope is the partition the bindings belong to — for a drawer widget, its pane
// name — so a renderer can title the group in the same words the page map uses.
// Action falls back to the description because a widget declares no stable
// identifier of its own: unlike a bubbles key map, whose struct field names are
// the identity keymap.HostedBindings uses, a widget's bindings are a plain
// slice with nothing but their text to name them.
func HostedBindings(scope string, bindings []KeyBinding) []hostedkeys.Binding {
	if len(bindings) == 0 {
		return nil
	}
	out := make([]hostedkeys.Binding, 0, len(bindings))
	for _, b := range bindings {
		out = append(out, hostedkeys.Binding{
			Scope:       scope,
			Action:      b.Desc,
			Keys:        SplitKeyAliases(b.Key),
			Description: b.Desc,
			When:        b.When,
		})
	}
	return out
}

// BindingsFromHosted reconstructs widget bindings from a wire declaration —
// what a host does with a hosted panel's granted claims when it wants to render
// them through the same code path as a drawer widget's keys.
//
// Active comes back nil, which is [KeyBinding]'s documented "cannot say" rather
// than "false": nothing on the far side of a socket can be asked whether its
// condition holds right now, and a renderer that dimmed these rows would be
// asserting something it never learned.
//
// Description is preferred over Action for Desc, falling back to it: a binding
// with no help text still deserves a row, and its identifier is the only thing
// left to name it with.
func BindingsFromHosted(bindings []hostedkeys.Binding) []KeyBinding {
	if len(bindings) == 0 {
		return nil
	}
	out := make([]KeyBinding, 0, len(bindings))
	for _, b := range bindings {
		desc := b.Description
		if desc == "" {
			desc = b.Action
		}
		out = append(out, KeyBinding{
			Key:  JoinKeyAliases(b.Keys),
			Desc: desc,
			When: b.When,
		})
	}
	return out
}

// SplitKeyAliases turns a declared key column ("q/esc", "ctrl+u/ctrl+d") into
// the individual chords the wire shape carries.
//
// A bare "/" is a legal key on its own, so a column that IS "/" survives rather
// than splitting into nothing. That case is not hypothetical — it is the
// search key in half the TUIs in this ecosystem.
func SplitKeyAliases(key string) []string {
	if key = strings.TrimSpace(key); key == "" {
		return nil
	}
	if key == "/" {
		return []string{"/"}
	}
	var out []string
	for _, part := range strings.Split(key, "/") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// JoinKeyAliases is SplitKeyAliases' inverse: the chord column a widget
// declares for a set of aliases.
func JoinKeyAliases(keys []string) string { return strings.Join(keys, "/") }
