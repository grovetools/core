package keymap

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/tui/components/whichkey"
	"github.com/grovetools/core/tui/theme"
)

// WhichKeyHost is the reusable which-key mixin every Grove TUI embeds to get the
// namespace-chord mechanism (arm a single-key prefix, render a popup of pending
// completions, dispatch the two-key chord) without hand-wiring the glue at a
// dozen dispatch sites. It folds the chord seam (Update side) and the popup
// overlay (View side) into two calls: ProcessChord and RenderOverlay.
//
// Canonical wiring — the flat-tier integration is a single switch in Update plus
// one call in View:
//
//	// Update, top of `case tea.KeyMsg:`
//	res, b, cmd := m.WhichKey.ProcessChord(msg, extraFlatChords...)
//	switch res {
//	case keymap.ChordMatched:
//	    // re-synthesize the resolved chord and fall through to the flat switch,
//	    // or special-case the returned binding (e.g. gg → gotoTop)
//	    if len(b.Keys()) > 0 {
//	        msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(b.Keys()[0])}
//	    }
//	case keymap.ChordPending:
//	    return m, cmd            // popup pending; the tick re-renders after Delay
//	case keymap.ChordConsumed:
//	    return m, nil            // esc-cancel or stray-while-armed — swallow it
//	case keymap.ChordNone:
//	    // not a chord — fall through to normal dispatch
//	}
//
//	// View, where the final frame is assembled
//	frame = m.WhichKey.RenderOverlay(frame, lipgloss.Width(frame), *theme.DefaultTheme)
//
// The value type is embeddable in value-semantics Bubble Tea models: the
// *SequenceState pointer inside carries the mutable buffer, so a copied Model
// shares one arm-state (exactly how a bare *SequenceState field behaved before).
//
// Modal-tier TUIs (a top-level-only guard, e.g. nb search-mode / flow
// detail-focus) run their own mode/pane guard BEFORE calling ProcessChord and
// make it consult Armed()/IsPending() so a chord's continuation key is not stolen
// — the host supplies the predicate's inputs but never owns the predicate.
//
// Not host-owned: the "outer shell stands down during a chord" concern. flow's
// status model runs embedded in an outer view.Model that intercepts keys before
// delegating; those outer guards consult IsPending() and stay flow-local.
// Standalone TUIs have no such wrapper, so the host does not try to own it.
type WhichKeyHost struct {
	// Sequence is the shared arm-state engine (wait-indefinitely default — an
	// armed chord only resolves via match, esc-cancel, or a stray key; it never
	// silently expires). Exported so per-pane handlers can drive gg/G directly.
	Sequence *SequenceState
	// Namespaces are the single-key-prefix chord families (v…, c…, …) this TUI
	// declares, in wire order.
	Namespaces []Namespace
	// Delay is the which-key popup SHOW delay (from WhichKeyDelay(cfg)): a prefix
	// must be held this long before the popup renders, swallowing a fast chord.
	// Exported so tests can force it to 0.
	Delay time.Duration
}

// NewWhichKeyHost builds a host with a fresh wait-indefinitely SequenceState, the
// given namespaces (in wire order), and the show-delay resolved from cfg (a nil
// cfg yields the default delay).
func NewWhichKeyHost(cfg *config.Config, namespaces ...Namespace) WhichKeyHost {
	return WhichKeyHost{
		Sequence:   NewSequenceState(),
		Namespaces: namespaces,
		Delay:      WhichKeyDelay(cfg),
	}
}

// IsPending reports whether a chord is armed (buffer non-empty). Nil-safe so a
// zero-value host (a bare Model{} in a test) reads false rather than panicking.
func (h WhichKeyHost) IsPending() bool {
	return h.Sequence != nil && h.Sequence.IsPending()
}

// Armed reports whether any namespace prefix is currently armed (the buffer is a
// strict prefix of at least one member, so more completions remain). This is the
// input a modal-tier guard consults to stand down mid-chord; a flat gg buffer
// ("g") is NOT namespace-armed.
func (h WhichKeyHost) Armed() bool {
	if h.Sequence == nil {
		return false
	}
	buf := h.Sequence.Buffer()
	for _, ns := range h.Namespaces {
		if ns.Armed(buf) {
			return true
		}
	}
	return false
}

// PopupVisible reports whether the which-key popup should render: a chord must be
// pending AND armed for at least Delay. This is the SHOW gate — the delay swallows
// a fast two-key chord so the popup only appears on a deliberate hold.
func (h WhichKeyHost) PopupVisible() bool {
	return h.IsPending() && h.Sequence.PendingFor() >= h.Delay
}

// ChordResult is the four-way outcome the caller switches on after ProcessChord.
type ChordResult int

