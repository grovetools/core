package table

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// A five-column table modelled on flow's: an undroppable identity column, then
// four rankable ones. Priorities ascend, so TITLE is the first to go.
func sample() []Column {
	return []Column{
		{Name: "JOB", Identity: true, MinWidth: 12},
		{Name: "TITLE", Priority: 1},
		{Name: "DURATION", Priority: 2},
		{Name: "TOKENS", Priority: 3},
		{Name: "STATUS", Priority: 4},
	}
}

func sampleWidths() map[string]int {
	return map[string]int{"JOB": 20, "TITLE": 30, "DURATION": 8, "TOKENS": 10, "STATUS": 6}
}

func TestChromeWidthMatchesTheSelectableRenderer(t *testing.T) {
	// Three columns of 5, 4 and 3: cells 12, padding 6, separators 2,
	// border 2, gutter 2 = 24.
	got := SelectableChrome.Width(
		[]string{"a", "b", "c"},
		map[string]int{"a": 5, "b": 4, "c": 3},
	)
	if got != 24 {
		t.Errorf("Width() = %d, want 24", got)
	}
	if got := SelectableChrome.Width(nil, nil); got != 0 {
		t.Errorf("Width() with no columns = %d, want 0", got)
	}
}

func TestFitEverythingFits(t *testing.T) {
	l := Fit(sample(), sampleWidths(), 500, SelectableChrome)
	if len(l.Columns) != 5 {
		t.Errorf("dropped %v from a table that fits", l.Dropped)
	}
	if l.IdentityCap != 0 {
		t.Errorf("IdentityCap = %d, want 0 when nothing had to be truncated", l.IdentityCap)
	}
}

func TestFitDropsLowestPriorityFirst(t *testing.T) {
	widths := sampleWidths()
	// Full table is 74 cells + 10 padding + 4 separators + 2 border + 2
	// gutter = 92. A budget of 70 needs TITLE gone, and only TITLE (59).
	l := Fit(sample(), widths, 70, SelectableChrome)

	if want := []string{"TITLE"}; !equal(l.Dropped, want) {
		t.Errorf("Dropped = %v, want %v", l.Dropped, want)
	}
	if l.Width > 70 {
		t.Errorf("Width = %d, over the 70 budget", l.Width)
	}
	if !l.Visible("DURATION") {
		t.Error("dropped DURATION when dropping TITLE alone was enough")
	}
}

func TestFitDropsInPriorityOrderUntilItFits(t *testing.T) {
	// 92 -> 59 without TITLE -> 48 without DURATION too, which fits 50.
	l := Fit(sample(), sampleWidths(), 50, SelectableChrome)

	if want := []string{"TITLE", "DURATION"}; !equal(l.Dropped, want) {
		t.Errorf("Dropped = %v, want %v", l.Dropped, want)
	}
	if l.Width > 50 {
		t.Errorf("Width = %d, over the 50 budget", l.Width)
	}
}

// An unranked column carries Priority 0, so it is the first out. This is the
// property that stops a newly added column from silently becoming
// undroppable and leaving the table overflowing no matter what else goes.
func TestFitDropsUnrankedColumnsFirst(t *testing.T) {
	cols := append(sample(), Column{Name: "NEW"})
	widths := sampleWidths()
	widths["NEW"] = 40

	l := Fit(cols, widths, 90, SelectableChrome)

	if len(l.Dropped) == 0 || l.Dropped[0] != "NEW" {
		t.Errorf("Dropped = %v, want NEW first", l.Dropped)
	}
}

func TestFitNeverDropsTheIdentityColumn(t *testing.T) {
	l := Fit(sample(), sampleWidths(), 1, SelectableChrome)

	if !l.Visible("JOB") {
		t.Fatal("dropped the identity column")
	}
	if len(l.Columns) != 1 {
		t.Errorf("survivors = %v, want the identity column alone", l.Names())
	}
	for _, name := range l.Dropped {
		if name == "JOB" {
			t.Error("identity column appears in Dropped")
		}
	}
}

