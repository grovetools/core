package pager

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// innerModel is a value-semantics bubbletea model of the shape the meta-panels
// wrap: Update returns a new copy of itself as a tea.Model.
type innerModel struct {
	body      string
	footer    string
	width     int
	height    int
	focused   bool
	typing    bool
	closed    *bool
	initCalls *int
}

func (m innerModel) Init() tea.Cmd {
	if m.initCalls != nil {
		*m.initCalls++
	}
	return nil
}

func (m innerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case FocusMsg:
		m.focused = true
	case BlurMsg:
		m.focused = false
	}
	return m, nil
}

func (m innerModel) View() string                  { return m.body }
func (m innerModel) FooterView() string            { return m.footer }
func (m innerModel) IsTextEntryActive() bool       { return m.typing }
func (m innerModel) TestState() map[string]any     { return map[string]any{"body": m.body} }
func (m innerModel) Close() error                  { *m.closed = true; return nil }
func (m innerModel) sized() (width, height int)    { return m.width, m.height }
func (m innerModel) withBody(s string) innerModel  { m.body = s; return m }
func (m innerModel) withFoot(s string) innerModel  { m.footer = s; return m }
func (m innerModel) withTyping(b bool) innerModel  { m.typing = b; return m }
func (m innerModel) withClosed(p *bool) innerModel { m.closed = p; return m }

func TestWrapRendersTheInnerBody(t *testing.T) {
	meta := Wrap(innerModel{}.withBody("INNER"), WrapConfig{Config: Config{HideTabBar: true}})
	meta, _ = meta.Step(tea.WindowSizeMsg{Width: 80, Height: 24})

	if !strings.Contains(meta.View(), "INNER") {
		t.Errorf("wrapped body not rendered:\n%s", meta.View())
	}
}

func TestWrapPinsTheInnerFooter(t *testing.T) {
	meta := Wrap(
		innerModel{}.withBody("INNER").withFoot("HELP-LINE"),
		WrapConfig{Config: Config{HideTabBar: true, FooterHeight: 1}},
	)
	meta, _ = meta.Step(tea.WindowSizeMsg{Width: 80, Height: 24})

	if !strings.Contains(meta.View(), "HELP-LINE") {
		t.Errorf("footer not pinned:\n%s", meta.View())
	}
}

func TestWrapTruncatesTheFooterToTheWidth(t *testing.T) {
	long := strings.Repeat("help ", 40)
	meta := Wrap(
		innerModel{}.withBody("INNER").withFoot(long),
		WrapConfig{
			Config:         Config{HideTabBar: true, FooterHeight: 1},
			TruncateFooter: true,
		},
	)
	meta, _ = meta.Step(tea.WindowSizeMsg{Width: 40, Height: 24})

	view := meta.View()
	for _, line := range strings.Split(view, "\n") {
		if len(line) > 200 { // generous: escapes inflate byte length
			t.Fatalf("a line survived at %d bytes; the footer was not truncated", len(line))
		}
	}
	if !strings.Contains(view, "…") {
		t.Errorf("truncated footer has no ellipsis:\n%s", view)
	}
}

// Asserted on the page adapter rather than the finished frame: the pager
// pads and joins the body afterwards, and whether a leading blank row
// survives that is lipgloss's business, not this option's.
func TestWrapTrimsTheLeadingNewline(t *testing.T) {
	trimmed := &wrapped[innerModel]{
		inner: innerModel{}.withBody("\nINNER"),
		cfg:   WrapConfig{TrimLeadingNewline: true},
	}
	if got := trimmed.View(); got != "INNER" {
		t.Errorf("View() = %q, want %q", got, "INNER")
	}

	kept := &wrapped[innerModel]{inner: innerModel{}.withBody("\nINNER")}
	if got := kept.View(); got != "\nINNER" {
		t.Errorf("View() = %q, want the newline kept", got)
	}

	// Only one newline comes off, and only when there is one.
	two := &wrapped[innerModel]{
		inner: innerModel{}.withBody("\n\nINNER"),
		cfg:   WrapConfig{TrimLeadingNewline: true},
	}
	if got := two.View(); got != "\nINNER" {
		t.Errorf("View() = %q, want exactly one newline removed", got)
	}
	none := &wrapped[innerModel]{
		inner: innerModel{}.withBody("INNER"),
		cfg:   WrapConfig{TrimLeadingNewline: true},
	}
	if got := none.View(); got != "INNER" {
		t.Errorf("View() = %q, want %q", got, "INNER")
	}
}