const (
	// ChordNone: not a chord — fall through to the TUI's flat dispatch.
	ChordNone ChordResult = iota
	// ChordPending: a prefix is armed; return (m, cmd) and wait for more input.
	ChordPending
	// ChordConsumed: esc dismissed an armed chord, or a stray key closed an armed
	// namespace menu — swallow the key and do nothing (return (m, nil)).
	ChordConsumed
	// ChordMatched: a chord completed — dispatch the returned binding.
	ChordMatched
)

// WhichKeyShowMsg is emitted by the SequencePending tick once the show-delay
// elapses; its only job is to force a Bubble Tea re-render so View can reveal the
// popup. It lives in core so every consumer shares one type. Consumers need no
// handler — Bubble Tea repaints after any Update pass — but may add an explicit
// no-op case for clarity.
type WhichKeyShowMsg struct{}

// ProcessChord folds the chord seam: it feeds msg through the shared sequence
// engine over [extra..., <all namespace members in wire order>] and returns the
// four-way result plus (on a match) the matched binding and (on a namespace
// pending) the delayed re-render tick.
//
// extra carries the TUI's flat sequence chords (e.g. the gg motion); pass them
// first so their indices stay stable. The matched binding lets the caller either
// special-case it (compare against a known binding) or re-synthesize its first
// key and fall through to the flat switch — the chord-only invariant makes
// Keys()[0] the chord itself, so key.Matches then resolves it.
func (h WhichKeyHost) ProcessChord(msg tea.KeyMsg, extra ...key.Binding) (ChordResult, key.Binding, tea.Cmd) {
	// Fresh slice: never append into the caller's extra backing array.
	bindings := make([]key.Binding, 0, len(extra)+len(h.Namespaces))
	bindings = append(bindings, extra...)
	for _, ns := range h.Namespaces {
		bindings = append(bindings, ns.Bindings...)
	}

	// Capture arm state BEFORE Process mutates the buffer: a stray key appends
	// then clears, so Armed() must be read first (the job-46 correction).
	wasArmed := h.Armed()

	res, idx := h.Sequence.Process(msg, bindings...)
	switch res {
	case SequenceMatch:
		h.Sequence.Clear()
		return ChordMatched, bindings[idx], nil
	case SequencePending:
		// A namespace prefix schedules a tick so the popup re-renders once the
		// show-delay elapses (Bubble Tea won't repaint without an event). A flat
		// prefix (gg) needs no tick — its footer hint is immediate.
		if h.Armed() {
			return ChordPending, key.Binding{}, tea.Tick(h.Delay, func(time.Time) tea.Msg {
				return WhichKeyShowMsg{}
			})
		}
		return ChordPending, key.Binding{}, nil
	case SequenceCancel:
		// esc dismissed an armed chord — consume it so it closes the popup instead
		// of falling through to a Back/Quit match.
		return ChordConsumed, key.Binding{}, nil
	default:
		h.Sequence.Clear()
		// A stray non-continuation key while a namespace menu was open closes the
		// menu and is consumed (the which-key idiom — "v" then "x" must not fire
		// the flat x-action). A flat single-key chord (gg) keeps falling through.
		if wasArmed {
			return ChordConsumed, key.Binding{}, nil
		}
		return ChordNone, key.Binding{}, nil
	}
}

// RenderOverlay is the View side: it composites the bottom-anchored which-key
// popup onto frame when the show-delay gate is open and a namespace is armed,
// otherwise it returns frame unchanged. width is the content width the popup band
// is drawn within (typically lipgloss.Width(frame)); t supplies the rule color.
//
// The popup is bottom-anchored (whichkey.OverlayBottom) with a full-width muted
// rule directly above it, so it reads as a docked panel just above the footer and
// keeps frame's height fixed. It falls back to a centered overlay only when the
// popup is taller than the frame (OverlayBottom's internal degenerate case).
func (h WhichKeyHost) RenderOverlay(frame string, width int, t theme.Theme) string {
	if !h.PopupVisible() {
		return frame
	}
	group, _ := ResolvePending(h.Sequence.Buffer(), h.Namespaces)
	if group == nil || len(group.Rows) == 0 {
		return frame
	}
	frameLines := len(strings.Split(frame, "\n"))
	// The namespace title is the popup header; the rows render under an untitled
	// group so the "View (v…)" label doesn't print twice (header + group title).
	popup := whichkey.RenderKeyGroups(group.Title, []whichkey.KeyGroup{{Title: "", Rows: group.Rows}}, t, width, frameLines)
	rule := t.Muted.Render(strings.Repeat("─", width))
	return whichkey.OverlayBottom(frame, popup, rule)
}

// FooterHint returns the immediate (undelayed) flat-chord footer indicator for
// the currently-armed buffer — "" when nothing is pending or a namespace popup is
// showing instead. Footer placement stays the caller's job; extra carries the
// flat sequence chords (typically CommonSequenceBindings(base)).
func (h WhichKeyHost) FooterHint(extra ...key.Binding) string {
	if !h.IsPending() {
		return ""
	}
	return PendingHint(h.Sequence.Buffer(), extra...)
}
