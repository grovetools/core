// Package hostedkeys defines the machine-readable key contract a hosted
// application publishes to whatever host is running it, and the shape a host
// answers with.
//
// A host such as treemux consumes its global hotkeys before the focused panel
// ever sees the keystroke. That is right for a PTY child and wrong for a
// hosted application that binds the same chord to a real action, so a hosted
// app declares the chords it wants back and the host arbitrates. The
// declaration is data, not code: flow publishes one in-process
// (flow/pkg/tui/view.HostedKeys), and a sidecar panel publishes the identical
// JSON over the embed/v1 control socket (core/panelkit/panelproto). Both are
// this type, so a host filters them through one code path.
//
// The package is deliberately stdlib-only — a sidecar or a standalone tool can
// import it without dragging in bubbletea, a config loader or a TUI runtime.
package hostedkeys

// SchemaVersion is the current Reference.SchemaVersion. It changes only for an
// incompatible shape change; additive fields do not bump it, because both
// sides decode with an ordinary JSON decoder that ignores what it does not
// know.
const SchemaVersion = 1

// Reference is one hosted application's complete key declaration.
type Reference struct {
	SchemaVersion int       `json:"schema_version"`
	App           string    `json:"app"`
	Bindings      []Binding `json:"bindings"`
}

// Binding is one declared binding: a key a component responds to, what it
// does, and the condition under which it applies. Keys is the stable surface a
// host joins on; everything else is description.
//
// It is THE shape for that statement in this ecosystem, and the other two
// vocabularies are projections onto it rather than rivals:
//
//   - bubbles key.Binding inside a keymap.SectionedKeyMap, which a hosted app
//     already writes for its own help overlay — projected by
//     keymap.HostedReference, so an app declares its keys once.
//   - widget.KeyBinding, which a drawer widget declares for the host's help
//     layers — projected by widget.HostedBindings, and reconstructed by
//     widget.BindingsFromHosted.
//
// It is the canonical one because it is the only one that can cross a process
// boundary: it is versioned (SchemaVersion), it is plain JSON on the embed/v1
// control socket, and it is what a sidecar written in another language emits.
// The other two carry Go values — a matcher the compiler checks, a live
// predicate — that no wire can transport, so the derivation only runs one way.
//
// # Scope, When and HostSwallowed are three different questions
//
// They read alike and are routinely confused, so each says which question it
// answers:
//
//   - Scope is WHERE the binding lives: the sub-application or mode it belongs
//     to, so a host can report "flow's browser binds this" rather than just
//     "flow does". It PARTITIONS a declaration, and a renderer turns it into a
//     heading over a group of rows.
//   - When is the narrower condition INSIDE a scope ("on a stale row", "tree
//     view"). It qualifies one binding, and a renderer puts it beside that row.
//   - HostSwallowed is not a condition at all: it says WHICH LAYER of the
//     declaring app acts on the key, not whether the binding applies. It is
//     advisory input to arbitration.
type Binding struct {
	// Scope names the sub-application or mode the binding belongs to, so a
	// host can report "flow's browser binds this" rather than just "flow does".
	Scope string `json:"scope"`
	// Action is the binding's stable identifier within its scope.
	Action string `json:"action"`
	// Keys are the chords, in the host's own key-string vocabulary
	// (bubbletea's for every grove TUI).
	Keys []string `json:"keys"`
	// Description is human-readable help text.
	Description string `json:"description"`
	// When narrows the binding to a mode or row kind within its scope ("on a
	// stale row", "tree view"). Empty means the binding is always live while
	// the declaring component holds focus.
	//
	// It carries the LABEL, never the answer. Whether the condition holds right
	// now is a question only a live component can answer, and a wire
	// declaration is a snapshot of what a component binds, not of what it is
	// doing — see widget.KeyBinding.Active, which is the in-process half and is
	// deliberately not expressible here. A renderer with only the label shows
	// the row normally, annotated; a renderer that can reach the live component
	// dims what does not currently apply.
	When string `json:"when,omitempty"`
	// ConfigKey names the config entry that rebinds this, when it has one.
	ConfigKey string `json:"config_key,omitempty"`
	// HostSwallowed marks a key the declaring app itself intercepts before
	// its own embedded sub-models see it. Advisory: it tells a host which
	// layer would act, not whether to defer.
	HostSwallowed bool `json:"host_swallowed"`
	// CollisionHints names host bindings this key is known to collide with.
	// Advisory only — a host joins on Keys.
	CollisionHints []string `json:"collision_hints,omitempty"`
}