func TestFitTruncatesTheIdentityColumnAsALastResort(t *testing.T) {
	// JOB alone renders to 20 + 2 padding + 2 border + 2 gutter = 26.
	// A budget of 20 is 6 over, so the cells cap at 20-6 = 14.
	l := Fit(sample(), sampleWidths(), 20, SelectableChrome)

	if l.IdentityCap != 14 {
		t.Errorf("IdentityCap = %d, want 14", l.IdentityCap)
	}
}

func TestFitFloorsTheTruncationAtMinWidth(t *testing.T) {
	// Far too narrow: the arithmetic would ask for a negative cap, and
	// MinWidth is what stops it. Below that a name identifies nothing and
	// letting the renderer clip is no worse.
	l := Fit(sample(), sampleWidths(), 2, SelectableChrome)

	if l.IdentityCap != 12 {
		t.Errorf("IdentityCap = %d, want the MinWidth floor of 12", l.IdentityCap)
	}
}

func TestFitFloorsAtOneColumnWithoutAMinWidth(t *testing.T) {
	cols := []Column{{Name: "JOB", Identity: true}}
	l := Fit(cols, map[string]int{"JOB": 20}, 2, SelectableChrome)

	if l.IdentityCap != 1 {
		t.Errorf("IdentityCap = %d, want the default floor of 1", l.IdentityCap)
	}
}

func TestFitTreatsAnUnsizedPaneAsUnconstrained(t *testing.T) {
	for _, maxWidth := range []int{0, -1, -100} {
		l := Fit(sample(), sampleWidths(), maxWidth, SelectableChrome)
		if len(l.Columns) != 5 || l.IdentityCap != 0 {
			t.Errorf("maxWidth %d dropped %v / capped at %d; a pane that has not been sized "+
				"is not a pane with no room", maxWidth, l.Dropped, l.IdentityCap)
		}
	}
}

func TestFitWithNoColumns(t *testing.T) {
	l := Fit(nil, nil, 80, SelectableChrome)
	if len(l.Columns) != 0 || l.Width != 0 || l.IdentityCap != 0 {
		t.Errorf("Fit(nil) = %+v, want an empty layout", l)
	}
}

func TestFitWithNoIdentityColumnDropsEverythingDroppable(t *testing.T) {
	cols := []Column{{Name: "A", Priority: 1}, {Name: "B", Priority: 2}}
	l := Fit(cols, map[string]int{"A": 10, "B": 10}, 1, SelectableChrome)

	if len(l.Columns) != 0 {
		t.Errorf("survivors = %v, want none", l.Names())
	}
	if l.IdentityCap != 0 {
		t.Errorf("IdentityCap = %d, want 0 with no identity column to truncate", l.IdentityCap)
	}
}

type row struct {
	name     string
	duration string
}

func TestMeasureUsesTheWiderOfHeaderAndCells(t *testing.T) {
	cols := []Rendered[row]{
		{Column: Column{Name: "JOB", Identity: true}, Render: func(r row) string { return r.name }},
		{Column: Column{Name: "DURATION", Priority: 1}, Render: func(r row) string { return r.duration }},
	}
	rows := []row{{"short", "1h"}, {"a-much-longer-job-name", "2m"}}

	widths := Measure(cols, rows)

	if got, want := widths["JOB"], len("a-much-longer-job-name"); got != want {
		t.Errorf("JOB width = %d, want %d (the widest cell)", got, want)
	}
	if got, want := widths["DURATION"], len("DURATION"); got != want {
		t.Errorf("DURATION width = %d, want %d (the header, wider than any cell)", got, want)
	}
}

// The invariant the estimator this replaces got wrong: a styled cell is
// measured by what it displays, not by how many bytes of escape sequence it
// carries.
func TestMeasureIgnoresStyling(t *testing.T) {
	bold := lipgloss.NewStyle().Bold(true)
	cols := []Rendered[row]{
		{Column: Column{Name: "JOB"}, Render: func(r row) string { return bold.Render(r.name) }},
	}

	widths := Measure(cols, []row{{name: "abcdefgh"}})

	if got := widths["JOB"]; got != 8 {
		t.Errorf("JOB width = %d, want 8; styling must not count toward the measure", got)
	}
}

