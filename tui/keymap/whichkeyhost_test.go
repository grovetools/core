package keymap

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/grovetools/core/tui/theme"
)

// testHost builds a host with a flat gg motion (Top) plus a two-member "v" View
// namespace — enough to drive every branch of ProcessChord.
func testHost() (WhichKeyHost, key.Binding) {
	top := key.NewBinding(key.WithKeys("gg"), key.WithHelp("gg", "top"))
	ns := Namespace{Prefix: "v", Label: "View", Bindings: []key.Binding{
		key.NewBinding(key.WithKeys("vl"), key.WithHelp("vl", "logs")),
		key.NewBinding(key.WithKeys("vf"), key.WithHelp("vf", "frontmatter")),
	}}
	return NewWhichKeyHost(nil, ns), top
}

func runeMsg(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

// TestProcessChordMatchReturnsBinding: "v" arms (pending), "l" completes and
// returns the ViewLogs binding.
func TestProcessChordMatchReturnsBinding(t *testing.T) {
	h, top := testHost()
	if res, _, _ := h.ProcessChord(runeMsg("v"), top); res != ChordPending {
		t.Fatalf("'v' → %v, want ChordPending", res)
	}
	res, b, _ := h.ProcessChord(runeMsg("l"), top)
	if res != ChordMatched {
		t.Fatalf("'vl' → %v, want ChordMatched", res)
	}
	if len(b.Keys()) == 0 || b.Keys()[0] != "vl" {
		t.Errorf("matched binding = %v, want keys [vl]", b.Keys())
	}
	if h.IsPending() {
		t.Errorf("buffer should be cleared after a match")
	}
}

// TestProcessChordNamespacePendingTicks: arming a namespace prefix returns a
// non-nil tick cmd (the delayed popup re-render).
func TestProcessChordNamespacePendingTicks(t *testing.T) {
	h, top := testHost()
	res, _, cmd := h.ProcessChord(runeMsg("v"), top)
	if res != ChordPending {
		t.Fatalf("'v' → %v, want ChordPending", res)
	}
	if cmd == nil {
		t.Errorf("arming a namespace should schedule a which-key tick")
	}
	if !h.Armed() {
		t.Errorf("'v' should report Armed")
	}
}

// TestProcessChordFlatPendingNoTick: the flat gg prefix is pending but schedules
// no tick (its footer hint is immediate) and is NOT namespace-armed.
func TestProcessChordFlatPendingNoTick(t *testing.T) {
	h, top := testHost()
	res, _, cmd := h.ProcessChord(runeMsg("g"), top)
	if res != ChordPending {
		t.Fatalf("'g' → %v, want ChordPending", res)
	}
	if cmd != nil {
		t.Errorf("the flat gg prefix should not schedule a tick")
	}
	if h.Armed() {
		t.Errorf("a pending 'g' must not report namespace-Armed")
	}
}

// TestProcessChordEscWhileArmedConsumes: esc while armed cancels the chord and is
// consumed, buffer cleared.
func TestProcessChordEscWhileArmedConsumes(t *testing.T) {
	h, top := testHost()
	h.ProcessChord(runeMsg("v"), top)
	res, _, _ := h.ProcessChord(tea.KeyMsg{Type: tea.KeyEsc}, top)
	if res != ChordConsumed {
		t.Fatalf("esc-while-armed → %v, want ChordConsumed", res)
	}
	if h.IsPending() {
		t.Errorf("esc should clear the armed buffer (buffer=%q)", h.Sequence.Buffer())
	}
}

// TestProcessChordStrayWhileArmedConsumes: a stray non-continuation key while a
// namespace menu is armed closes the menu and is consumed.
func TestProcessChordStrayWhileArmedConsumes(t *testing.T) {
	h, top := testHost()
	h.ProcessChord(runeMsg("v"), top)
	res, _, _ := h.ProcessChord(runeMsg("x"), top)
	if res != ChordConsumed {
		t.Fatalf("stray-while-armed → %v, want ChordConsumed", res)
	}
	if h.IsPending() {
		t.Errorf("stray key should clear the buffer (buffer=%q)", h.Sequence.Buffer())
	}
}

// TestProcessChordUnknownIdleFallsThrough: an unknown key while idle is not ours.
func TestProcessChordUnknownIdleFallsThrough(t *testing.T) {
	h, top := testHost()
	res, _, _ := h.ProcessChord(runeMsg("x"), top)
	if res != ChordNone {
		t.Fatalf("unknown-idle → %v, want ChordNone", res)
	}
}

// TestRenderOverlayGatedByDelay: under a huge delay a freshly-armed chord leaves
// the frame untouched; at delay 0 the popup renders bottom-anchored with the
// frame height preserved.
func TestRenderOverlayGatedByDelay(t *testing.T) {
	var rows []string
	for i := 0; i < 20; i++ {
		rows = append(rows, "r"+string(rune('a'+i)))
	}
	frame := strings.Join(rows, "\n")
	width := 80
	th := *theme.DefaultTheme

	// Huge delay: fast chord must not show → frame unchanged.
	slow, top := testHost()
	slow.Delay = time.Hour
	slow.ProcessChord(runeMsg("v"), top)
	if got := slow.RenderOverlay(frame, width, th); got != frame {
		t.Errorf("a fast chord under a large delay must leave the frame unchanged")
	}

	// Delay 0: popup renders.
	fast, top2 := testHost()
	fast.Delay = 0
	fast.ProcessChord(runeMsg("v"), top2)
	out := fast.RenderOverlay(frame, width, th)
	if out == frame {
		t.Fatalf("delay 0 should render the popup")
	}
	lines := strings.Split(out, "\n")
	if len(lines) != 20 {
		t.Errorf("overlay changed frame height: got %d lines, want 20", len(lines))
	}
	title := -1
	for i, l := range lines {
		if strings.Contains(l, "View (v") {
			title = i
			break
		}
	}
	if title < 0 {
		t.Fatalf("popup title not found in overlay:\n%s", out)
	}
	if title < len(lines)/2 {
		t.Errorf("popup title at line %d of %d — expected the lower half (bottom-anchored)", title, len(lines))
	}
}

// TestFooterHintImmediate: a flat prefix produces an immediate (undelayed) hint;
// idle produces none.
func TestFooterHintImmediate(t *testing.T) {
	h, top := testHost()
	if hint := h.FooterHint(top); hint != "" {
		t.Errorf("idle FooterHint should be empty, got %q", hint)
	}
	h.ProcessChord(runeMsg("g"), top)
	hint := h.FooterHint(top)
	if !strings.Contains(hint, "gg") {
		t.Errorf("armed flat 'g' FooterHint = %q, want it to mention the gg chord", hint)
	}
}
