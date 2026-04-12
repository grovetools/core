package panes

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines the keybindings for the pane manager.
type KeyMap struct {
	CycleNext        key.Binding
	CyclePrev        key.Binding
	ToggleFullscreen key.Binding
	TogglePinned     key.Binding
	ToggleDirection  key.Binding
	ResizeGrow       key.Binding
	ResizeShrink     key.Binding
}

// DefaultKeyMap returns the standard pane manager keybindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		CycleNext: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next pane"),
		),
		CyclePrev: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "prev pane"),
		),
		ToggleFullscreen: key.NewBinding(
			key.WithKeys("z"),
			key.WithHelp("z", "zoom"),
		),
		TogglePinned: key.NewBinding(
			key.WithKeys("Z"),
			key.WithHelp("Z", "pin zoom"),
		),
		ToggleDirection: key.NewBinding(
			key.WithKeys("V"),
			key.WithHelp("V", "split dir"),
		),
		ResizeGrow: key.NewBinding(
			key.WithKeys("+"),
			key.WithHelp("+", "grow pane"),
		),
		ResizeShrink: key.NewBinding(
			key.WithKeys("-"),
			key.WithHelp("-", "shrink pane"),
		),
	}
}

// ShortHelp returns the keybindings for the short help view.
func (km KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{
		km.CycleNext,
		km.ToggleFullscreen,
		km.TogglePinned,
		km.ToggleDirection,
		km.ResizeGrow,
		km.ResizeShrink,
	}
}

// FullHelp returns the keybindings for the full help view.
func (km KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{km.CycleNext, km.CyclePrev, km.ToggleFullscreen, km.TogglePinned, km.ToggleDirection},
		{km.ResizeGrow, km.ResizeShrink},
	}
}