// DeclaredKeys returns every distinct chord in the reference, in declaration
// order. Order is stable so a host's accepted/rejected lists are diffable
// across successive declarations.
func (r Reference) DeclaredKeys() []string {
	var keys []string
	seen := make(map[string]bool)
	for _, b := range r.Bindings {
		for _, k := range b.Keys {
			if k == "" || seen[k] {
				continue
			}
			seen[k] = true
			keys = append(keys, k)
		}
	}
	return keys
}

// Rejection is one declared chord a host declined to defer, with the reason.
// Reasons are stable strings, not prose: a hosted app is expected to branch on
// them (rebind, or accept that the key already reaches it).
type Rejection struct {
	Key    string `json:"key"`
	Reason string `json:"reason"`
}

// Rejection reasons. A host that cannot classify a refusal precisely should
// still pick one of these rather than invent a fifth.
const (
	// ReasonNonDeferrable — an escape hatch the host owns unconditionally
	// (its leader/action arm, ctrl+c, the session F-keys). Granting it could
	// strand the user inside the panel with no way back to the host.
	ReasonNonDeferrable = "non_deferrable"
	// ReasonHostWins — an unconditional host binding it keeps regardless.
	// Rebind on the hosted side.
	ReasonHostWins = "host_wins"
	// ReasonContextual — the host binding exists only in a state where the
	// declaring panel cannot hold focus (a focused sidebar, an armed chord),
	// so no collision exists. Nothing to grant.
	ReasonContextual = "contextual"
	// ReasonNoCollision — the host does not bind this chord at all. The
	// keystroke already reaches the hosted app. Nothing to grant.
	ReasonNoCollision = "no_collision"
)

// Grant is a host's answer to a Reference: what it will defer, and why it
// refused the rest.
//
// Claims is keyed for O(1) lookup on the key path (a host consults it on every
// keystroke) and must be treated as read-only by consumers. Accepted carries
// the same set in declaration order for reporting; Rejected explains every
// declared chord that is not in Claims, so a refusal is never silent.
//
// Ref retains the declaration the grant answers. Arbitration used to reduce a
// Reference to a set of chord strings, which threw away every human-readable
// thing the hosted app said about them: a host could report THAT it deferred
// ctrl+f and never what ctrl+f does. Keeping the declaration is what lets the
// help overlay, the which-key popup and the config keys page describe a hosted
// panel's keys in the app's own words.
type Grant struct {
	Claims   map[string]bool
	Accepted []string
	Rejected []Rejection

	// Ref is the declaration this grant answers, retained verbatim. Its
	// Bindings carry the Scope/Action/Description the wire already transported
	// and arbitration previously discarded.
	Ref Reference
}

// App names the declaring application, or "" for an empty grant.
func (g Grant) App() string { return g.Ref.App }

// GrantedBindings returns the declared bindings with at least one chord the
// host will defer, each paired with the subset of its chords that were
// actually granted.
//
// The pairing matters because a binding may declare several chords and the
// host can grant only some of them: rendering the whole Keys list next to a
// description would advertise a deferral the user does not have.
func (g Grant) GrantedBindings() []BoundKeys {
	var out []BoundKeys
	for _, b := range g.Ref.Bindings {
		var keys []string
		for _, k := range b.Keys {
			if g.Claims[k] {
				keys = append(keys, k)
			}
		}
		if len(keys) > 0 {
			out = append(out, BoundKeys{Binding: b, Keys: keys})
		}
	}
	return out
}

// SelfBindings returns the declared bindings the host granted nothing for.
//
// These are not failures: a chord the host does not bind at all already
// reaches the hosted app, which is why the commonest rejection reason is
// no_collision. They are the app's OWN keys, and they are most of what a user
// looking at the panel wants to see — a help overlay that listed only the
// arbitrated chords would show the two exotic ones and hide j/k/enter.
func (g Grant) SelfBindings() []BoundKeys {
	var out []BoundKeys
	for _, b := range g.Ref.Bindings {
		var keys []string
		for _, k := range b.Keys {
			if k != "" && !g.Claims[k] {
				keys = append(keys, k)
			}
		}
		if len(keys) > 0 {
			out = append(out, BoundKeys{Binding: b, Keys: keys})
		}
	}
	return out
}

// BoundKeys is one declared binding narrowed to the chords a caller asked
// about — the granted ones, or the non-granted ones. Binding is the whole
// declaration (Binding.Keys included); Keys is the subset this row describes.
// Deliberately not an embed: two fields named Keys meaning different sets is
// exactly the confusion this type exists to remove.
type BoundKeys struct {
	Binding Binding
	Keys    []string
}