// The point of wrapping: the inner model is sized against the body, not the
// pane, so a tab bar and a footer come out of the pager's height rather than
// out of the model's own rendering.
func TestWrapSizesTheInnerModelAgainstTheBody(t *testing.T) {
	meta := Wrap(innerModel{}.withBody("INNER").withFoot("help"),
		WrapConfig{Config: Config{FooterHeight: 1}})
	meta, _ = meta.Step(tea.WindowSizeMsg{Width: 80, Height: 24})

	w, h := meta.Inner().sized()
	if w != 80 {
		t.Errorf("inner width = %d, want the pane width 80", w)
	}
	if h <= 0 || h >= 24 {
		t.Errorf("inner height = %d, want a positive height below the pane's 24 "+
			"(tab bar, spacer and footer come out of it)", h)
	}
}

func TestWrapDeliversFocusAndBlur(t *testing.T) {
	meta := Wrap(innerModel{}.withBody("INNER"), WrapConfig{Config: Config{HideTabBar: true}})
	meta, _ = meta.Step(tea.WindowSizeMsg{Width: 80, Height: 24})

	meta, _ = meta.Step(FocusMsg{})
	if !meta.Inner().focused {
		t.Error("FocusMsg did not reach the inner model")
	}
	meta, _ = meta.Step(BlurMsg{})
	if meta.Inner().focused {
		t.Error("BlurMsg did not reach the inner model")
	}
}

func TestWrapReportsTextEntry(t *testing.T) {
	meta := Wrap(innerModel{}.withTyping(true), WrapConfig{})
	if !meta.IsTextEntryActive() {
		t.Error("IsTextEntryActive did not reach the inner model")
	}

	quiet := Wrap(innerModel{}, WrapConfig{})
	if quiet.IsTextEntryActive() {
		t.Error("IsTextEntryActive = true for a model that is not typing")
	}
}

func TestWrapPassesThroughTestState(t *testing.T) {
	meta := Wrap(innerModel{}.withBody("INNER"), WrapConfig{})
	if got := meta.TestState()["body"]; got != "INNER" {
		t.Errorf("TestState()[body] = %v, want INNER", got)
	}
}

func TestWrapCloses(t *testing.T) {
	closed := false
	meta := Wrap(innerModel{}.withClosed(&closed), WrapConfig{})
	if err := meta.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}
	if !closed {
		t.Error("Close did not reach the inner model")
	}
}

// A model implementing none of the optional interfaces must still wrap.
type bareModel struct{}

func (bareModel) Init() tea.Cmd                       { return nil }
func (bareModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return bareModel{}, nil }
func (bareModel) View() string                        { return "BARE" }

func TestWrapWithNoOptionalInterfaces(t *testing.T) {
	meta := Wrap(bareModel{}, WrapConfig{Config: Config{HideTabBar: true}})
	meta, _ = meta.Step(tea.WindowSizeMsg{Width: 40, Height: 10})

	if !strings.Contains(meta.View(), "BARE") {
		t.Errorf("bare model not rendered:\n%s", meta.View())
	}
	if meta.IsTextEntryActive() {
		t.Error("IsTextEntryActive = true for a model that does not implement it")
	}
	if err := meta.Close(); err != nil {
		t.Errorf("Close() = %v, want nil for a model that does not implement it", err)
	}
	if len(meta.TestState()) != 0 {
		t.Errorf("TestState() = %v, want empty", meta.TestState())
	}
}

func TestWrapDefaultsTheTabName(t *testing.T) {
	meta := Wrap(innerModel{}.withBody("INNER"), WrapConfig{})
	meta, _ = meta.Step(tea.WindowSizeMsg{Width: 60, Height: 20})

	if !strings.Contains(meta.View(), "Browser") {
		t.Errorf("default tab name missing from the bar:\n%s", meta.View())
	}
}

// optModel implements every optional Inner interface at once, so each
// forwarding path can be exercised with the others left at an inert value.
// Constructed through newOpt so "ready" and "enabled" start true — a zero
// value would open not-ready and unswitchable, which is the opposite of what
// a model that never opted in should look like.
type optModel struct {
	minW, minH int
	title      string
	ready      bool
	loading    string
	enabled    bool
}

func newOpt() optModel { return optModel{ready: true, enabled: true} }

