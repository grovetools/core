package pager

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// stubPage is a Page that records the last size it was handed.
type stubPage struct {
	name string
	w, h int
}

func (p *stubPage) Name() string                   { return p.name }
func (p *stubPage) Init() tea.Cmd                  { return nil }
func (p *stubPage) Update(tea.Msg) (Page, tea.Cmd) { return p, nil }
func (p *stubPage) View() string                   { return p.name }
func (p *stubPage) Focus() tea.Cmd                 { return nil }
func (p *stubPage) Blur()                          {}
func (p *stubPage) SetSize(width, height int)      { p.w, p.h = width, height }

// noFooterPage renders its own footer inside its body and so wants the
// pager's shared footer reservation handed back.
type noFooterPage struct{ stubPage }

func (p *noFooterPage) FooterHeight() int { return 0 }

var (
	_ Page                 = (*stubPage)(nil)
	_ PageWithFooterHeight = (*noFooterPage)(nil)
)

// TestFooterHeightPerPage covers the core of the fix: Config.FooterHeight is a
// single reservation shared by every tab, so a page that renders no footer in
// that slot used to lose the rows to blank space at the bottom of its pane.
func TestFooterHeightPerPage(t *testing.T) {
	shared := &stubPage{name: "shared"}
	own := &noFooterPage{stubPage{name: "own"}}

	m := NewWith([]Page{shared, own}, DefaultKeyMap(), Config{
		OuterPadding: [4]int{1, 2, 0, 2},
		ShowTitleRow: true,
		FooterHeight: 4,
	})

	// chrome = outer top 1 + tab bar 2 + title 1 = 4, plus the footer slot.
	if got, want := m.ChromeRowsFor(shared), 8; got != want {
		t.Errorf("ChromeRowsFor(shared) = %d, want %d", got, want)
	}
	if got, want := m.ChromeRowsFor(own), 4; got != want {
		t.Errorf("ChromeRowsFor(own) = %d, want %d (its 4 footer rows returned to the body)", got, want)
	}

	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	if shared.h != 32 {
		t.Errorf("shared page height = %d, want 32", shared.h)
	}
	if own.h != 36 {
		t.Errorf("page opting out of the footer slot got height = %d, want 36", own.h)
	}
	if shared.w != 96 || own.w != 96 {
		t.Errorf("widths = (%d, %d), want (96, 96)", shared.w, own.w)
	}
}

// TestFooterHeightDefaultsToConfig keeps the opt-in nature explicit: a page
// that doesn't implement PageWithFooterHeight is sized exactly as before.
func TestFooterHeightDefaultsToConfig(t *testing.T) {
	p := &stubPage{name: "plain"}
	m := NewWith([]Page{p}, DefaultKeyMap(), Config{FooterHeight: 3})

	// tab bar 2 + footer 3.
	if got, want := m.ChromeRows(), 5; got != want {
		t.Errorf("ChromeRows() = %d, want %d", got, want)
	}
	if got := m.SubSize(80, 24); got.Height != 19 {
		t.Errorf("SubSize height = %d, want 19", got.Height)
	}
}
