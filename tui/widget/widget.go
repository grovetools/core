// Package widget declares the contract a drawer widget implements, so that the
// component and its logic can live in the repository that owns the data it
// renders instead of in the host that mounts it.
//
// # Terminology
//
// The three words below are the whole vocabulary. They are used consistently in
// this package, in every widget package that implements the contract, and in
// the host's registration code:
//
//   - widget — the embeddable component described by a [Spec]: how to build it,
//     when it is available, what it says when it is empty, and which keys it
//     binds. A widget knows nothing about drawers, pages or slots.
//   - pane — a widget MOUNTED in a drawer slot. "Pane" is a position, not a
//     type: the same widget mounted twice is two panes.
//   - page — a named layout of panes, e.g. the built-in "git" page that stacks
//     state, changes and activity.
//
// The word "group" is deliberately absent. It is already spoken for three times
// over in this ecosystem (rail groups, nb note groups, the files trie's
// container rows), and a fourth meaning here would make every one of them
// ambiguous.
//
// # Message vocabulary
//
// Drawer panes never appear in the host's session panel tree, so they do not
// receive the host's ordinary panel fan-out. Everything a widget learns arrives
// as one of three broadcast INTAKE messages, and everything it asks for leaves
// as an action message:
//
//	intake   embed.SetWorkspaceMsg   the active workspace changed (see [SetWorkspaceMsg])
//	         VisibilityMsg           the drawer expanded or collapsed
//	         UpdateMsg               the host's tracker state changed; re-render
//
//	actions  LeaveFocusMsg           "I'm done here" — hand focus back to the
//	                                 main area, leave the drawer mounted
//
// [LeaveFocusMsg] is the only action every widget shares, so it is the only one
// declared here. A widget's own verbs — "open the git viewer at this path",
// "open the notebook at this note group" — are owned by that widget's package,
// next to the key handler that emits them; the host imports the package it
// mounts and switches on them there.
//
// A widget must tolerate every intake message arriving while it is collapsed
// and invisible: the host keeps the mounted page alive and keeps broadcasting.
// [VisibilityMsg] exists precisely so a widget that refreshes off background
// signals can stop paying for data nobody can see, and reconcile on reveal.
//
// # Empty-reason convention
//
// A widget that has nothing to show says WHY, in one muted line prefixed with
// theme.IconInfo, and distinguishes the reasons that call for different user
// action. The accessed-files widget is the exemplar: "no agent session focused"
// (focus an agent), "no files touched yet" (wait), and "file access not tracked
// for <provider>" (nothing will ever appear) are three different sentences, and
// collapsing them into one empty list would state a falsehood in two of the
// three cases.
//
// [RenderEmptyReason] renders the line. [Spec.EmptyReason] exposes the same
// sentence to the host, so a page map, a help overlay or a too-small
// placeholder can explain an absent pane without mounting it.
//
// # Settings placement
//
// A widget's settings block lives BESIDE the page definitions, never inside
// one: `[tui.drawer.files]`, not `[tui.drawer.pages.files.files]`. A widget is
// not owned by a page — any page's layout may mount it, and every mount of it
// wants the same setting — so nesting the block under one page would make the
// same widget behave differently depending on which page you reached it from.
// (The rationale is recorded on core/config's DrawerViewsConfig.Files field,
// which is the pattern to copy.)
package widget

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/grovetools/core/tui/theme"
	"github.com/grovetools/tuimux"
)

// Spec is a widget's registration entry: everything a host needs in order to
// mount, gate, describe and document a widget without knowing its type.
//
// Every func field is optional except Build. A zero func means "no opinion",
// and the accessor methods on Spec — not the host — decide what that resolves
// to, so a new consumer cannot invent a different default.
type Spec struct {
	// Name is the registry key, and the name a page layout mounts by. Lower
	// case, stable across releases: it appears in user config.
	Name string

	// Glyph is the page-map icon KEY (not a rendered glyph): the host resolves
	// it through its own icon table, so an icon-set switch re-renders without
	// the widget knowing. Empty means "use the map's neutral fallback".
	Glyph string

	// Build returns a FRESH panel. Drawer roots are disposable — the host
	// recompiles a page whenever its pane availability changes — so a Build
	// that returned a shared panel would leak state across recompiles and pin
	// scope resolved at registration time.
	Build func() tuimux.Panel

	// Available gates whether the pane is mounted at all. Nil means always
	// available. It is polled on every page compile and on every page-map
	// frame, so it must be cheap and must not block.
	Available func() bool

	// EmptyReason explains, in one actionable sentence, why the widget is
	// unavailable or has nothing to show right now. Nil, or "", means the
	// widget has nothing to add beyond its own rendering.
	//
	// Phrase it as a fact the reader can act on ("no agent session focused"),
	// never as an apology ("nothing to display").
	EmptyReason func() string

	// Keymap declares the keys the widget binds while focused. It is resolved
	// at call time so a widget whose bindings depend on its current mode (a
	// fold-aware tree, a two-view toggle) can say what is true NOW.
	//
	// Declaring bindings is not the same as rendering them: the drawer help
	// overlay is what renders these, and a widget that declares none simply
	// documents nothing.
	Keymap func() []KeyBinding

	// SizeHint is the widget's opinion about the space it needs. Nil means "no
	// opinion" and is the correct value for a widget that degrades gracefully
	// at any size.
	//
	// A host's page compiler weighs it when deciding whether a multi-column
	// layout still fits — a hint that cannot be met is what makes an explicit
	// split stack instead. A layout may state a minimum of its own, which
	// outranks this: the layout knows what one page is for, the widget only
	// knows what it needs wherever it is mounted.
	SizeHint SizeHint
}

