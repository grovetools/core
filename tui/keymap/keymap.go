package keymap

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/grovetools/core/config"
)

// Base contains the standard keybindings used across all Grove TUIs
// Prioritizes vim-style navigation and standard actions
type Base struct {
	// Navigation - vim style takes precedence
	Up       key.Binding
	Down     key.Binding
	Left     key.Binding
	Right    key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	Home     key.Binding
	End      key.Binding
	Top      key.Binding // gg sequence
	Bottom   key.Binding // G

	// Core actions
	Quit    key.Binding
	Help    key.Binding
	Confirm key.Binding
	Cancel  key.Binding
	Back    key.Binding
	Edit    key.Binding
	Delete  key.Binding // dd sequence
	Yank    key.Binding // yy sequence

	// Search
	Search      key.Binding
	SearchNext  key.Binding
	SearchPrev  key.Binding
	ClearSearch key.Binding
	Grep        key.Binding

	// View management
	SwitchView    key.Binding
	NextTab       key.Binding
	PrevTab       key.Binding
	FocusNext     key.Binding
	FocusPrev     key.Binding
	TogglePreview key.Binding

	// Selection
	Select       key.Binding
	SelectAll    key.Binding
	SelectNone   key.Binding
	ToggleSelect key.Binding

	// Fold operations (for tree-based TUIs)
	FoldOpen     key.Binding // zo
	FoldClose    key.Binding // zc
	FoldToggle   key.Binding // za
	FoldOpenAll  key.Binding // zR
	FoldCloseAll key.Binding // zM
}

// NewBase creates a new Base keymap with default Grove keybindings (vim style)
func NewBase() Base {
	return DefaultVim()
}

// DefaultVim returns the default vim-style keymap
func DefaultVim() Base {
	return Base{
		// Navigation - vim style takes precedence
		Up: key.NewBinding(
			key.WithKeys("k", "up"),
			key.WithHelp("k/up", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("j", "down"),
			key.WithHelp("j/down", "down"),
		),
		Left: key.NewBinding(
			key.WithKeys("h", "left"),
			key.WithHelp("h/left", "left"),
		),
		Right: key.NewBinding(
			key.WithKeys("l", "right"),
			key.WithHelp("l/right", "right"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("ctrl+u", "pgup"),
			key.WithHelp("C-u", "page up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("ctrl+d", "pgdown"),
			key.WithHelp("C-d", "page down"),
		),
		Home: key.NewBinding(
			key.WithKeys("home"),
			key.WithHelp("home", "start"),
		),
		End: key.NewBinding(
			key.WithKeys("end"),
			key.WithHelp("end", "end"),
		),
		Top: key.NewBinding(
			key.WithKeys("gg"),
			key.WithHelp("gg", "top"),
		),
		Bottom: key.NewBinding(
			key.WithKeys("G"),
			key.WithHelp("G", "bottom"),
		),

		// Core actions
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Confirm: key.NewBinding(
			key.WithKeys("enter", "y"),
			key.WithHelp("enter", "confirm"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("n", "ctrl+g"),
			key.WithHelp("n", "cancel"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Edit: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "edit"),
		),
		Delete: key.NewBinding(
			key.WithKeys("dd"),
			key.WithHelp("dd", "delete"),
		),
		Yank: key.NewBinding(
			key.WithKeys("yy"),
			key.WithHelp("yy", "yank"),
		),

		// Search - '/' initiates search as per vim convention
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		SearchNext: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "next match"),
		),
		SearchPrev: key.NewBinding(
			key.WithKeys("N"),
			key.WithHelp("N", "prev match"),
		),
		ClearSearch: key.NewBinding(
			key.WithKeys("ctrl+l"),
			key.WithHelp("C-l", "clear search"),
		),
		Grep: key.NewBinding(
			key.WithKeys("*"),
			key.WithHelp("*", "grep"),
		),

		// View management - tab for switching views/components
		SwitchView: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "switch view"),
		),
		NextTab: key.NewBinding(
			key.WithKeys("]"),
			key.WithHelp("]", "next tab"),
		),
		PrevTab: key.NewBinding(
			key.WithKeys("["),
			key.WithHelp("[", "prev tab"),
		),
		FocusNext: key.NewBinding(
			key.WithKeys("ctrl+n"),
			key.WithHelp("C-n", "focus next"),
		),
		FocusPrev: key.NewBinding(
			key.WithKeys("ctrl+p"),
			key.WithHelp("C-p", "focus prev"),
		),
		TogglePreview: key.NewBinding(
			key.WithKeys("v"),
			key.WithHelp("v", "preview"),
		),

		// Selection
		Select: key.NewBinding(
			key.WithKeys("space", "x"),
			key.WithHelp("space", "select"),
		),
		SelectAll: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "all"),
		),
		SelectNone: key.NewBinding(
			key.WithKeys("A"),
			key.WithHelp("A", "none"),
		),
		ToggleSelect: key.NewBinding(
			key.WithKeys("v"),
			key.WithHelp("v", "toggle"),
		),

		// Fold operations
		FoldOpen: key.NewBinding(
			key.WithKeys("zo"),
			key.WithHelp("zo", "open fold"),
		),
		FoldClose: key.NewBinding(
			key.WithKeys("zc"),
			key.WithHelp("zc", "close fold"),
		),
		FoldToggle: key.NewBinding(
			key.WithKeys("za"),
			key.WithHelp("za", "toggle fold"),
		),
		FoldOpenAll: key.NewBinding(
			key.WithKeys("zR"),
			key.WithHelp("zR", "open all"),
		),
		FoldCloseAll: key.NewBinding(
			key.WithKeys("zM"),
			key.WithHelp("zM", "close all"),
		),
	}
}

