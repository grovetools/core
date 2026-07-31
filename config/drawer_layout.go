package config

import "strings"

// DrawerPageRefPrefix marks a layout leaf as a reference to another PAGE rather
// than to a pane: `pane = "page:sessions"` mounts the sessions page's whole
// layout at that position, which is how a drawer wide enough for two subjects
// shows two pages side by side without either of them becoming a special kind
// of page.
//
// It is spelled as a prefix on the existing `pane` key rather than as a key of
// its own because a reference IS a leaf everywhere it matters — it sits where a
// pane sits, it is a child of a split, it takes a min_width — and one field with
// one grammar keeps every host walker on one shape.
const DrawerPageRefPrefix = "page:"

// DrawerPageRef splits a leaf's pane value into the page it references and
// whether it references one at all.
//
// It lives here, beside the schema description that documents the grammar to
// users, so the two cannot drift. A host that does not compose can ignore the
// second return entirely: a reference then reads as a pane name no registry
// has, which is already a leaf that normalizes away.
func DrawerPageRef(pane string) (string, bool) {
	name, ok := strings.CutPrefix(pane, DrawerPageRefPrefix)
	return name, ok && name != ""
}

// The built-in per-leaf minimums a host applies to a drawer node that declares
// none of its own. They are the point below which a pane stops being readable
// rather than merely cramped: under ~24 columns a session row, a note title or
// a diff path truncates to a prefix that identifies nothing, and under 4 rows a
// pane has no room left for content once its heading is drawn.
//
// They live here, beside the [DrawerNodeConfig.MinWidth] field they are the
// default for, so the schema documentation and the host that enforces them
// cannot drift onto different numbers.
const (
	DrawerMinWidthDefault  = 24
	DrawerMinHeightDefault = 4
)

// The bounds a drawer split ratio is held to. A child of a split always gets at
// least a tenth of the budget: a ratio outside this range is not a layout, it
// is a pane that was going to be hidden, and hiding a pane is what `delete` and
// availability are for.
const (
	drawerRatioDefault = .5
	drawerRatioMin     = .1
	drawerRatioMax     = .9
)

// EffectiveRatio is the fraction of a split's budget the FIRST child gets: the
// configured value clamped to the accepted range, with the unset value reading
// as an even split.
//
// It is shared rather than open-coded because two callers must agree on it
// exactly — the host that compiles the layout, and the lint that predicts
// whether that compile can satisfy the children's minimums. A lint that
// clamped differently from the compiler would warn about layouts that work and
// stay silent about ones that do not.
func (n *DrawerNodeConfig) EffectiveRatio() float64 {
	if n == nil {
		return drawerRatioDefault
	}
	r := n.Ratio
	if r == 0 {
		return drawerRatioDefault
	}
	if r < drawerRatioMin {
		return drawerRatioMin
	}
	if r > drawerRatioMax {
		return drawerRatioMax
	}
	return r
}
