package window

import "testing"

func TestClamp(t *testing.T) {
	tests := []struct {
		name string
		in   Window
		want Window
	}{
		{"zero value is already coherent", Window{}, Window{}},
		{
			"cursor past the end lands on the last row",
			Window{Cursor: 99, Height: 10, Total: 5},
			Window{Cursor: 4, Height: 10, Total: 5},
		},
		{
			"negative cursor lands on the first row",
			Window{Cursor: -3, Height: 10, Total: 5},
			Window{Cursor: 0, Height: 10, Total: 5},
		},
		{
			"an empty list pins the cursor to zero rather than -1",
			Window{Cursor: 7, Height: 10, Total: 0},
			Window{Cursor: 0, Height: 10, Total: 0},
		},
		{
			"negative height and total are floored at zero",
			Window{Cursor: 2, Offset: 3, Height: -5, Total: -9},
			Window{},
		},
		{
			"offset may not sit past the last full page",
			Window{Cursor: 5, Offset: 40, Height: 10, Total: 30},
			Window{Cursor: 5, Offset: 20, Height: 10, Total: 30},
		},
		{
			"a list shorter than the viewport pins the offset to the top",
			Window{Cursor: 1, Offset: 6, Height: 20, Total: 3},
			Window{Cursor: 1, Offset: 0, Height: 20, Total: 3},
		},
		{
			"negative offset is floored at zero",
			Window{Cursor: 1, Offset: -4, Height: 10, Total: 30},
			Window{Cursor: 1, Offset: 0, Height: 10, Total: 30},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.Clamp(); got != tt.want {
				t.Errorf("Clamp() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRange(t *testing.T) {
	tests := []struct {
		name             string
		in               Window
		wantStart, wantE int
	}{
		{"zero value spans nothing", Window{}, 0, 0},
		{"a full viewport spans exactly Height rows", Window{Offset: 0, Height: 10, Total: 30}, 0, 10},
		{"a scrolled viewport starts at the offset", Window{Offset: 5, Height: 10, Total: 30}, 5, 15},
		{"the last page is short", Window{Offset: 25, Height: 10, Total: 30}, 25, 30},
		{"a list shorter than the viewport spans the whole list", Window{Height: 20, Total: 3}, 0, 3},
		{"a negative offset starts at the top", Window{Offset: -5, Height: 10, Total: 30}, 0, 10},
		{"zero height spans nothing", Window{Offset: 4, Height: 0, Total: 30}, 4, 4},

		// The panic this widget exists to retire: a filter shrinks the list
		// under an offset that is still scrolled down. Every copy of this
		// arithmetic that computed end from the old total, or start from the
		// stale offset without clamping, sliced with start > end and killed
		// the render with "makeslice: cap out of range".
		{"a list that shrank below a stale offset spans nothing, and does not invert", Window{Offset: 200, Height: 10, Total: 3}, 3, 3},
		{"a list that emptied under a stale offset spans nothing", Window{Offset: 40, Height: 10, Total: 0}, 0, 0},
		{"a negative total spans nothing", Window{Offset: 2, Height: 10, Total: -1}, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end := tt.in.Range()
			if start != tt.wantStart || end != tt.wantE {
				t.Errorf("Range() = (%d, %d), want (%d, %d)", start, end, tt.wantStart, tt.wantE)
			}
			if start > end {
				t.Fatalf("Range() inverted: start %d > end %d", start, end)
			}
		})
	}
}

// TestRangeIsAlwaysSliceable is the property the panic case above is one
// instance of: whatever is in the fields, the span can be used to slice a
// list of Total elements without panicking.
func TestRangeIsAlwaysSliceable(t *testing.T) {
	for _, total := range []int{-2, 0, 1, 7} {
		for _, height := range []int{-3, 0, 1, 5, 100} {
			for _, offset := range []int{-4, 0, 3, 999} {
				w := Window{Offset: offset, Height: height, Total: total}
				start, end := w.Range()
				n := total
				if n < 0 {
					n = 0
				}
				if start < 0 || end < start || end > n {
					t.Fatalf("%+v: Range() = (%d, %d), not sliceable into a list of %d", w, start, end, n)
				}
				list := make([]int, n)
				_ = list[start:end] // panics if the span is bad
			}
		}
	}
}

func TestEnsureVisible(t *testing.T) {
	tests := []struct {
		name string
		in   Window
		want Window
	}{
		{
			"a cursor already on screen does not scroll",
			Window{Cursor: 5, Offset: 3, Height: 10, Total: 30},
			Window{Cursor: 5, Offset: 3, Height: 10, Total: 30},
		},
		{
			"a cursor one row above scrolls up by exactly one",
			Window{Cursor: 9, Offset: 10, Height: 10, Total: 30},
			Window{Cursor: 9, Offset: 9, Height: 10, Total: 30},
		},
		{
			"a cursor one row below scrolls down by exactly one",
			Window{Cursor: 10, Offset: 0, Height: 10, Total: 30},
			Window{Cursor: 10, Offset: 1, Height: 10, Total: 30},
		},
		{
			"a cursor far above jumps the offset to it",
			Window{Cursor: 2, Offset: 20, Height: 10, Total: 30},
			Window{Cursor: 2, Offset: 2, Height: 10, Total: 30},
		},
		{
			"a viewport that grew reclaims the dead space below the last row",
			Window{Cursor: 28, Offset: 25, Height: 20, Total: 30},
			Window{Cursor: 28, Offset: 10, Height: 20, Total: 30},
		},
		{
			// Nothing is on screen, so there is nothing to scroll into view.
			// The offset is left alone rather than reset: it is the position
			// to restore once a real height arrives.
			"an unsized viewport leaves the offset where it was",
			Window{Cursor: 5, Offset: 2, Height: 0, Total: 30},
			Window{Cursor: 5, Offset: 2, Height: 0, Total: 30},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.EnsureVisible(); got != tt.want {
				t.Errorf("EnsureVisible() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestEnsureVisibleKeepsTheCursorOnScreen is the invariant EnsureVisible
// promises, across every shape of state a caller can produce.
func TestEnsureVisibleKeepsTheCursorOnScreen(t *testing.T) {
	for _, total := range []int{0, 1, 5, 40} {
		for _, height := range []int{0, 1, 4, 60} {
			for _, cursor := range []int{-5, 0, 3, 39, 500} {
				for _, offset := range []int{-2, 0, 7, 300} {
					w := Window{Cursor: cursor, Offset: offset, Height: height, Total: total}.EnsureVisible()
					if total == 0 || height == 0 {
						continue // nothing can be on screen
					}
					if _, visible := w.CursorRow(); !visible {
						t.Fatalf("EnsureVisible() left the cursor off screen: %+v", w)
					}
				}
			}
		}
	}
}

func TestCenter(t *testing.T) {
	tests := []struct {
		name string
		in   Window
		want Window
	}{
		{
			"the cursor lands in the middle of the viewport",
			Window{Cursor: 20, Height: 10, Total: 100},
			Window{Cursor: 20, Offset: 15, Height: 10, Total: 100},
		},
		{
			"near the top the offset stops at zero",
			Window{Cursor: 2, Height: 10, Total: 100},
			Window{Cursor: 2, Offset: 0, Height: 10, Total: 100},
		},
		{
			"near the bottom the offset stops at the last full page",
			Window{Cursor: 98, Height: 10, Total: 100},
			Window{Cursor: 98, Offset: 90, Height: 10, Total: 100},
		},
		{
			"a list shorter than the viewport centers nowhere",
			Window{Cursor: 1, Height: 20, Total: 3},
			Window{Cursor: 1, Offset: 0, Height: 20, Total: 3},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.Center(); got != tt.want {
				t.Errorf("Center() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestMoveBy(t *testing.T) {
	tests := []struct {
		name  string
		in    Window
		delta int
		want  Window
	}{
		{
			"moving down within the viewport does not scroll",
			Window{Cursor: 0, Height: 10, Total: 30},
			3,
			Window{Cursor: 3, Height: 10, Total: 30},
		},
		{
			"moving past the bottom edge scrolls by the overshoot",
			Window{Cursor: 9, Height: 10, Total: 30},
			3,
			Window{Cursor: 12, Offset: 3, Height: 10, Total: 30},
		},
		{
			"the cursor stops at the last row rather than wrapping",
			Window{Cursor: 28, Offset: 20, Height: 10, Total: 30},
			50,
			Window{Cursor: 29, Offset: 20, Height: 10, Total: 30},
		},
		{
			"the cursor stops at the first row rather than wrapping",
			Window{Cursor: 2, Offset: 2, Height: 10, Total: 30},
			-50,
			Window{Cursor: 0, Offset: 0, Height: 10, Total: 30},
		},
		{
			"moving in an empty list is a no-op",
			Window{Height: 10},
			5,
			Window{Height: 10},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.MoveBy(tt.delta); got != tt.want {
				t.Errorf("MoveBy(%d) = %+v, want %+v", tt.delta, got, tt.want)
			}
		})
	}
}

func TestPageBy(t *testing.T) {
	tests := []struct {
		name  string
		in    Window
		pages int
		want  Window
	}{
		{
			"page down moves a viewport's worth",
			Window{Cursor: 0, Height: 10, Total: 100},
			1,
			Window{Cursor: 10, Offset: 1, Height: 10, Total: 100},
		},
		{
			"page up moves a viewport's worth back",
			Window{Cursor: 40, Offset: 35, Height: 10, Total: 100},
			-1,
			Window{Cursor: 30, Offset: 30, Height: 10, Total: 100},
		},
		{
			"an unsized viewport pages by one row rather than standing still",
			Window{Cursor: 4, Height: 0, Total: 100},
			1,
			Window{Cursor: 5, Height: 0, Total: 100},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.PageBy(tt.pages); got != tt.want {
				t.Errorf("PageBy(%d) = %+v, want %+v", tt.pages, got, tt.want)
			}
		})
	}
}

func TestTopAndBottom(t *testing.T) {
	w := Window{Cursor: 50, Offset: 45, Height: 10, Total: 100}

	if got, want := w.Top(), (Window{Cursor: 0, Offset: 0, Height: 10, Total: 100}); got != want {
		t.Errorf("Top() = %+v, want %+v", got, want)
	}
	if got, want := w.Bottom(), (Window{Cursor: 99, Offset: 90, Height: 10, Total: 100}); got != want {
		t.Errorf("Bottom() = %+v, want %+v", got, want)
	}
	if got := (Window{Height: 10}).Bottom(); got.Cursor != 0 {
		t.Errorf("Bottom() on an empty list = cursor %d, want 0", got.Cursor)
	}
}

func TestAtTopAtBottom(t *testing.T) {
	tests := []struct {
		name                string
		in                  Window
		wantTop, wantBottom bool
	}{
		{"scrolled to the top of a long list", Window{Height: 10, Total: 100}, true, false},
		{"scrolled to the end of a long list", Window{Offset: 90, Height: 10, Total: 100}, false, true},
		{"scrolled into the middle", Window{Offset: 40, Height: 10, Total: 100}, false, false},
		{"a list shorter than the viewport is both", Window{Height: 20, Total: 3}, true, true},
		{"an empty list is both", Window{Height: 20}, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.AtTop(); got != tt.wantTop {
				t.Errorf("AtTop() = %v, want %v", got, tt.wantTop)
			}
			if got := tt.in.AtBottom(); got != tt.wantBottom {
				t.Errorf("AtBottom() = %v, want %v", got, tt.wantBottom)
			}
		})
	}
}

func TestCursorRow(t *testing.T) {
	tests := []struct {
		name        string
		in          Window
		wantRow     int
		wantVisible bool
	}{
		{"the cursor at the top of the viewport is row 0", Window{Cursor: 5, Offset: 5, Height: 10, Total: 30}, 0, true},
		{"the cursor mid-viewport is its distance from the offset", Window{Cursor: 8, Offset: 5, Height: 10, Total: 30}, 3, true},
		{"a cursor above the viewport is not visible", Window{Cursor: 1, Offset: 5, Height: 10, Total: 30}, 0, false},
		{"a cursor below the viewport is not visible", Window{Cursor: 25, Offset: 5, Height: 10, Total: 30}, 0, false},
		{"nothing is visible in an empty list", Window{Cursor: 0, Height: 10}, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row, visible := tt.in.CursorRow()
			if row != tt.wantRow || visible != tt.wantVisible {
				t.Errorf("CursorRow() = (%d, %v), want (%d, %v)", row, visible, tt.wantRow, tt.wantVisible)
			}
		})
	}
}

func TestLen(t *testing.T) {
	if got := (Window{Offset: 25, Height: 10, Total: 30}).Len(); got != 5 {
		t.Errorf("Len() on a short last page = %d, want 5", got)
	}
	if got := (Window{Offset: 200, Height: 10, Total: 3}).Len(); got != 0 {
		t.Errorf("Len() on a shrunk list = %d, want 0", got)
	}
}