// DefaultEmacs returns an emacs-style keymap
func DefaultEmacs() Base {
	b := DefaultVim()
	// Override navigation with emacs bindings
	b.Up = key.NewBinding(
		key.WithKeys("ctrl+p", "up"),
		key.WithHelp("C-p", "up"),
	)
	b.Down = key.NewBinding(
		key.WithKeys("ctrl+n", "down"),
		key.WithHelp("C-n", "down"),
	)
	b.Left = key.NewBinding(
		key.WithKeys("ctrl+b", "left"),
		key.WithHelp("C-b", "left"),
	)
	b.Right = key.NewBinding(
		key.WithKeys("ctrl+f", "right"),
		key.WithHelp("C-f", "right"),
	)
	b.PageUp = key.NewBinding(
		key.WithKeys("alt+v", "pgup"),
		key.WithHelp("M-v", "page up"),
	)
	b.PageDown = key.NewBinding(
		key.WithKeys("ctrl+v", "pgdown"),
		key.WithHelp("C-v", "page down"),
	)
	b.Top = key.NewBinding(
		key.WithKeys("alt+<", "home"),
		key.WithHelp("M-<", "top"),
	)
	b.Bottom = key.NewBinding(
		key.WithKeys("alt+>", "end"),
		key.WithHelp("M->", "bottom"),
	)
	// Emacs search
	b.Search = key.NewBinding(
		key.WithKeys("ctrl+s"),
		key.WithHelp("C-s", "search"),
	)
	b.SearchPrev = key.NewBinding(
		key.WithKeys("ctrl+r"),
		key.WithHelp("C-r", "prev match"),
	)
	return b
}

// DefaultArrows returns a simplified keymap using primarily arrow keys
func DefaultArrows() Base {
	b := DefaultVim()
	// Override navigation with arrow-only bindings
	b.Up = key.NewBinding(
		key.WithKeys("up"),
		key.WithHelp("up", "up"),
	)
	b.Down = key.NewBinding(
		key.WithKeys("down"),
		key.WithHelp("down", "down"),
	)
	b.Left = key.NewBinding(
		key.WithKeys("left"),
		key.WithHelp("left", "left"),
	)
	b.Right = key.NewBinding(
		key.WithKeys("right"),
		key.WithHelp("right", "right"),
	)
	b.PageUp = key.NewBinding(
		key.WithKeys("pgup", "shift+up"),
		key.WithHelp("PgUp", "page up"),
	)
	b.PageDown = key.NewBinding(
		key.WithKeys("pgdown", "shift+down"),
		key.WithHelp("PgDn", "page down"),
	)
	b.Top = key.NewBinding(
		key.WithKeys("home", "ctrl+home"),
		key.WithHelp("Home", "top"),
	)
	b.Bottom = key.NewBinding(
		key.WithKeys("end", "ctrl+end"),
		key.WithHelp("End", "bottom"),
	)
	// Simplified actions without sequences
	b.Delete = key.NewBinding(
		key.WithKeys("delete", "backspace"),
		key.WithHelp("Del", "delete"),
	)
	b.Yank = key.NewBinding(
		key.WithKeys("ctrl+c"),
		key.WithHelp("C-c", "copy"),
	)
	// Disable vim-style select, use ctrl
	b.SelectAll = key.NewBinding(
		key.WithKeys("ctrl+a"),
		key.WithHelp("C-a", "all"),
	)
	return b
}

