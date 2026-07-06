package keymap

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"

	"github.com/grovetools/core/tui/components/whichkey"
)

// Namespace groups a family of chord bindings behind a shared single-key
// prefix, e.g. a "View" namespace with prefix "v" whose members are declared
// key.WithKeys("vl"), key.WithKeys("vf"), … The chord machinery in sequence.go
// (SequenceState.Process / IsPrefix) already fires these two-key sequences by
// literal string-prefix match with no engine change: "vl" arms on "v" exactly
// as "gg" arms on "g". A Namespace is the display+help layer over that: while
// the prefix is armed it renders a which-key popup of the pending completions,
// and it exposes those same bindings to the ? help overlay as an ordinary
// keymap.Section.
//
// Precedence (mirrors SequenceState.Process, which checks MatchesAny before
// IsPrefixOfAny): an EXACT match fires before a pending prefix. A namespace
// prefix key must therefore be left UNBOUND as a flat key in the hosting TUI —
// the sanctioned g/gg shape — or the flat action fires and the chord never
// arms. Core only documents this invariant; the grove-side audit enforces it.
//
// ConfigKey stability: keep namespace members as NAMED struct fields on the
// TUI's keymap (e.g. ViewLogs key.Binding) and build the Namespace from those
// fields. MakeTUIInfo matches bindings back to fields by value signature
// (bindingSignature = Keys()+Help(), see audit.go), and collectBindingFields
// recurses through nested structs, so a field referenced through the Bindings
// slice still resolves to a stable snake_case ConfigKey. Do not construct
// namespace members inline, or they fall back to a description-derived key.
type Namespace struct {
	Prefix   string        // "v"
	Label    string        // "View"
	Bindings []key.Binding // declared key.WithKeys("vl"), key.WithKeys("vf"), …
}

// Armed reports whether the given sequence buffer has this namespace's prefix
// armed: the buffer is non-empty, starts with Prefix, and is still a strict
// prefix of at least one enabled member binding (i.e. more completions remain).
// Reuses IsPrefix from sequence.go rather than reimplementing prefix logic.
func (ns Namespace) Armed(buffer string) bool {
	if buffer == "" || !strings.HasPrefix(buffer, ns.Prefix) {
		return false
	}
	for _, b := range ns.Bindings {
		if b.Enabled() && IsPrefix(buffer, b) {
			return true
		}
	}
	return false
}

// PendingRows returns the completion rows to display for the given buffer: one
// row per enabled member whose key still extends the buffer, keyed by the
// remaining suffix. For buffer "v" a namespace of {vl,vf,vv} yields rows
// l/f/v; for buffer "vf" it narrows to the single vf row (empty remainder).
func (ns Namespace) PendingRows(buffer string) []whichkey.KeyRow {
	var rows []whichkey.KeyRow
	for _, b := range ns.Bindings {
		if !b.Enabled() {
			continue
		}
		for _, k := range b.Keys() {
			if strings.HasPrefix(k, buffer) {
				rows = append(rows, whichkey.KeyRow{
					Keys: strings.TrimPrefix(k, buffer),
					Desc: b.Help().Desc,
				})
				break // one row per binding, using the first matching key
			}
		}
	}
	return rows
}

// KeyGroup renders this namespace's pending completions for the given buffer as
// a titled which-key group, e.g. title "View (v…)".
func (ns Namespace) KeyGroup(buffer string) whichkey.KeyGroup {
	return whichkey.KeyGroup{
		Title: ns.title(),
		Rows:  ns.PendingRows(buffer),
	}
}

// Section renders this namespace as a help section so MakeTUIInfo and the ?
// overlay list its members (vl, vf, …) as ordinary bindings. No export-schema
// change is needed: ExportBinding copies multi-char keys verbatim and the
// registry already stores chords like "gg"/"zo".
func (ns Namespace) Section() Section {
	return Section{Name: ns.title(), Bindings: ns.Bindings}
}

// title is the shared "Label (Prefix…)" header used by both KeyGroup and Section.
func (ns Namespace) title() string {
	return fmt.Sprintf("%s (%s…)", ns.Label, ns.Prefix)
}

// PendingHint returns a short footer indicator for the EXISTING flat chords
// (gg, dd, yy, z*) so single-key arming is no longer invisible — the fleet-wide
// fix for the "pressed d, nothing happened" complaint. Given a partially-typed
// buffer it lists the chords it could still complete, e.g. "g… (gg top)"; it
// returns "" when the buffer completes none of the supplied bindings.
func PendingHint(buffer string, bindings ...key.Binding) string {
	if buffer == "" {
		return ""
	}
	var parts []string
	for _, b := range bindings {
		if !b.Enabled() || !IsPrefix(buffer, b) {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s %s", b.Help().Key, b.Help().Desc))
	}
	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf("%s… (%s)", buffer, strings.Join(parts, ", "))
}

// ResolvePending is the single call the consuming TUI's View makes while a
// sequence is pending. It returns the popup group for the first armed
// namespace, or — if no namespace is armed — a flat-chord footer hint built
// from the extra bindings (typically CommonSequenceBindings(base)). Exactly one
// of the two results is non-empty (or neither, when nothing is pending).
func ResolvePending(buffer string, namespaces []Namespace, extra ...key.Binding) (*whichkey.KeyGroup, string) {
	for _, ns := range namespaces {
		if ns.Armed(buffer) {
			g := ns.KeyGroup(buffer)
			return &g, ""
		}
	}
	return nil, PendingHint(buffer, extra...)
}
