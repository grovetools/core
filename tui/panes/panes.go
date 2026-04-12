package panes

import tea "github.com/charmbracelet/bubbletea"

// Direction controls the split orientation.
type Direction int

const (
	DirectionHorizontal Direction = iota // Side-by-side
	DirectionVertical                    // Top-and-bottom
)

// Pane wraps an underlying tea.Model with layout preferences.
type Pane struct {
	ID      string
	Model   tea.Model
	Flex    int  // Flex ratio (e.g., 1 for 33%, 2 for 66%). Ignored when Fixed > 0.
	Fixed   int  // If > 0, pane gets exactly this many cells on the axis. Overrides Flex.
	MinSize int  // MinWidth for Horizontal, MinHeight for Vertical
	Hidden  bool // Hidden panes are excluded from layout, rendering, and focus cycling.
}

// Focusable is an optional interface inner models can implement
// to receive focus/blur lifecycle events.
type Focusable interface {
	Focus() tea.Cmd
	Blur()
}

// TextInputActive gates pane-switching keys. If the active pane's model
// returns true, keys like Tab, z, V fall through to the inner model.
type TextInputActive interface {
	IsTextEntryActive() bool
}

// StatusProvider is an optional interface inner models can implement
// to display a 1-line status bar below the pane content.
type StatusProvider interface {
	StatusLine() string
}

// Manager orchestrates layout, focus, and rendering for multiple Panes.
type Manager struct {
	Panes         []Pane
	Direction     Direction
	ActivePaneIdx int
	FullscreenIdx int  // -1 means no pane is fullscreen
	PinnedMode    bool // true = pinned zoom (Fixed panes stay visible at MinSize)
	Width         int
	Height        int
	KeyMap        KeyMap
}

// New creates a Manager with the given panes and default settings.
func New(panes ...Pane) Manager {
	m := Manager{
		Panes:         panes,
		Direction:     DirectionHorizontal,
		ActivePaneIdx: 0,
		FullscreenIdx: -1,
		KeyMap:        DefaultKeyMap(),
	}
	return m
}

// ActiveModel returns the currently focused pane's model.
func (m Manager) ActiveModel() tea.Model {
	if m.ActivePaneIdx >= 0 && m.ActivePaneIdx < len(m.Panes) {
		return m.Panes[m.ActivePaneIdx].Model
	}
	return nil
}

// ActivePane returns the currently focused pane.
func (m Manager) ActivePane() *Pane {
	if m.ActivePaneIdx >= 0 && m.ActivePaneIdx < len(m.Panes) {
		return &m.Panes[m.ActivePaneIdx]
	}
	return nil
}

// IsTextInputActive checks if the active pane's model has text entry active.
func (m Manager) IsTextInputActive() bool {
	model := m.ActiveModel()
	if model == nil {
		return false
	}
	if tia, ok := model.(TextInputActive); ok {
		return tia.IsTextEntryActive()
	}
	return false
}

// SetHidden toggles a pane's visibility. When a pane is hidden it is excluded
// from layout, rendering, and focus cycling. Returns an updated Manager and a
// tea.Cmd that redistributes sizes so remaining panes reflow immediately.
func (m Manager) SetHidden(id string, hidden bool) (Manager, tea.Cmd) {
	for i := range m.Panes {
		if m.Panes[i].ID != id {
			continue
		}
		if m.Panes[i].Hidden == hidden {
			return m, nil // no change
		}
		m.Panes[i].Hidden = hidden

		if hidden {
			// If this pane was fullscreened, exit fullscreen.
			if m.FullscreenIdx == i {
				m.FullscreenIdx = -1
			}
			// If this pane was focused, cycle to the next visible pane.
			if m.ActivePaneIdx == i {
				m = m.advanceToVisible(1)
			}
		}
		return m.distributeSize()
	}
	return m, nil
}

// IsHidden returns true if the pane with the given ID is hidden.
func (m Manager) IsHidden(id string) bool {
	for _, p := range m.Panes {
		if p.ID == id {
			return p.Hidden
		}
	}
	return false
}

// advanceToVisible moves ActivePaneIdx in the given direction until a
// non-hidden pane is found. If all panes are hidden, keeps current index.
func (m Manager) advanceToVisible(delta int) Manager {
	n := len(m.Panes)
	for range n {
		m.ActivePaneIdx = (m.ActivePaneIdx + delta + n) % n
		if !m.Panes[m.ActivePaneIdx].Hidden {
			return m
		}
	}
	return m
}
