package panes

import tea "github.com/charmbracelet/bubbletea"

// CalculateDimensions distributes space among visible panes.
// Fixed panes consume their exact size first; the remainder is distributed
// among flex panes by ratio, clamping to MinSize.
// Returns a WindowSizeMsg per pane (hidden panes get zero size).
func (m Manager) CalculateDimensions() []tea.WindowSizeMsg {
	n := len(m.Panes)
	if n == 0 {
		return nil
	}

	// Pinned mode: fullscreened pane gets flex, Fixed panes get MinSize, other Flex panes get 0
	if m.FullscreenIdx >= 0 && m.PinnedMode {
		return m.CalculatePinnedDimensions()
	}

	// Determine axis dimension and cross dimension
	var axisDim, crossDim int
	if m.Direction == DirectionHorizontal {
		axisDim = m.Width
		crossDim = m.Height
	} else {
		axisDim = m.Height
		crossDim = m.Width
	}

	// Count visible panes for separator math
	visibleCount := 0
	for _, p := range m.Panes {
		if !p.Hidden && !p.Promoted {
			visibleCount++
		}
	}

	// Subtract separator space (1 char per gap between visible panes)
	separators := max(visibleCount-1, 0)
	available := max(axisDim-separators, 0)

	// Allocate sizes array — hidden panes stay at 0
	sizes := make([]int, n)

	// First: subtract Fixed pane sizes from available space
	for i, p := range m.Panes {
		if p.Hidden || p.Promoted {
			continue
		}
		if p.Fixed > 0 {
			sizes[i] = min(p.Fixed, available)
			available -= sizes[i]
			available = max(available, 0)
		}
	}

	// Second: distribute remaining space among flex panes (Fixed == 0, !Hidden)
	flexible := make([]bool, n)
	for i, p := range m.Panes {
		if !p.Hidden && !p.Promoted && p.Fixed == 0 {
			flexible[i] = true
		}
	}

	// Iterative clamping loop for MinSize
	for {
		changed := false
		flexSum := 0
		flexAvailable := available
		for i, p := range m.Panes {
			if !flexible[i] {
				if p.Fixed == 0 && !p.Hidden {
					// Already clamped flex pane — subtract its locked size
					flexAvailable -= sizes[i]
				}
				continue
			}
			flexSum += p.Flex
		}
		flexAvailable = max(flexAvailable, 0)

		for i, p := range m.Panes {
			if !flexible[i] {
				continue
			}
			var allocated int
			if flexSum > 0 {
				allocated = (flexAvailable * p.Flex) / flexSum
			}
			// Only clamp up to MinSize if MinSize fits in remaining space.
			// Otherwise, when the available axis is smaller than the sum of
			// MinSizes, MinSize-clamping would push total allocation past
			// axisDim and overflow the parent layout.
			if p.MinSize > 0 && allocated < p.MinSize && p.MinSize <= flexAvailable {
				sizes[i] = p.MinSize
				flexible[i] = false
				changed = true
				break
			}
			sizes[i] = allocated
		}
		if !changed {
			break
		}
	}

	// Build WindowSizeMsg per pane
	msgs := make([]tea.WindowSizeMsg, n)
	for i := range m.Panes {
		if m.Panes[i].Hidden {
			continue
		}
		w, h := sizes[i], crossDim
		if m.Direction == DirectionVertical {
			w, h = crossDim, sizes[i]
		}
		// Subtract 1 row for status line if the model implements StatusProvider
		if _, ok := m.Panes[i].Model.(StatusProvider); ok {
			if m.Direction == DirectionHorizontal {
				h = max(h-1, 0)
			} else {
				h = max(h-1, 0)
			}
		}
		msgs[i] = tea.WindowSizeMsg{Width: w, Height: h}
	}
	return msgs
}

// CalculatePinnedDimensions handles layout when PinnedMode is active.
// The fullscreened pane gets all remaining space; Fixed panes render at MinSize;
// other Flex panes are hidden (0 size).
func (m Manager) CalculatePinnedDimensions() []tea.WindowSizeMsg {
	n := len(m.Panes)
	msgs := make([]tea.WindowSizeMsg, n)

	var axisDim, crossDim int
	if m.Direction == DirectionHorizontal {
		axisDim = m.Width
		crossDim = m.Height
	} else {
		axisDim = m.Height
		crossDim = m.Width
	}

	// Count visible panes that will render (Fixed + the zoomed pane)
	visibleCount := 0
	for i, p := range m.Panes {
		if p.Hidden || p.Promoted {
			continue
		}
		if i == m.FullscreenIdx {
			visibleCount++
		} else if p.Fixed > 0 {
			visibleCount++
		}
		// Other Flex panes get 0 — not counted
	}

	separators := max(visibleCount-1, 0)
	available := max(axisDim-separators, 0)

	// Allocate Fixed panes at MinSize first
	sizes := make([]int, n)
	for i, p := range m.Panes {
		if p.Hidden || i == m.FullscreenIdx {
			continue
		}
		if p.Fixed > 0 {
			s := max(p.MinSize, 1)
			sizes[i] = min(s, available)
			available -= sizes[i]
			available = max(available, 0)
		}
		// Flex panes (not the zoomed one) stay at 0
	}

	// Zoomed pane gets the rest
	if m.FullscreenIdx >= 0 && m.FullscreenIdx < n {
		sizes[m.FullscreenIdx] = max(available, 0)
	}

	for i := range m.Panes {
		if m.Panes[i].Hidden {
			continue
		}
		w, h := sizes[i], crossDim
		if m.Direction == DirectionVertical {
			w, h = crossDim, sizes[i]
		}
		// Subtract 1 row for status line if the model implements StatusProvider
		if _, ok := m.Panes[i].Model.(StatusProvider); ok {
			if m.Direction == DirectionHorizontal {
				h = max(h-1, 0)
			} else {
				h = max(h-1, 0)
			}
		}
		msgs[i] = tea.WindowSizeMsg{Width: w, Height: h}
	}

	return msgs
}
