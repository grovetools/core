// Package tree holds the two pieces every tree view in the ecosystem builds
// itself out of: the drawing prefixes for a flat, parent-first node list, and
// the fold state that decides which of those nodes are in the list at all.
//
// Deliberately not here: building the tree. Five independent tree
// implementations across the ecosystem disagreed about almost everything, but
// the two that read best — nb's browser and flow's job table — agreed on the
// shape: flatten to a parent-first list carrying a depth, keep folds in a map
// keyed by a stable ID, and render prefixes from the depths. Construction is
// per-app and stays per-app; what is shared is the rendering and the folding.
//
// The flat-list shape is what makes this work. A recursive tree with fold flags
// stored inside the nodes has to be walked to answer "what is on screen", loses
// its fold state whenever the data is rebuilt, and cannot be windowed without a
// second flattening pass. A flat list of depths plus a map of IDs has none of
// those problems: it slices directly into a viewport, survives a rebuild, and
// renders with one pass over integers.
package tree

// Glyphs are the box-drawing characters a tree renders with. All four must be
// the same display width or the tree will not line up.
type Glyphs struct {
	// Branch precedes a node that has siblings after it.
	Branch string
	// LastBranch precedes the final node of a sibling group.
	LastBranch string
	// Vertical is the ancestor line drawn under a Branch, continuing the run
	// down past this node's descendants.
	Vertical string
	// Space is what an ancestor line becomes under a LastBranch: nothing more
	// comes at that level, so the line stops.
	Space string
}

// DefaultGlyphs is the three-column style: ├─ / └─ with a continuing │ line.
var DefaultGlyphs = Glyphs{
	Branch:     "├─ ",
	LastBranch: "└─ ",
	Vertical:   "│  ",
	Space:      "   ",
}

// Prefixes returns one drawing prefix per entry for a parent-first list of
// depths, using DefaultGlyphs and no outer indent.
//
// Parent-first is the requirement: an entry's children are the entries that
// follow it at a greater depth, up to the next entry at its own depth or
// shallower. That is what a depth-first flatten produces, and it is the only
// input shape from which last-sibling can be decided in one pass.
func Prefixes(depths []int) []string {
	return PrefixesWith(depths, "", DefaultGlyphs)
}

// PrefixesWith is Prefixes with an outer indent prepended to every prefix and
// a caller-chosen glyph set — for a tree drawn inside something else, such as
// a per-session forest nested under a workspace row.
func PrefixesWith(depths []int, indent string, g Glyphs) []string {
	prefixes := make([]string, len(depths))

	// lastAtDepth[d] records whether the most recent entry at depth d was the
	// last of its siblings. Walking parent-first, the entry at depth d-1 that
	// most recently went past is this entry's parent, so this map is the
	// ancestor chain — and whether each ancestor was last is exactly what
	// decides between a continuing line and a blank.
	lastAtDepth := make(map[int]bool, 8)

	for i, depth := range depths {
		if depth < 0 {
			depth = 0
		}

		last := isLastSibling(depths, i, depth)

		prefix := indent
		for level := 0; level < depth; level++ {
			if lastAtDepth[level] {
				prefix += g.Space
			} else {
				prefix += g.Vertical
			}
		}
		if last {
			prefix += g.LastBranch
		} else {
			prefix += g.Branch
		}
		prefixes[i] = prefix

		lastAtDepth[depth] = last
		// Anything deeper belonged to a subtree that just ended. Well-formed
		// input never reads a stale deeper entry, but a list that skips a
		// level would, and a tree that draws a line from an unrelated subtree
		// is worse than one that draws none.
		for d := range lastAtDepth {
			if d > depth {
				delete(lastAtDepth, d)
			}
		}
	}
	return prefixes
}

// isLastSibling reports whether the entry at i is the final child of its
// parent: no later entry sits at the same depth before the walk pops back
// above it.
func isLastSibling(depths []int, i, depth int) bool {
	for j := i + 1; j < len(depths); j++ {
		d := depths[j]
		if d < 0 {
			d = 0
		}
		if d < depth {
			return true // popped above this level
		}
		if d == depth {
			return false // another sibling
		}
	}
	return true
}