// Load creates a Base keymap based on configuration.
// It starts with the selected preset (vim/emacs/arrows), then applies
// global keybinding overrides, and finally TUI-specific overrides.
func Load(cfg *config.Config, tuiName string) Base {
	// Determine which preset to use
	preset := "vim"
	if cfg != nil && cfg.TUI != nil && cfg.TUI.Preset != "" {
		preset = cfg.TUI.Preset
	}

	// Start with the preset
	var base Base
	switch preset {
	case "emacs":
		base = DefaultEmacs()
	case "arrows":
		base = DefaultArrows()
	default:
		base = DefaultVim()
	}

	// If no keybindings config, return preset as-is
	if cfg == nil || cfg.TUI == nil || cfg.TUI.Keybindings == nil {
		return base
	}

	kb := cfg.TUI.Keybindings

	// Apply global section overrides
	applyNavigationOverrides(&base, kb.Navigation)
	applySelectionOverrides(&base, kb.Selection)
	applyActionsOverrides(&base, kb.Actions)
	applySearchOverrides(&base, kb.Search)
	applyViewOverrides(&base, kb.View)
	applyFoldOverrides(&base, kb.Fold)
	applySystemOverrides(&base, kb.System)

	// Apply TUI-specific overrides
	if tuiName != "" && kb.Overrides != nil {
		if tuiOverrides, ok := kb.Overrides[tuiName]; ok {
			applyGenericOverrides(&base, tuiOverrides)
		}
	}

	return base
}

// Helper to update a binding with new keys while preserving the help description
func updateBinding(binding *key.Binding, keys []string) {
	if len(keys) > 0 {
		helpDesc := binding.Help().Desc
		*binding = key.NewBinding(
			key.WithKeys(keys...),
			key.WithHelp(keys[0], helpDesc),
		)
	}
}

func applyNavigationOverrides(base *Base, nav config.KeybindingSectionConfig) {
	if nav == nil {
		return
	}
	if k, ok := nav["up"]; ok {
		updateBinding(&base.Up, k)
	}
	if k, ok := nav["down"]; ok {
		updateBinding(&base.Down, k)
	}
	if k, ok := nav["left"]; ok {
		updateBinding(&base.Left, k)
	}
	if k, ok := nav["right"]; ok {
		updateBinding(&base.Right, k)
	}
	if k, ok := nav["page_up"]; ok {
		updateBinding(&base.PageUp, k)
	}
	if k, ok := nav["page_down"]; ok {
		updateBinding(&base.PageDown, k)
	}
	if k, ok := nav["home"]; ok {
		updateBinding(&base.Home, k)
	}
	if k, ok := nav["end"]; ok {
		updateBinding(&base.End, k)
	}
	if k, ok := nav["top"]; ok {
		updateBinding(&base.Top, k)
	}
	if k, ok := nav["bottom"]; ok {
		updateBinding(&base.Bottom, k)
	}
}

func applySelectionOverrides(base *Base, sel config.KeybindingSectionConfig) {
	if sel == nil {
		return
	}
	if k, ok := sel["select"]; ok {
		updateBinding(&base.Select, k)
	}
	if k, ok := sel["select_all"]; ok {
		updateBinding(&base.SelectAll, k)
	}
	if k, ok := sel["select_none"]; ok {
		updateBinding(&base.SelectNone, k)
	}
	if k, ok := sel["toggle_select"]; ok {
		updateBinding(&base.ToggleSelect, k)
	}
}

func applyActionsOverrides(base *Base, actions config.KeybindingSectionConfig) {
	if actions == nil {
		return
	}
	if k, ok := actions["confirm"]; ok {
		updateBinding(&base.Confirm, k)
	}
	if k, ok := actions["cancel"]; ok {
		updateBinding(&base.Cancel, k)
	}
	if k, ok := actions["back"]; ok {
		updateBinding(&base.Back, k)
	}
	if k, ok := actions["edit"]; ok {
		updateBinding(&base.Edit, k)
	}
	if k, ok := actions["delete"]; ok {
		updateBinding(&base.Delete, k)
	}
	if k, ok := actions["yank"]; ok {
		updateBinding(&base.Yank, k)
	}
}

