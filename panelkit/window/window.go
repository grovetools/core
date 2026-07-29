// Package window is the scroll-and-cursor arithmetic behind a viewport onto a
// flat list.
//
// Every list view in the ecosystem had re-derived this: a cursor index, a
// scroll offset, a visible height, and four operations over them — clamp the
// state, compute the visible span, move the cursor, keep the cursor on screen.
// It was re-implemented some twenty times, subtly differently each time, and
// at least one of those copies could panic: a filter that shrank the list
// below a scrolled-down offset left the render slicing [start:end] with
// start > end ("makeslice: cap out of range").
//
// The type is a value with value methods, so it composes into bubbletea models
// that copy themselves, and every operation is pure integer arithmetic with no
// rendering, no strings and no dependencies. Nothing here can panic and
// nothing here returns an out-of-range span, whatever you put in the fields.
package window

// Window is a viewport onto a flat list of Total rows, showing Height of them
// starting at Offset, with a Cursor somewhere in the list.
//
// Cursor and Offset are in list coordinates: Cursor is an index into the list,
// not a screen row. The two are independent — a cursor can sit outside the
// visible span, which is exactly the state EnsureVisible exists to repair.
//
// The zero Window (an empty list, no height) is valid and yields an empty
// Range.
type Window struct {
	// Cursor is the index of the selected row.
	Cursor int
	// Offset is the index of the first visible row.
	Offset int
	// Height is how many rows the viewport can show. This is the body height
	// after chrome — headers, footers, borders — has been deducted.
	Height int
	// Total is how many rows the list holds.
	Total int
}

// Clamp returns w with every field pulled into a coherent range: no negative
// height or total, a cursor inside the list, and an offset that neither goes
// negative nor sits past the last full page.
//
// "Past the last full page" is the non-obvious one. An offset larger than
// Total-Height means rows scrolled off the top while blank space sits below
// the last row, which is never the right picture; it happens when a pane grows
// (a terminal resize, a split closing) and keeps the offset it had while it
// was short.
func (w Window) Clamp() Window {
	if w.Total < 0 {
		w.Total = 0
	}
	if w.Height < 0 {
		w.Height = 0
	}

	if w.Cursor < 0 || w.Total == 0 {
		w.Cursor = 0
	} else if w.Cursor > w.Total-1 {
		w.Cursor = w.Total - 1
	}

	if max := w.Total - w.Height; w.Offset > max {
		w.Offset = max
	}
	if w.Offset < 0 {
		w.Offset = 0
	}
	return w
}

// Range returns the [start, end) span of list indices the viewport shows.
//
// Both bounds are clamped into the list independently of the rest of the
// state, so the span is never inverted and never out of range even when the
// window is stale — a list that shrank under a scrolled-down offset returns an
// empty span here rather than panicking the render. Callers may slice with the
// result directly.
func (w Window) Range() (start, end int) {
	total := w.Total
	if total < 0 {
		total = 0
	}
	start = w.Offset
	if start < 0 {
		start = 0
	}
	if start > total {
		start = total
	}
	height := w.Height
	if height < 0 {
		height = 0
	}
	end = start + height
	if end > total {
		end = total
	}
	return start, end
}

// Len returns the number of rows Range spans.
func (w Window) Len() int {
	start, end := w.Range()
	return end - start
}

// EnsureVisible scrolls the minimum distance needed to bring the cursor inside
// the visible span, then applies Clamp.
//
// Minimum distance is the point: a cursor one row above the viewport scrolls
// up by one, not to the top, and the rows already on screen stay where the eye
// left them. Pulling the offset back to the last full page afterwards can only
// move the cursor further inside the viewport, never out of it.
func (w Window) EnsureVisible() Window {
	w = w.Clamp()
	if w.Height <= 0 {
		return w
	}
	if w.Cursor < w.Offset {
		w.Offset = w.Cursor
	} else if w.Cursor >= w.Offset+w.Height {
		w.Offset = w.Cursor - w.Height + 1
	}
	return w.Clamp()
}

// Center scrolls so the cursor sits as close to the middle of the viewport as
// the ends of the list allow, then applies Clamp.
//
// This is the other scroll policy in use: EnsureVisible moves as little as
// possible and is right for arrow-key navigation, Center re-frames around the
// cursor and is right for a jump — a search hit, a "go to definition", a
// restored selection — where the surrounding rows matter more than visual
// continuity. A list shorter than the viewport centers nowhere: Clamp pins the
// offset to 0.
func (w Window) Center() Window {
	w = w.Clamp()
	if w.Height <= 0 {
		return w
	}
	w.Offset = w.Cursor - w.Height/2
	return w.Clamp()
}

// MoveBy moves the cursor by delta rows and keeps it visible. The cursor stops
// at the ends of the list rather than wrapping.
func (w Window) MoveBy(delta int) Window {
	w = w.Clamp()
	w.Cursor += delta
	return w.EnsureVisible()
}

// MoveTo moves the cursor to an absolute index and keeps it visible.
func (w Window) MoveTo(index int) Window {
	w = w.Clamp()
	w.Cursor = index
	return w.EnsureVisible()
}

// PageBy moves the cursor by pages viewports — PageBy(1) is page-down,
// PageBy(-1) is page-up — and keeps it visible.
//
// A zero-height window pages by one row, so a viewport that has not been sized
// yet still responds to the key rather than appearing dead.
func (w Window) PageBy(pages int) Window {
	step := w.Height
	if step <= 0 {
		step = 1
	}
	return w.MoveBy(pages * step)
}

// Top moves the cursor to the first row and scrolls to the top.
func (w Window) Top() Window {
	w = w.Clamp()
	w.Cursor = 0
	w.Offset = 0
	return w.Clamp()
}

// Bottom moves the cursor to the last row and scrolls to the end.
func (w Window) Bottom() Window {
	w = w.Clamp()
	w.Cursor = w.Total - 1
	return w.EnsureVisible()
}

// AtTop reports whether the first row of the list is on screen.
func (w Window) AtTop() bool {
	start, _ := w.Range()
	return start <= 0
}

// AtBottom reports whether the last row of the list is on screen. An empty
// list is both at the top and at the bottom.
func (w Window) AtBottom() bool {
	_, end := w.Range()
	total := w.Total
	if total < 0 {
		total = 0
	}
	return end >= total
}

// CursorRow returns the cursor's position as a row within the visible span,
// and whether it is visible at all. Renderers use it to decide which of the
// rows they just sliced to draw as selected.
func (w Window) CursorRow() (row int, visible bool) {
	start, end := w.Range()
	if w.Cursor < start || w.Cursor >= end {
		return 0, false
	}
	return w.Cursor - start, true
}
