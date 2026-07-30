package pager

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// countingPage records how a focus transition arrived: through the Page
// methods, or through Update as a message. Both are counted because the
// contract is not "Focus is called" but "the transition happens once" — a page
// that saw it on both paths would run its side effect twice.
type countingPage struct {
	name        string
	focusCalls  int
	blurCalls   int
	focusMsgs   int
	blurMsgs    int
	updateCalls int
	// cmd is what Update hands back. The pager used to consult it to decide
	// whether to also call Focus, so a page that returned work here would
	// silently lose its Focus call.
	cmd tea.Cmd
}

func (p *countingPage) Name() string  { return p.name }
func (p *countingPage) Init() tea.Cmd { return nil }
func (p *countingPage) View() string  { return p.name }

func (p *countingPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	p.updateCalls++
	switch msg.(type) {
	case FocusMsg:
		p.focusMsgs++
	case BlurMsg:
		p.blurMsgs++
	}
	return p, p.cmd
}

func (p *countingPage) Focus() tea.Cmd {
	p.focusCalls++
	return nil
}

func (p *countingPage) Blur()            { p.blurCalls++ }
func (p *countingPage) SetSize(_, _ int) {}

func (p *countingPage) transitions() (focused, blurred int) {
	return p.focusCalls + p.focusMsgs, p.blurCalls + p.blurMsgs
}

func TestFocusReachesThePageExactlyOnce(t *testing.T) {
	page := &countingPage{name: "one"}
	m := New([]Page{page}, DefaultKeyMap())

	m, _ = m.Update(FocusMsg{})
	m, _ = m.Update(BlurMsg{})

	focused, blurred := page.transitions()
	if focused != 1 {
		t.Errorf("focus transitions = %d, want exactly 1 (Focus %d, FocusMsg %d)",
			focused, page.focusCalls, page.focusMsgs)
	}
	if blurred != 1 {
		t.Errorf("blur transitions = %d, want exactly 1 (Blur %d, BlurMsg %d)",
			blurred, page.blurCalls, page.blurMsgs)
	}
	if page.focusCalls != 1 || page.blurCalls != 1 {
		t.Errorf("Focus/Blur calls = %d/%d, want 1/1: the page methods are the "+
			"canonical delivery", page.focusCalls, page.blurCalls)
	}
}

// The defect this covers: Focus used to be called only when the page's Update
// returned a nil command, so a page that did any work on focus never got its
// Focus call at all.
func TestFocusIsDeliveredToAPageWhoseUpdateReturnsWork(t *testing.T) {
	busy := &countingPage{
		name: "busy",
		cmd:  func() tea.Msg { return nil },
	}
	m := New([]Page{busy}, DefaultKeyMap())

	m, _ = m.Update(FocusMsg{})

	if busy.focusCalls != 1 {
		t.Errorf("Focus calls = %d, want 1 regardless of what Update returns", busy.focusCalls)
	}
	if busy.focusMsgs != 0 {
		t.Errorf("FocusMsg reached Update %d times, want 0", busy.focusMsgs)
	}
}

// The command a page returns from Focus is the pager's, to hand up the tree:
// dropping it would strand any work the transition scheduled.
func TestFocusCommandIsReturned(t *testing.T) {
	page := &focusCmdPage{}
	m := New([]Page{page}, DefaultKeyMap())

	_, cmd := m.Update(FocusMsg{})
	if cmd == nil {
		t.Fatal("Update(FocusMsg) returned no command, want the page's focus command")
	}
	if _, ok := cmd().(focusRan); !ok {
		t.Error("the returned command is not the page's focus command")
	}
}

type focusRan struct{}

// focusCmdPage schedules work from Focus, the way a page that starts a poll or
// a refresh on becoming visible does.
type focusCmdPage struct{ countingPage }

func (p *focusCmdPage) Focus() tea.Cmd {
	p.focusCalls++
	return func() tea.Msg { return focusRan{} }
}

func (p *focusCmdPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	_, cmd := p.countingPage.Update(msg)
	return p, cmd
}

// A tab switch is the pager's other focus transition, and it goes through the
// same door: the page left gets one blur, the page arrived at one focus.
func TestTabSwitchTransitionsEachPageOnce(t *testing.T) {
	first := &countingPage{name: "first"}
	second := &countingPage{name: "second"}
	m := New([]Page{first, second}, DefaultKeyMap())

	m, _ = m.Update(SwitchTabMsg{TabIndex: 1})

	if _, blurred := first.transitions(); blurred != 1 {
		t.Errorf("outgoing page blur transitions = %d, want 1", blurred)
	}
	if focused, _ := second.transitions(); focused != 1 {
		t.Errorf("incoming page focus transitions = %d, want 1", focused)
	}
	if first.focusMsgs+first.blurMsgs+second.focusMsgs+second.blurMsgs != 0 {
		t.Error("a tab switch put a focus message through Update")
	}
}

// Wrap's inner model is the case the SDK ships: it reacts to FocusMsg, and
// wrapped is what turns the Focus call back into that message. It must arrive
// once.
func TestWrapDeliversFocusToTheInnerModelOnce(t *testing.T) {
	var focus, blur int
	meta := Wrap(countingInner{focus: &focus, blur: &blur}, WrapConfig{})

	meta, _ = meta.Step(FocusMsg{})
	meta, _ = meta.Step(BlurMsg{})

	if focus != 1 {
		t.Errorf("inner model saw FocusMsg %d times, want exactly 1", focus)
	}
	if blur != 1 {
		t.Errorf("inner model saw BlurMsg %d times, want exactly 1", blur)
	}
}

// countingInner counts through pointers so the count survives the value-copy
// Update the wrapped models use.
type countingInner struct {
	focus *int
	blur  *int
}

func (m countingInner) Init() tea.Cmd { return nil }
func (m countingInner) View() string  { return "INNER" }

func (m countingInner) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case FocusMsg:
		*m.focus++
	case BlurMsg:
		*m.blur++
	}
	return m, nil
}