func applySearchOverrides(base *Base, search config.KeybindingSectionConfig) {
	if search == nil {
		return
	}
	if k, ok := search["search"]; ok {
		updateBinding(&base.Search, k)
	}
	if k, ok := search["next_match"]; ok {
		updateBinding(&base.SearchNext, k)
	}
	if k, ok := search["prev_match"]; ok {
		updateBinding(&base.SearchPrev, k)
	}
	if k, ok := search["clear_search"]; ok {
		updateBinding(&base.ClearSearch, k)
	}
	if k, ok := search["grep"]; ok {
		updateBinding(&base.Grep, k)
	}
}

func applyViewOverrides(base *Base, view config.KeybindingSectionConfig) {
	if view == nil {
		return
	}
	if k, ok := view["switch_view"]; ok {
		updateBinding(&base.SwitchView, k)
	}
	if k, ok := view["next_tab"]; ok {
		updateBinding(&base.NextTab, k)
	}
	if k, ok := view["prev_tab"]; ok {
		updateBinding(&base.PrevTab, k)
	}
	if k, ok := view["focus_next"]; ok {
		updateBinding(&base.FocusNext, k)
	}
	if k, ok := view["focus_prev"]; ok {
		updateBinding(&base.FocusPrev, k)
	}
	if k, ok := view["toggle_preview"]; ok {
		updateBinding(&base.TogglePreview, k)
	}
}

func applyFoldOverrides(base *Base, fold config.KeybindingSectionConfig) {
	if fold == nil {
		return
	}
	if k, ok := fold["open"]; ok {
		updateBinding(&base.FoldOpen, k)
	}
	if k, ok := fold["close"]; ok {
		updateBinding(&base.FoldClose, k)
	}
	if k, ok := fold["toggle"]; ok {
		updateBinding(&base.FoldToggle, k)
	}
	if k, ok := fold["open_all"]; ok {
		updateBinding(&base.FoldOpenAll, k)
	}
	if k, ok := fold["close_all"]; ok {
		updateBinding(&base.FoldCloseAll, k)
	}
}

func applySystemOverrides(base *Base, sys config.KeybindingSectionConfig) {
	if sys == nil {
		return
	}
	if k, ok := sys["quit"]; ok {
		updateBinding(&base.Quit, k)
	}
	if k, ok := sys["help"]; ok {
		updateBinding(&base.Help, k)
	}
}

// applyGenericOverrides applies a flat map of overrides to any matching binding.
// This is used for TUI-specific overrides that might override any binding.
func applyGenericOverrides(base *Base, overrides config.KeybindingSectionConfig) {
	if overrides == nil {
		return
	}
	// Navigation
	if k, ok := overrides["up"]; ok {
		updateBinding(&base.Up, k)
	}
	if k, ok := overrides["down"]; ok {
		updateBinding(&base.Down, k)
	}
	if k, ok := overrides["left"]; ok {
		updateBinding(&base.Left, k)
	}
	if k, ok := overrides["right"]; ok {
		updateBinding(&base.Right, k)
	}
	if k, ok := overrides["page_up"]; ok {
		updateBinding(&base.PageUp, k)
	}
	if k, ok := overrides["page_down"]; ok {
		updateBinding(&base.PageDown, k)
	}
	if k, ok := overrides["top"]; ok {
		updateBinding(&base.Top, k)
	}
	if k, ok := overrides["bottom"]; ok {
		updateBinding(&base.Bottom, k)
	}
	// Actions
	if k, ok := overrides["confirm"]; ok {
		updateBinding(&base.Confirm, k)
	}
	if k, ok := overrides["cancel"]; ok {
		updateBinding(&base.Cancel, k)
	}
	if k, ok := overrides["back"]; ok {
		updateBinding(&base.Back, k)
	}
	if k, ok := overrides["edit"]; ok {
		updateBinding(&base.Edit, k)
	}
	if k, ok := overrides["delete"]; ok {
		updateBinding(&base.Delete, k)
	}
	if k, ok := overrides["yank"]; ok {
		updateBinding(&base.Yank, k)
	}
	// Search
	if k, ok := overrides["search"]; ok {
		updateBinding(&base.Search, k)
	}
	if k, ok := overrides["next_match"]; ok {
		updateBinding(&base.SearchNext, k)
	}
	if k, ok := overrides["prev_match"]; ok {
		updateBinding(&base.SearchPrev, k)
	}
	// Selection
	if k, ok := overrides["select"]; ok {
		updateBinding(&base.Select, k)
	}
	if k, ok := overrides["select_all"]; ok {
		updateBinding(&base.SelectAll, k)
	}
	if k, ok := overrides["select_none"]; ok {
		updateBinding(&base.SelectNone, k)
	}
	// View
	if k, ok := overrides["switch_view"]; ok {
		updateBinding(&base.SwitchView, k)
	}
	if k, ok := overrides["toggle_preview"]; ok {
		updateBinding(&base.TogglePreview, k)
	}
	// Fold
	if k, ok := overrides["fold_open"]; ok {
		updateBinding(&base.FoldOpen, k)
	}
	if k, ok := overrides["fold_close"]; ok {
		updateBinding(&base.FoldClose, k)
	}
	if k, ok := overrides["fold_toggle"]; ok {
		updateBinding(&base.FoldToggle, k)
	}
	// System
	if k, ok := overrides["quit"]; ok {
		updateBinding(&base.Quit, k)
	}
	if k, ok := overrides["help"]; ok {
		updateBinding(&base.Help, k)
	}
}

