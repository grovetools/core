package pager

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/grovetools/core/tui/keymap"
)

// Inner is what Wrap needs from the model it adapts: an ordinary bubbletea
// model. Everything beyond this — a footer, text-entry state, a debug snapshot
// — is discovered at runtime through the optional interfaces below, so a model
// pays for only what it implements.
type Inner interface {
	Init() tea.Cmd
	Update(tea.Msg) (tea.Model, tea.Cmd)
	View() string
}

// InnerWithFooter is an Inner that supplies the meta-panel's footer line,
// typically its help text. Wrap pins it below the body on every frame.
type InnerWithFooter interface {
	FooterView() string
}

// InnerWithTextEntry is an Inner that can absorb navigation keys while a text
// field is focused. Wrap forwards the answer up so both the pager and the host
// multiplexer stand down.
type InnerWithTextEntry interface {
	IsTextEntryActive() bool
}

// InnerWithTestState is an Inner that publishes a state snapshot to the debug
// API.
type InnerWithTestState interface {
	TestState() map[string]interface{}
}

// InnerWithClose is an Inner holding a resource to release when the panel goes
// away.
type InnerWithClose interface {
	Close() error
}

// InnerWithMinSize is an Inner that declares the smallest body it can draw
// anything meaningful in. Wrap forwards it to the pager's automatic too-small
// gate, so the model gets TooSmallPlaceholder in place of its own view without
// checking the pane size itself. See PageWithMinSize for what a minimum is —
// and, more to the point, is not — for.
type InnerWithMinSize interface {
	MinSize() (width, height int)
}

// InnerWithTitle is an Inner that supplies the bold title row the pager renders
// above the body when configured with ShowTitleRow.
type InnerWithTitle interface {
	Title() string
}

// InnerWithReady is an Inner that loads asynchronously. Until it reports ready,
// the pager renders a centered loading message in place of its view.
type InnerWithReady interface {
	Ready() (ready bool, loadingMsg string)
}

// InnerWithEnabled is an Inner that can decline to be switched to: its tab is
// dimmed, and numeric jumps and [/] cycling skip it. Inert while Wrap's single
// tab is the whole pager, and the reason to declare it is the panel that grows
// past one — see Meta.Pager.
type InnerWithEnabled interface {
	Enabled() bool
}

// WrapConfig configures a wrapped meta-panel.
type WrapConfig struct {
	// Config is the pager's own configuration: padding, footer reservation,
	// whether the tab bar shows.
	Config

	// Name is the tab label. Defaults to "Browser".
	Name string

	// TabID is the stable identifier for SwitchTabMsg deep links. Optional.
	TabID string

	// Keys are the pager's navigation bindings. The zero value is derived
	// from keymap.NewBase(), which is what every meta-panel in the ecosystem
	// was passing by hand.
	Keys *KeyMap

	// TrimLeadingNewline strips one leading newline from the inner view.
	//
	// A model that also runs standalone usually opens with a blank row to
	// leave a gap above its title. Hosted, the pager already puts a blank row
	// between the tab bar and the body, and with HideTabBar the body starts at
	// the top of the pane with nothing to leave a gap from — either way the
	// row is doubled. This is only free if the inner model drops that row from
	// its own height accounting when hosted; otherwise it clips a row instead.
	TrimLeadingNewline bool

	// TruncateFooter clips the footer to the pager's width.
	//
	// Footer help text is one long unwrapped line. FooterHeight reserves
	// exactly one row for it, so a footer that wrapped to two would take the
	// extra row out of the body — which in a half-width panel is where it
	// hurts.
	TruncateFooter bool
}

// Meta is a single-page pager wrapped around one model: the tabbed meta-panel
// shape five repos had each written out by hand, in about a hundred lines each,
// with the same adapter, the same footer plumbing and the same leading-newline
// workaround.
//
// The wrapping is not ceremony. It is what gives a plain bubbletea model the
// host contract — sized against pager chrome, focus and blur delivered, a
// pinned footer, a tab bar to grow into — without the model knowing it is
// hosted. The single tab is a starting point, not the design: Pager gives
// access to the underlying pager for a panel that grows more.
type Meta[T Inner] struct {
	pager Model
	page  *wrapped[T]
}

// Wrap builds a meta-panel around inner.
func Wrap[T Inner](inner T, cfg WrapConfig) Meta[T] {
	if cfg.Name == "" {
		cfg.Name = "Browser"
	}
	keys := KeyMapFromBase(keymap.NewBase())
	if cfg.Keys != nil {
		keys = *cfg.Keys
	}
	page := &wrapped[T]{inner: inner, cfg: cfg}
	return Meta[T]{
		pager: NewWith([]Page{page}, keys, cfg.Config),
		page:  page,
	}
}

// Init starts the pager and the inner model.
func (m Meta[T]) Init() tea.Cmd { return m.pager.Init() }

