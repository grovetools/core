// Package table fits a set of columns into a pane too narrow to hold them all.
//
// It is the promotion of flow's job-table fitting pass, which was the only
// good answer to the problem in the ecosystem and had been copied verbatim
// into flow's plan browser while seven other table call sites had no fitting
// at all and simply overflowed their pane.
//
// The three ideas worth keeping from that implementation:
//
//   - Measure from the cells the renderer actually emits, never from an
//     estimate. The estimator this replaced sized columns by their header text,
//     which is how a 25-column TOKENS cell read as 6 and the table overflowed
//     anyway.
//   - Drop whole columns rather than squeezing all of them. A table of
//     truncated cells is unreadable; a table of fewer, complete columns is not.
//   - The identity column is never dropped, only truncated as a last resort. It
//     carries the row's name, and a table you cannot match to a row is not a
//     table.
package table

import "github.com/charmbracelet/lipgloss"

// Column is one column's contribution to a fitting decision.
type Column struct {
	// Name is the header text, and the key the measured widths are held under.
	Name string

	// Priority orders the drop pass: the lowest-priority column goes first,
	// ties broken by declaration order.
	//
	// The zero value means "drop me first", and that default is the point. A
	// column added to a table without being ranked is expendable until someone
	// says otherwise — the alternative, where an unranked column is implicitly
	// undroppable, is how flow's TOKENS column (the second widest in the
	// table) survived every drop pass and left the table overflowing no matter
	// how many other columns went.
	Priority int

	// Identity marks the column carrying the row's identity — its name, path
	// or slug. An identity column is never dropped; when everything droppable
	// is gone and the table still overflows, its cells are truncated instead.
	// A table with at most one identity column is the normal case; if several
	// are marked, the first is the one truncated.
	Identity bool

	// Pinned marks a column that is never dropped and never truncated: a
	// selection checkbox, a status glyph, anything one or two columns wide
	// whose absence changes what the table means rather than how much of it
	// you can see. Unlike Identity it is not a truncation target — pinning is
	// what a fixed-width column needs, and it costs so little width that
	// dropping it can never be the thing that makes a table fit.
	Pinned bool

	// MinWidth floors the truncation of an identity column. Below some width a
	// filename identifies nothing and clipping would be no worse, so the fit
	// stops there and lets the renderer clip. Unset means a floor of one
	// column.
	MinWidth int
}

// Rendered binds a Column to the function that draws its cells, for callers
// that let Measure do the measuring.
//
// It is a separate type from Column because measuring per column is the wrong
// shape for some renderers: a row whose cells share expensive computation —
// one status lookup feeding four columns — wants to render the whole row once.
// Those callers keep their row-oriented renderer, measure themselves, and call
// Fit with the widths. Everyone else uses Rendered and FitRows.
type Rendered[T any] struct {
	Column
	// Render draws this column's cell for one row. It returns the final
	// styled string, because that is what has to be measured: measuring
	// anything other than what is drawn is the bug this package exists to
	// stop.
	Render func(row T) string
}

// Chrome is the fixed width a table renderer spends on everything that is not
// cell content. It is what makes the fit decision agree with the renderer, so
// each renderer declares its own.
type Chrome struct {
	// CellPadding is the columns of padding on each side of every cell.
	CellPadding int
	// Separator is the columns between two adjacent cells.
	Separator int
	// Border is the total columns of border, both edges together.
	Border int
	// Gutter is the fixed columns the renderer prepends to every row, such as
	// a selection indicator.
	Gutter int
}

// SelectableChrome is the chrome of core/tui/components/table's
// SelectableTableWithOptions: a space of padding either side of each cell, a
// │ between them, a rounded border, and the two-column selection gutter.
var SelectableChrome = Chrome{CellPadding: 1, Separator: 1, Border: 2, Gutter: 2}

// Width returns the width a table with these columns renders to, given each
// one's measured cell width. Columns missing from widths count as zero.
func (c Chrome) Width(names []string, widths map[string]int) int {
	if len(names) == 0 {
		return 0
	}
	total := 0
	for _, name := range names {
		total += widths[name]
	}
	total += len(names) * 2 * c.CellPadding
	total += (len(names) - 1) * c.Separator
	total += c.Border
	total += c.Gutter
	return total
}

// Layout is the fitting decision: which columns survive, how wide each is, and
// what the table renders to.
type Layout struct {
	// Columns are the surviving columns, in the order they were given.
	Columns []Column
	// Dropped are the columns that were removed to fit, in the order they went.
	Dropped []string
	// Widths is the measured width of every column that was considered,
	// dropped ones included. It is the input measurement, unchanged.
	Widths map[string]int
	// Width is what the surviving columns render to, chrome included. It is
	// not guaranteed to be within the budget: when the identity column alone
	// overflows, this reports the pre-truncation width and IdentityCap says
	// how much has to come off.
	Width int
	// IdentityCap is the width the identity column's cells must be truncated
	// to, or 0 when no truncation is needed.
	IdentityCap int
}