func TestMeasureWithNoRowsFallsBackToHeaders(t *testing.T) {
	cols := []Rendered[row]{{Column: Column{Name: "DURATION"}, Render: func(r row) string { return r.duration }}}
	if got := Measure(cols, nil)["DURATION"]; got != 8 {
		t.Errorf("DURATION width = %d, want the header width 8", got)
	}
}

func TestFitRows(t *testing.T) {
	cols := []Rendered[row]{
		{Column: Column{Name: "JOB", Identity: true, MinWidth: 4}, Render: func(r row) string { return r.name }},
		{Column: Column{Name: "DURATION", Priority: 1}, Render: func(r row) string { return r.duration }},
	}
	rows := []row{{"a-job", "1h"}}

	// JOB(5) + DURATION(8) = 13 cells, 4 padding, 1 separator, 2 border,
	// 2 gutter = 22. A budget of 15 drops DURATION down to 11.
	l := FitRows(cols, rows, 15, SelectableChrome)

	if want := []string{"DURATION"}; !equal(l.Dropped, want) {
		t.Errorf("Dropped = %v, want %v", l.Dropped, want)
	}
	if l.Width != 11 {
		t.Errorf("Width = %d, want 11", l.Width)
	}
}

// The measured widths are reported for every column considered, dropped ones
// included, so a caller can render a "3 columns hidden" hint without
// re-measuring.
func TestLayoutKeepsWidthsForDroppedColumns(t *testing.T) {
	l := Fit(sample(), sampleWidths(), 45, SelectableChrome)
	if l.Widths["TITLE"] != 30 {
		t.Errorf("Widths[TITLE] = %d after dropping it, want 30", l.Widths["TITLE"])
	}
}

func TestFitDoesNotMutateTheInput(t *testing.T) {
	cols := sample()
	before := strings.Join(namesOf(cols), ",")

	Fit(cols, sampleWidths(), 20, SelectableChrome)

	if after := strings.Join(namesOf(cols), ","); after != before {
		t.Errorf("Fit mutated its columns: %q -> %q", before, after)
	}
}

func namesOf(cols []Column) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = c.Name
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestFitNeverDropsAPinnedColumn(t *testing.T) {
	// A pinned column with the most expendable priority there is: it must
	// still outlive every ranked column, and survive alongside the identity
	// column when nothing else does.
	cols := append([]Column{{Name: "SEL", Pinned: true}}, sample()...)
	widths := sampleWidths()
	widths["SEL"] = 2

	l := Fit(cols, widths, 1, SelectableChrome)

	if !l.Visible("SEL") {
		t.Errorf("dropped the pinned column; survivors = %v", l.Names())
	}
	if !l.Visible("JOB") {
		t.Errorf("dropped the identity column; survivors = %v", l.Names())
	}
	if len(l.Columns) != 2 {
		t.Errorf("survivors = %v, want the pinned and identity columns only", l.Names())
	}
}

func TestPinnedColumnIsNotATruncationTarget(t *testing.T) {
	// Identity, not Pinned, is what gets truncated — even when the pinned
	// column comes first in display order.
	cols := []Column{
		{Name: "SEL", Pinned: true},
		{Name: "JOB", Identity: true, MinWidth: 4},
	}
	l := Fit(cols, map[string]int{"SEL": 2, "JOB": 20}, 10, SelectableChrome)

	// SEL(2) + JOB(20) = 22 cells, 4 padding, 1 separator, 2 border, 2
	// gutter = 31. Over a budget of 10 by 21, so JOB caps at 20-21 -> the
	// MinWidth floor.
	if l.IdentityCap != 4 {
		t.Errorf("IdentityCap = %d, want the JOB floor of 4", l.IdentityCap)
	}
}
