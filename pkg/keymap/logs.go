// Package keymap contains extracted TUI keymaps for registry integration.
package keymap

import (
	"github.com/charmbracelet/bubbles/key"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/tui/keymap"
)

// LogKeyMap defines all key bindings for the logs TUI.
type LogKeyMap struct {
	keymap.Base
	PageUp           key.Binding
	PageDown         key.Binding
	HalfUp           key.Binding
	HalfDown         key.Binding
	GotoTop          key.Binding
	GotoEnd          key.Binding
	Expand           key.Binding
	Search           key.Binding
	Clear            key.Binding
	ToggleFollow     key.Binding
	ToggleFilters    key.Binding
	ToggleEvents     key.Binding
	ViewJSON         key.Binding
	VisualModeStart  key.Binding
	Yank             key.Binding
	SwitchFocus      key.Binding
	ToggleScope      key.Binding
	ToggleSystem     key.Binding
	CycleLevel       key.Binding
	ComponentSummary key.Binding
	ClearBuffer      key.Binding
	CopyRawText      key.Binding
	OpenEditor       key.Binding
}

// NewLogKeyMap creates a new LogKeyMap with user configuration applied.
func NewLogKeyMap(cfg *config.Config) LogKeyMap {
	km := LogKeyMap{
		Base: keymap.Load(cfg, "core.logs"),
		PageUp: key.NewBinding(
			key.WithKeys("pgup"),
			key.WithHelp("pgup", "page up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown"),
			key.WithHelp("pgdn", "page down"),
		),
		HalfUp: key.NewBinding(
			key.WithKeys("ctrl+u"),
			key.WithHelp("ctrl+u", "half page up"),
		),
		HalfDown: key.NewBinding(
			key.WithKeys("ctrl+d"),
			key.WithHelp("ctrl+d", "half page down"),
		),
		GotoTop: key.NewBinding(
			key.WithKeys("gg"),
			key.WithHelp("gg", "go to top"),
		),
		GotoEnd: key.NewBinding(
			key.WithKeys("G"),
			key.WithHelp("G", "go to end"),
		),
		Expand: key.NewBinding(
			key.WithKeys(" ", "enter"),
			key.WithHelp("space/enter", "expand/collapse"),
		),
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		Clear: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "clear/back"),
		),
		// Toggle (t…) namespace member. RULE T (canon 60 §4.2): every
		// display/filter toggle is a chord. `tt` = toggle tail/follow.
		ToggleFollow: key.NewBinding(
			key.WithKeys("tt"),
			key.WithHelp("tt", "toggle follow (tail)"),
		),
		// Toggle (t…) namespace member (was flat `f`).
		ToggleFilters: key.NewBinding(
			key.WithKeys("tf"),
			key.WithHelp("tf", "toggle filters"),
		),
		// Toggle (t…) namespace member (was flat `E`).
		ToggleEvents: key.NewBinding(
			key.WithKeys("te"),
			key.WithHelp("te", "toggle events only"),
		),
		// View (v…) namespace member (was flat `J`). Canon 60 §4.1.
		ViewJSON: key.NewBinding(
			key.WithKeys("vj"),
			key.WithHelp("vj", "view json"),
		),
		VisualModeStart: key.NewBinding(
			key.WithKeys("V"),
			key.WithHelp("V", "visual line mode"),
		),
		// The canonical vim yank chord (was flat `y`). Canon 60 §5.6: flat `y`
		// is the arming letter for `yy` and must stay unbound. Base.Yank is
		// disabled below so `yy` routes here.
		Yank: key.NewBinding(
			key.WithKeys("yy"),
			key.WithHelp("yy", "yank json"),
		),
		SwitchFocus: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "switch focus"),
		),
		// Toggle (t…) namespace member (was flat `s`).
		//
		// The consolidation pass moved scope off `ts` onto `tw`: `ts` was
		// carrying four meanings across eight TUIs, and sort held the
		// plurality (cx-view, grove-config, nb-browser). `tw` = "this
		// Workspace" — the local half of the local/global toggle — and is
		// shared with hooks-browser and memory-view.
		ToggleScope: key.NewBinding(
			key.WithKeys("tw"),
			key.WithHelp("tw", "cycle scope"),
		),
		// Toggle (t…) namespace member (was flat `S`). Uppercase-in-chord is
		// established house style (flow-status ships cM/cA).
		ToggleSystem: key.NewBinding(
			key.WithKeys("tS"),
			key.WithHelp("tS", "toggle system logs"),
		),
		// Toggle (t…) namespace member. Vacates flat `v`, the reserved view
		// prefix this TUI was squatting on.
		CycleLevel: key.NewBinding(
			key.WithKeys("tl"),
			key.WithHelp("tl", "cycle log level"),
		),
		// Toggle (t…) namespace member (was flat `C`).
		ComponentSummary: key.NewBinding(
			key.WithKeys("tc"),
			key.WithHelp("tc", "component filter"),
		),
		ClearBuffer: key.NewBinding(
			key.WithKeys("ctrl+l"),
			key.WithHelp("ctrl+l", "clear buffer"),
		),
		// Ring-1 `Y` = yank the ALTERNATE payload (canon 60 §5.1), matching
		// memory-view's Y=copy_chunk. Vacates flat `c`, the reserved change
		// prefix this TUI was squatting on.
		CopyRawText: key.NewBinding(
			key.WithKeys("Y"),
			key.WithHelp("Y", "copy raw text"),
		),
		OpenEditor: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "open in editor"),
		),
	}

	// Base bindings the logs viewer does not implement. They were enabled but
	// unreachable, so they were invisible squatters — notably Base.TogglePreview
	// on flat `v` (the reserved view prefix this TUI now uses as a namespace),
	// Base.Confirm's `y` half (the `yy` arming letter), and Base.ClearSearch on
	// ctrl+l (which the log viewer means as clear-buffer, canon 60 §7.4).
	// Disabling them keeps help truthful and AuditCoverage clean. Only Up, Down,
	// Left, Right, Help and Quit are actually dispatched from Base.
	disableBindings(
		&km.Base.PageUp, &km.Base.PageDown, &km.Base.Home, &km.Base.End,
		&km.Base.Top, &km.Base.Bottom,
		&km.Base.Confirm, &km.Base.Cancel, &km.Base.Back, &km.Base.Edit,
		&km.Base.Delete, &km.Base.Yank, &km.Base.Rename, &km.Base.Refresh,
		&km.Base.CopyPath,
		&km.Base.SearchNext, &km.Base.SearchPrev, &km.Base.ClearSearch, &km.Base.Grep,
		&km.Base.SwitchView, &km.Base.NextTab, &km.Base.PrevTab,
		&km.Base.FocusNext, &km.Base.FocusPrev, &km.Base.TogglePreview,
		&km.Base.Tab1, &km.Base.Tab2, &km.Base.Tab3, &km.Base.Tab4, &km.Base.Tab5,
		&km.Base.Tab6, &km.Base.Tab7, &km.Base.Tab8, &km.Base.Tab9,
		&km.Base.Select, &km.Base.SelectAll, &km.Base.SelectNone,
		&km.Base.FoldOpen, &km.Base.FoldClose, &km.Base.FoldToggle,
		&km.Base.FoldOpenAll, &km.Base.FoldCloseAll,
	)

	// Apply TUI-specific overrides from config
	keymap.ApplyTUIOverrides(cfg, "core", "logs", &km)

	return km
}