// Names returns the surviving column names, in order.
func (l Layout) Names() []string {
	names := make([]string, len(l.Columns))
	for i, c := range l.Columns {
		names[i] = c.Name
	}
	return names
}

// Visible reports whether a column survived the fit.
func (l Layout) Visible(name string) bool {
	for _, c := range l.Columns {
		if c.Name == name {
			return true
		}
	}
	return false
}

// Measure returns each column's rendered width: the wider of its header text
// and the widest cell the given rows put under it.
//
// Pass the rows that are actually on screen, not the whole list. A column is
// sized for what the viewport shows, so scrolling to a row with a longer value
// re-fits the table rather than permanently reserving width for a row nobody
// is looking at.
func Measure[T any](cols []Rendered[T], rows []T) map[string]int {
	widths := make(map[string]int, len(cols))
	for _, c := range cols {
		widths[c.Name] = lipgloss.Width(c.Name)
	}
	for _, row := range rows {
		for _, c := range cols {
			if c.Render == nil {
				continue
			}
			if w := lipgloss.Width(c.Render(row)); w > widths[c.Name] {
				widths[c.Name] = w
			}
		}
	}
	return widths
}

// FitRows measures the rows and fits the columns into maxWidth in one call.
func FitRows[T any](cols []Rendered[T], rows []T, maxWidth int, chrome Chrome) Layout {
	plain := make([]Column, len(cols))
	for i, c := range cols {
		plain[i] = c.Column
	}
	return Fit(plain, Measure(cols, rows), maxWidth, chrome)
}

// Fit drops the lowest-priority columns until the table renders within
// maxWidth, and reports what survived.
//
// A maxWidth of zero or less means "unconstrained" and everything survives —
// a pane that has not been sized yet must not be read as a pane with no room.
//
// The pass is: check whether everything already fits; if not, drop columns in
// priority order, re-checking after each one, and stop at the first set that
// fits; if every droppable column is gone and the identity column alone still
// overflows, report the width its cells must be truncated to.
//
// Widths are measured once, before any dropping, because a column's own width
// does not depend on which other columns are visible.
func Fit(cols []Column, widths map[string]int, maxWidth int, chrome Chrome) Layout {
	layout := Layout{Columns: cols, Widths: widths}
	if len(cols) == 0 || maxWidth <= 0 {
		layout.Width = chrome.Width(layout.Names(), widths)
		return layout
	}

	layout.Width = chrome.Width(layout.Names(), widths)
	if layout.Width <= maxWidth {
		return layout // Everything fits.
	}

	for _, name := range DropOrder(cols) {
		layout.Columns = without(layout.Columns, name)
		layout.Dropped = append(layout.Dropped, name)
		layout.Width = chrome.Width(layout.Names(), widths)
		if layout.Width <= maxWidth {
			return layout
		}
	}

	// Every droppable column is gone and the identity column alone still
	// overflows — long names in a narrow pane. Truncating its cells beats a
	// table whose right-hand border falls off the pane.
	for _, c := range layout.Columns {
		if !c.Identity {
			continue
		}
		floor := c.MinWidth
		if floor < 1 {
			floor = 1
		}
		cap := widths[c.Name] - (layout.Width - maxWidth)
		if cap < floor {
			cap = floor
		}
		layout.IdentityCap = cap
		break
	}
	return layout
}

// DropOrder returns the droppable column names, most expendable first:
// ascending priority, ties in declaration order. Identity and pinned columns
// are absent — an identity column is truncated rather than dropped, and a
// pinned one is neither.
func DropOrder(cols []Column) []string {
	type ranked struct {
		name     string
		priority int
		index    int
	}
	var candidates []ranked
	for i, c := range cols {
		if c.Identity || c.Pinned {
			continue
		}
		candidates = append(candidates, ranked{c.Name, c.Priority, i})
	}
	// Insertion sort: these lists are a dozen entries at most, and it keeps
	// ties in declaration order without pulling in a comparator.
	for i := 1; i < len(candidates); i++ {
		for j := i; j > 0; j-- {
			if candidates[j-1].priority <= candidates[j].priority {
				break
			}
			candidates[j-1], candidates[j] = candidates[j], candidates[j-1]
		}
	}
	names := make([]string, len(candidates))
	for i, c := range candidates {
		names[i] = c.name
	}
	return names
}

func without(cols []Column, name string) []Column {
	out := make([]Column, 0, len(cols))
	for _, c := range cols {
		if c.Name != name {
			out = append(out, c)
		}
	}
	return out
}