// ShortHelp returns a slice of key bindings for the short help view
func (k Base) ShortHelp() []key.Binding {
	return []key.Binding{
		k.Quit,
	}
}

// FullHelp returns a slice of all key bindings for the full help view
func (k Base) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		// Navigation
		{k.Up, k.Down, k.Left, k.Right},
		{k.PageUp, k.PageDown, k.Top, k.Bottom},
		// Actions
		{k.Confirm, k.Cancel, k.Back, k.Edit},
		{k.Delete, k.Yank},
		// Search
		{k.Search, k.SearchNext, k.SearchPrev, k.ClearSearch},
		// Selection
		{k.Select, k.SelectAll, k.SelectNone},
		// View
		{k.SwitchView, k.NextTab, k.PrevTab, k.TogglePreview},
		// Fold
		{k.FoldOpen, k.FoldClose, k.FoldToggle, k.FoldOpenAll, k.FoldCloseAll},
		// Core
		{k.Help, k.Quit},
	}
}

// DefaultKeyMap is the default keymap instance for the Grove ecosystem
var DefaultKeyMap = NewBase()

// Extended keymaps for specific use cases

// ListKeyMap extends Base with list-specific bindings
type ListKeyMap struct {
	Base
	Copy  key.Binding
	Paste key.Binding
}

// NewListKeyMap creates a new list-specific keymap
func NewListKeyMap() ListKeyMap {
	return ListKeyMap{
		Base: NewBase(),
		Copy: key.NewBinding(
			key.WithKeys("y"),
			key.WithHelp("y", "copy/yank"),
		),
		Paste: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "paste"),
		),
	}
}

// FormKeyMap extends Base with form-specific bindings
type FormKeyMap struct {
	Base
	NextField key.Binding
	PrevField key.Binding
	Submit    key.Binding
	Reset     key.Binding
}

// NewFormKeyMap creates a new form-specific keymap
func NewFormKeyMap() FormKeyMap {
	return FormKeyMap{
		Base: NewBase(),
		NextField: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next field"),
		),
		PrevField: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("S-tab", "prev field"),
		),
		Submit: key.NewBinding(
			key.WithKeys("ctrl+s", "ctrl+enter"),
			key.WithHelp("C-s", "submit"),
		),
		Reset: key.NewBinding(
			key.WithKeys("ctrl+r"),
			key.WithHelp("C-r", "reset"),
		),
	}
}

// TreeKeyMap extends Base with tree-specific bindings
type TreeKeyMap struct {
	Base
	Expand   key.Binding
	Collapse key.Binding
	Toggle   key.Binding
}

// NewTreeKeyMap creates a new tree-specific keymap
func NewTreeKeyMap() TreeKeyMap {
	return TreeKeyMap{
		Base: NewBase(),
		Expand: key.NewBinding(
			key.WithKeys("o", "right"),
			key.WithHelp("o", "expand"),
		),
		Collapse: key.NewBinding(
			key.WithKeys("c", "left"),
			key.WithHelp("c", "collapse"),
		),
		Toggle: key.NewBinding(
			key.WithKeys("space", "enter"),
			key.WithHelp("space", "toggle"),
		),
	}
}