// Node is the minimum a fold-aware tree needs from a caller's row type.
//
// It is three methods rather than a struct because every app already has a row
// type carrying its own payload, and making them wrap it in a kit struct would
// buy nothing. ID has to be stable across a rebuild — a path, a job ID, a slug
// — because that is what folds are keyed on; an index would collapse the wrong
// node the moment the data changed underneath.
type Node interface {
	// ID identifies this node stably across rebuilds.
	ID() string
	// Depth is the node's level, 0 for a root.
	Depth() int
	// Foldable reports whether this node can hold children. A leaf answers
	// false and is never folded.
	Foldable() bool
}

// FoldState is which nodes are collapsed: the user's explicit choices, over a
// default the app decides.
//
// Splitting the two is what makes "collapsed unless it is running" or
// "collapsed unless it needs attention" expressible without writing a fold
// entry for every node on every rebuild. An override is recorded only when the
// user actually folds something, so a node the user has not touched keeps
// following the policy as its state changes underneath.
//
// The zero value is usable: no overrides, and a default of expanded.
type FoldState struct {
	// Default reports whether a node with no explicit override is collapsed.
	// Nil means expanded.
	Default func(id string) bool

	overrides map[string]bool
}

// NewFoldState returns a FoldState with the given default policy.
func NewFoldState(def func(id string) bool) *FoldState {
	return &FoldState{Default: def}
}

// Collapsed reports whether a node is currently collapsed: its explicit
// override when it has one, otherwise the default policy.
func (f *FoldState) Collapsed(id string) bool {
	if f == nil {
		return false
	}
	if v, ok := f.overrides[id]; ok {
		return v
	}
	if f.Default != nil {
		return f.Default(id)
	}
	return false
}

// Set records an explicit choice for a node, overriding the default policy
// from here on.
func (f *FoldState) Set(id string, collapsed bool) {
	if f.overrides == nil {
		f.overrides = make(map[string]bool)
	}
	f.overrides[id] = collapsed
}

// Toggle flips a node's state, recording the result as an explicit choice.
// A node following the default is toggled relative to what the default says,
// so the first toggle always does what the user is looking at.
func (f *FoldState) Toggle(id string) {
	f.Set(id, !f.Collapsed(id))
}

// Clear drops a node's explicit choice, returning it to the default policy.
func (f *FoldState) Clear(id string) {
	delete(f.overrides, id)
}

// Reset drops every explicit choice, returning the whole tree to the default
// policy. This is what "collapse all"/"expand all" should call before setting
// its own overrides, so stale choices do not survive the reset.
func (f *FoldState) Reset() {
	f.overrides = nil
}

// Overrides returns a copy of the explicit choices, for persisting them.
func (f *FoldState) Overrides() map[string]bool {
	if len(f.overrides) == 0 {
		return nil
	}
	out := make(map[string]bool, len(f.overrides))
	for k, v := range f.overrides {
		out[k] = v
	}
	return out
}

// SetOverrides replaces the explicit choices wholesale, for restoring them.
func (f *FoldState) SetOverrides(overrides map[string]bool) {
	if len(overrides) == 0 {
		f.overrides = nil
		return
	}
	f.overrides = make(map[string]bool, len(overrides))
	for k, v := range overrides {
		f.overrides[k] = v
	}
}

// Visible filters a parent-first node list down to what is on screen: every
// node whose ancestors are all expanded. A collapsed node is itself visible —
// it has to be, or there would be nothing to expand — but its descendants are
// not.
func Visible[T Node](nodes []T, folds *FoldState) []T {
	out := make([]T, 0, len(nodes))
	// hiddenBelow is the depth under which everything is folded away, or -1
	// when nothing is. A node at or above it ends the hidden run.
	hiddenBelow := -1
	for _, n := range nodes {
		if hiddenBelow >= 0 {
			if n.Depth() > hiddenBelow {
				continue
			}
			hiddenBelow = -1
		}
		out = append(out, n)
		if n.Foldable() && folds.Collapsed(n.ID()) {
			hiddenBelow = n.Depth()
		}
	}
	return out
}

// Depths returns the depth of each node, for handing to Prefixes.
func Depths[T Node](nodes []T) []int {
	depths := make([]int, len(nodes))
	for i, n := range nodes {
		depths[i] = n.Depth()
	}
	return depths
}