func (m optModel) Init() tea.Cmd                       { return nil }
func (m optModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func (m optModel) View() string                        { return "OPT-BODY" }
func (m optModel) MinSize() (int, int)                 { return m.minW, m.minH }
func (m optModel) Title() string                       { return m.title }
func (m optModel) Ready() (bool, string)               { return m.ready, m.loading }
func (m optModel) Enabled() bool                       { return m.enabled }

func (m optModel) withMinSize(w, h int) optModel { m.minW, m.minH = w, h; return m }
func (m optModel) withTitle(s string) optModel   { m.title = s; return m }
func (m optModel) notReady(msg string) optModel  { m.ready, m.loading = false, msg; return m }
func (m optModel) withEnabled(b bool) optModel   { m.enabled = b; return m }

// The defect this set covers: wrapped implemented none of these, so every
// optional Page extension was unreachable through Wrap however the inner model
// was written.
var (
	_ PageWithMinSize   = (*wrapped[optModel])(nil)
	_ PageWithTitle     = (*wrapped[optModel])(nil)
	_ PageWithReady     = (*wrapped[optModel])(nil)
	_ PageWithEnabled   = (*wrapped[optModel])(nil)
	_ PageWithTextInput = (*wrapped[optModel])(nil)
	_ PageWithID        = (*wrapped[optModel])(nil)
)

func TestWrapForwardsTheMinimumSize(t *testing.T) {
	meta := Wrap(newOpt().withMinSize(40, 10), WrapConfig{Config: Config{HideTabBar: true}})

	meta, _ = meta.Step(tea.WindowSizeMsg{Width: 80, Height: 24})
	if !strings.Contains(meta.View(), "OPT-BODY") {
		t.Errorf("a pane above the minimum did not render the body:\n%s", meta.View())
	}

	meta, _ = meta.Step(tea.WindowSizeMsg{Width: 30, Height: 24})
	narrow := meta.View()
	if strings.Contains(narrow, "OPT-BODY") {
		t.Errorf("a pane below the minimum width still rendered the body:\n%s", narrow)
	}
	if !strings.Contains(narrow, "too small") {
		t.Errorf("no placeholder in a pane below the minimum width:\n%s", narrow)
	}

	// Height is checked independently of width.
	meta, _ = meta.Step(tea.WindowSizeMsg{Width: 80, Height: 6})
	if strings.Contains(meta.View(), "OPT-BODY") {
		t.Errorf("a pane below the minimum height still rendered the body:\n%s", meta.View())
	}
}

// A model that declares no minimum keeps the opt-in default: it is never too
// small, however little room it is given.
func TestWrapWithoutAMinimumIsNeverTooSmall(t *testing.T) {
	meta := Wrap(bareModel{}, WrapConfig{Config: Config{HideTabBar: true}})
	meta, _ = meta.Step(tea.WindowSizeMsg{Width: 4, Height: 2})

	if !strings.Contains(meta.View(), "BARE") {
		t.Errorf("a model that declared no minimum was gated anyway:\n%s", meta.View())
	}
}

func TestWrapForwardsTheTitle(t *testing.T) {
	meta := Wrap(newOpt().withTitle("TITLE-ROW"),
		WrapConfig{Config: Config{HideTabBar: true, ShowTitleRow: true}})
	meta, _ = meta.Step(tea.WindowSizeMsg{Width: 80, Height: 24})

	if !strings.Contains(meta.View(), "TITLE-ROW") {
		t.Errorf("title row not rendered:\n%s", meta.View())
	}
}

func TestWrapForwardsReadiness(t *testing.T) {
	meta := Wrap(newOpt().notReady("Loading timers…"), WrapConfig{Config: Config{HideTabBar: true}})
	meta, _ = meta.Step(tea.WindowSizeMsg{Width: 80, Height: 24})

	view := meta.View()
	if strings.Contains(view, "OPT-BODY") {
		t.Errorf("a model that is not ready still rendered its body:\n%s", view)
	}
	if !strings.Contains(view, "Loading timers…") {
		t.Errorf("loading message not rendered:\n%s", view)
	}
}

// Enabled is inert on the single tab Wrap starts with, so the assertion is on
// a two-page pager built from the same adapter — the shape a panel reaches
// through Meta.Pager once it grows.
func TestWrapForwardsEnabled(t *testing.T) {
	first := &wrapped[optModel]{inner: newOpt(), cfg: WrapConfig{Name: "first"}}
	second := &wrapped[optModel]{inner: newOpt().withEnabled(false), cfg: WrapConfig{Name: "second"}}
	m := New([]Page{first, second}, DefaultKeyMap())

	jump2 := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")}
	m, _ = m.Update(jump2)
	if m.ActiveIndex() != 0 {
		t.Fatalf("a numeric jump landed on a disabled tab (index %d)", m.ActiveIndex())
	}

	second.inner = second.inner.withEnabled(true)
	m, _ = m.Update(jump2)
	if m.ActiveIndex() != 1 {
		t.Errorf("active tab = %d, want the now-enabled tab 1", m.ActiveIndex())
	}
}