// Step advances the panel and returns the updated Meta. It is the
// value-semantics form; Update is the tea.Model conformance over it.
func (m Meta[T]) Step(msg tea.Msg) (Meta[T], tea.Cmd) {
	var cmd tea.Cmd
	m.pager, cmd = m.pager.Update(msg)
	return m, cmd
}

// Update implements tea.Model.
func (m Meta[T]) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return m.Step(msg)
}

// View renders the panel, refreshing the pinned footer from the inner model
// first.
func (m Meta[T]) View() string {
	if f, ok := any(m.page.inner).(InnerWithFooter); ok {
		footer := f.FooterView()
		if m.page.cfg.TruncateFooter {
			if w, _ := m.pager.Size(); w > 0 && lipgloss.Width(footer) > w {
				footer = ansi.Truncate(footer, w, "…")
			}
		}
		m.pager.SetFooter(footer)
	}
	return m.pager.View()
}

// Inner returns the wrapped model, for a host that needs at its state.
func (m Meta[T]) Inner() T { return m.page.inner }

// Pager returns the underlying pager, for a panel growing past one tab.
func (m Meta[T]) Pager() Model { return m.pager }

// IsTextEntryActive reports whether the inner model has a focused text field,
// so the host multiplexer suspends its own navigation bindings.
func (m Meta[T]) IsTextEntryActive() bool {
	if t, ok := any(m.page.inner).(InnerWithTextEntry); ok {
		return t.IsTextEntryActive()
	}
	return false
}

// TestState returns the inner model's debug snapshot, or an empty map when it
// publishes none. A panel that wants to add its own keys should call this and
// extend the result rather than replace it.
func (m Meta[T]) TestState() map[string]interface{} {
	if s, ok := any(m.page.inner).(InnerWithTestState); ok {
		if state := s.TestState(); state != nil {
			return state
		}
	}
	return map[string]interface{}{}
}

// Close releases the inner model's resources, if it holds any.
func (m Meta[T]) Close() error {
	if c, ok := any(m.page.inner).(InnerWithClose); ok {
		return c.Close()
	}
	return nil
}

// wrapped adapts an Inner to Page.
type wrapped[T Inner] struct {
	inner T
	cfg   WrapConfig
}

func (p *wrapped[T]) Name() string  { return p.cfg.Name }
func (p *wrapped[T]) TabID() string { return p.cfg.TabID }
func (p *wrapped[T]) Init() tea.Cmd { return p.inner.Init() }

func (p *wrapped[T]) View() string {
	view := p.inner.View()
	if p.cfg.TrimLeadingNewline && len(view) > 0 && view[0] == '\n' {
		return view[1:]
	}
	return view
}

// step forwards a message and keeps the inner model if the update returned one
// of the same type. A bubbletea Update returns tea.Model, and a model that
// hands back something else — a replacement screen, say — is not something
// this adapter can hold, so the existing one is kept rather than dropped.
func (p *wrapped[T]) step(msg tea.Msg) tea.Cmd {
	updated, cmd := p.inner.Update(msg)
	if next, ok := updated.(T); ok {
		p.inner = next
	}
	return cmd
}

func (p *wrapped[T]) Update(msg tea.Msg) (Page, tea.Cmd) {
	return p, p.step(msg)
}

// Focus and Blur are where the inner model learns about a focus transition:
// the pager calls these and does not forward FocusMsg/BlurMsg to Update, so
// each step below is the inner model's one delivery. Blur drops the returned
// command because Page.Blur has nowhere to return one; a model with deferred
// work to do on blur should do it on the next focus instead.
func (p *wrapped[T]) Focus() tea.Cmd { return p.step(FocusMsg{}) }
func (p *wrapped[T]) Blur()          { p.step(BlurMsg{}) }

func (p *wrapped[T]) SetSize(w, h int) {
	p.step(tea.WindowSizeMsg{Width: w, Height: h})
}

// The optional Page extensions below are forwarded to the inner model when it
// implements the matching Inner interface. wrapped declares all of them
// unconditionally — a generic type cannot acquire a method only for some
// instantiations — so each answers, for a model that has not opted in, exactly
// what the pager assumes of a page that does not implement the extension at
// all: no text entry, no minimum, no title, always ready, always enabled.

func (p *wrapped[T]) IsTextEntryActive() bool {
	if t, ok := any(p.inner).(InnerWithTextEntry); ok {
		return t.IsTextEntryActive()
	}
	return false
}

func (p *wrapped[T]) MinSize() (width, height int) {
	if ms, ok := any(p.inner).(InnerWithMinSize); ok {
		return ms.MinSize()
	}
	return 0, 0
}

func (p *wrapped[T]) Title() string {
	if t, ok := any(p.inner).(InnerWithTitle); ok {
		return t.Title()
	}
	return ""
}

func (p *wrapped[T]) Ready() (ready bool, loadingMsg string) {
	if r, ok := any(p.inner).(InnerWithReady); ok {
		return r.Ready()
	}
	return true, ""
}

func (p *wrapped[T]) Enabled() bool {
	if e, ok := any(p.inner).(InnerWithEnabled); ok {
		return e.Enabled()
	}
	return true
}
