package panes

import tea "github.com/charmbracelet/bubbletea"

// calculateDimensions distributes space among visible panes.
// Fixed panes consume their exact size first; the remainder is distributed
// among flex panes by ratio, clamping to MinSize.
// Returns a WindowSizeMsg per pane (hidden panes get zero size).
func (m Manager) calculateDimensions() []tea.WindowSizeMsg {
	n := len(m.Panes)
	if n == 0 {
		return nil
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
		if !p.Hidden {
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
		if p.Hidden {
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
		if !p.Hidden && p.Fixed == 0 {
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
			if p.MinSize > 0 && allocated < p.MinSize {
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
		if m.Direction == DirectionHorizontal {
			msgs[i] = tea.WindowSizeMsg{Width: sizes[i], Height: crossDim}
		} else {
			msgs[i] = tea.WindowSizeMsg{Width: crossDim, Height: sizes[i]}
		}
	}
	return msgs
}
