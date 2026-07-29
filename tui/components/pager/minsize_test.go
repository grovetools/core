package pager

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// minSizePage declares a minimum and draws a marker string, so a test can tell
// whether View() was called at all.
type minSizePage struct {
	stubPage
	minW, minH int
}

func (p *minSizePage) MinSize() (int, int) { return p.minW, p.minH }
func (p *minSizePage) View() string        { return "PAGE-BODY" }

// Override the promoted stubPage.Update: its receiver is the embedded
// *stubPage, so returning it would swap this page out of the pager for its
// own inner value and lose both MinSize and View.
func (p *minSizePage) Update(tea.Msg) (Page, tea.Cmd) { return p, nil }

func newSizedPager(t *testing.T, p Page, width, height int) Model {
	t.Helper()
	m := New([]Page{p}, DefaultKeyMap())
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return m
}

func TestPagerRendersThePageWhenItFits(t *testing.T) {
	p := &minSizePage{stubPage: stubPage{name: "wide"}, minW: 40, minH: 10}
	m := newSizedPager(t, p, 80, 24)

	if !strings.Contains(m.View(), "PAGE-BODY") {
		t.Errorf("pane meets the minimum but the body was not rendered:\n%s", m.View())
	}
}

func TestPagerReplacesATooNarrowPageWithThePlaceholder(t *testing.T) {
	p := &minSizePage{stubPage: stubPage{name: "wide"}, minW: 60}
	m := newSizedPager(t, p, 30, 24)

	view := m.View()
	if strings.Contains(view, "PAGE-BODY") {
		t.Errorf("a page below its minimum width was still rendered:\n%s", view)
	}
	if !strings.Contains(view, "too small") {
		t.Errorf("no placeholder in a pane below the minimum width:\n%s", view)
	}
}

func TestPagerReplacesATooShortPageWithThePlaceholder(t *testing.T) {
	// Tall minimum, generous width: the height check must be independent of
	// the width one.
	p := &minSizePage{stubPage: stubPage{name: "tall"}, minH: 40}
	m := newSizedPager(t, p, 120, 12)

	if strings.Contains(m.View(), "PAGE-BODY") {
		t.Errorf("a page below its minimum height was still rendered:\n%s", m.View())
	}
}

func TestAPageWithoutAMinimumIsNeverTooSmall(t *testing.T) {
	p := &stubPage{name: "flexible"}
	m := newSizedPager(t, p, 4, 3)

	if !strings.Contains(m.View(), "flexible") {
		t.Errorf("a page that never opted into a minimum was replaced:\n%s", m.View())
	}
}

// A pager that has not received its first WindowSizeMsg has zero dimensions.
// Treating that as "too small" would flash the placeholder on every first
// paint.
func TestAnUnsizedPagerDoesNotReportTooSmall(t *testing.T) {
	p := &minSizePage{stubPage: stubPage{name: "wide"}, minW: 60, minH: 20}
	m := New([]Page{p}, DefaultKeyMap())

	if strings.Contains(m.View(), "too small") {
		t.Errorf("an unsized pager rendered the too-small placeholder:\n%s", m.View())
	}
}

func TestTooSmallPlaceholderDegradesWithWidth(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		expect string
	}{
		{"a wide pane gets the actionable sentence", 40, "widen or hide a split"},
		{"a medium pane gets the short form", 20, "Pane too small"},
		{"a very narrow pane gets a glyph rather than nothing", 6, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TooSmallPlaceholder(tt.width, 5)
			if got == "" {
				t.Fatal("placeholder is empty; a blank pane reads as a crash")
			}
			if tt.expect != "" && !strings.Contains(got, tt.expect) {
				t.Errorf("placeholder = %q, want it to contain %q", got, tt.expect)
			}
		})
	}
}

func TestTooSmallPlaceholderWithNoRoomAtAll(t *testing.T) {
	if got := TooSmallPlaceholder(0, 0); got != "" {
		t.Errorf("TooSmallPlaceholder(0, 0) = %q, want empty", got)
	}
}