// KeyBinding is one declared key of a widget.
type KeyBinding struct {
	// Key is the chord as bubbletea spells it ("enter", "ctrl+d", "q"), or
	// several separated by "/" when they are aliases of one action ("q/esc").
	Key string
	// Desc is the imperative one-liner ("copy path", "toggle tree view").
	Desc string
	// When narrows the binding to a mode or row kind ("on a stale row", "tree
	// view"). Empty means the binding is always live while the pane is focused.
	When string
	// Active evaluates the When condition against the widget's state RIGHT NOW.
	// Nil means "cannot say", which is different from "false": a renderer shows
	// such a binding normally, annotated with its When label.
	//
	// It exists so a help surface can DIM what does not currently apply instead
	// of hiding it. Hiding would make a binding that is one keystroke away look
	// like a binding that does not exist, and the user would have no way to
	// learn what to press first. Only a live panel can answer this — see
	// [Keymapper], which is how a renderer reaches one.
	Active func() bool
}

// Live reports whether the binding applies right now. A binding with no Active
// predicate is treated as live, so a widget that declares plain labels keeps
// rendering exactly as it did before predicates existed.
func (b KeyBinding) Live() bool { return b.Active == nil || b.Active() }

// Keymapper is implemented by a widget's PANEL when its bindings depend on
// state only the mounted instance holds — which view it is in, what is under
// the cursor, what kind of source it is following.
//
// [Spec.Keymap] answers the same question for a widget that has not been built
// (a page map, a help surface describing an unmounted pane), and is the
// fallback: a host asks the focused panel first and falls back to the spec, so
// a widget that has nothing mode-dependent to say implements nothing.
//
// The two must agree on the KEYS. A panel that returned a different set from
// its spec would make the drawer's help depend on whether the pane happened to
// be mounted, which is exactly the drift the contract exists to prevent — so
// the idiomatic implementation starts from the package-level declaration and
// only attaches [KeyBinding.Active] predicates to it.
type Keymapper interface {
	Keymap() []KeyBinding
}

// SizeHint is a widget's opinion about how much room it needs to be useful.
//
// It is consumed by a host's page compiler, which uses it to decide whether a
// layout can still be given the shape it asks for at the size the drawer is
// now. Implementations should be pure and cheap: it is polled during layout.
//
// It is a property of the WIDGET, fixed at registration and true wherever the
// widget is mounted. Its dynamic counterpart is a property of the mounted PANE:
// a panel that implements tuimux.SizeHintProvider tells the layout how many
// rows its CURRENT content can use, which is what lets a pane with nothing to
// show hand its rows to a content-bearing sibling. The two never disagree
// because they answer different questions — "how small can this widget be built
// at" versus "how much of what it was given is it using right now".
//
// A panel implementing the dynamic hint must report a bounded row count only
// for content that is genuinely bounded (an empty state, a fixed table). A hint
// that tracked a live stream row by row would move the layout on every append.
type SizeHint interface {
	// MinSize reports the smallest width and height at which the widget still
	// renders something worth showing. Either value may be 0, meaning "no
	// opinion on that axis".
	MinSize() (w, h int)
}

// MinSize is the trivial [SizeHint]: a fixed floor on either axis.
type MinSize struct{ W, H int }

func (m MinSize) MinSize() (int, int) { return m.W, m.H }

// IsAvailable reports the spec's availability, treating a nil gate as "always".
// Hosts must call this rather than testing Available themselves, so the nil
// default lives in exactly one place.
func (s Spec) IsAvailable() bool { return s.Available == nil || s.Available() }

// Reason returns the widget's empty/unavailable explanation, or "" when it has
// none. Safe on a zero Spec.
func (s Spec) Reason() string {
	if s.EmptyReason == nil {
		return ""
	}
	return s.EmptyReason()
}

// Bindings returns the widget's declared keys, or nil. Safe on a zero Spec.
func (s Spec) Bindings() []KeyBinding {
	if s.Keymap == nil {
		return nil
	}
	return s.Keymap()
}

// RenderEmptyReason renders one empty-state line in the drawer's convention:
// the muted info glyph, the reason, italic. Returns "" for an empty reason so a
// caller can hand through whatever [Spec.Reason] gave it.
//
// The implementation lives in core/tui/theme so that a view which cannot import
// this package — git-viewer's changes model, which this package's own widget
// wraps — still renders the identical line. This is the name to call from a
// widget; both spellings are the same one convention.
func RenderEmptyReason(reason string) string { return theme.RenderEmptyReason(reason) }

// HighlightStyle is the drawer's cursor/heading highlight. Read from the live
// theme at render time — a package-level capture would freeze the palette
// chosen at init and survive a re-theme.
func HighlightStyle() lipgloss.Style { return theme.DefaultTheme.Highlight }

// Truncate clips s to max cells, marking the cut with an ellipsis. It is the
// drawer's shared row truncation: a pane is often a few dozen columns wide, and
// every widget that renders rows needs the same rule.
func Truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) > max {
		return string(runes[:max-1]) + "…"
	}
	return s
}