// disableBindings turns off a set of bindings in place. Used to switch off the
// keymap.Base defaults a TUI never dispatches, so they neither render in help
// nor squat on a key the chord canon needs.
func disableBindings(bs ...*key.Binding) {
	for _, b := range bs {
		b.SetEnabled(false)
	}
}

// Namespaces returns the which-key chord namespaces for the logs TUI, built
// from the named LogKeyMap fields (so any user override applied by
// ApplyTUIOverrides is reflected — namespace.go's ConfigKey-stability rule;
// never construct members inline). Order here is the wire order ProcessChord
// relies on.
func (k LogKeyMap) Namespaces() []keymap.Namespace {
	return []keymap.Namespace{
		{Prefix: "t", Label: "Toggle", Bindings: []key.Binding{
			k.CycleLevel, k.ToggleScope, k.ToggleSystem, k.ToggleFilters,
			k.ToggleEvents, k.ToggleFollow, k.ComponentSummary,
		}},
		{Prefix: "v", Label: "View", Bindings: []key.Binding{
			k.ViewJSON,
		}},
	}
}

// Sections returns the log viewer's grouped keybindings for structured help.
// LogKeyMap embeds Base, whose promoted Sections method describes the generic
// base keymap; defining this method ensures help shows the log-specific keys.
func (k LogKeyMap) Sections() []keymap.Section {
	ns := k.Namespaces()
	return []keymap.Section{
		keymap.NavigationSection(
			k.Base.Up, k.Base.Down, k.Base.Left, k.Base.Right,
			k.PageUp, k.PageDown,
			k.HalfUp, k.HalfDown, k.GotoTop, k.GotoEnd,
		),
		// Toggle (t…) and View (v…) namespace sections, so the ? overlay and
		// the generated registry list the chord members as ordinary bindings.
		ns[0].Section(),
		ns[1].Section(),
		keymap.ActionsSection(
			k.Expand, k.VisualModeStart, k.Yank,
			k.CopyRawText, k.ClearBuffer, k.OpenEditor, k.SwitchFocus,
			k.Search, k.Clear,
		),
		keymap.SystemSection(k.Base.Help, k.Base.Quit),
	}
}

// ShortHelp returns keybindings to be shown in the mini help view.
func (k LogKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Base.Help, k.Base.Quit, k.ToggleScope, k.CycleLevel, k.ComponentSummary, k.Search, k.ToggleFollow}
}

// FullHelp returns keybindings for the expanded help view.
func (k LogKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{ // Navigation
			k.Base.Up,
			k.Base.Down,
			k.PageUp,
			k.PageDown,
			k.HalfUp,
			k.HalfDown,
			k.GotoTop,
			k.GotoEnd,
		},
		{ // Toggle (t…) / View (v…)
			k.ToggleScope,
			k.ToggleSystem,
			k.CycleLevel,
			k.ComponentSummary,
			k.ToggleFilters,
			k.ToggleEvents,
			k.ToggleFollow,
			k.Search,
		},
		{ // Actions
			k.ViewJSON,
			k.VisualModeStart,
			k.Yank,
			k.CopyRawText,
			k.ClearBuffer,
			k.OpenEditor,
			k.SwitchFocus,
			k.Base.Help,
			k.Base.Quit,
		},
	}
}

// KeymapInfo returns the keymap metadata for the logs TUI.
// Used by the grove keys registry generator to aggregate all TUI keybindings.
func KeymapInfo() keymap.TUIInfo {
	return keymap.MakeTUIInfo(
		"core-logs",
		"core",
		"Aggregated log viewer with filtering and search",
		NewLogKeyMap(nil),
	)
}
