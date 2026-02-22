package keymap

import "github.com/charmbracelet/bubbles/key"

// Standard section names - use these for consistency across all TUIs.
// Using these constants ensures the keys registry and help displays are uniform.
const (
	SectionNavigation = "Navigation"
	SectionActions    = "Actions"
	SectionSearch     = "Search"
	SectionSelection  = "Selection"
	SectionView       = "View"
	SectionFold       = "Fold"
	SectionSystem     = "System"
)

// Section represents a logical grouping of keybindings for structured help display.
// Sections provide a first-class way to organize keybindings into categories like
// "Navigation", "Actions", "Search", etc. This replaces the previous ad-hoc approach
// of using empty key.Binding as section headers.
type Section struct {
	Name     string
	Bindings []key.Binding
}

// SectionedKeyMap is an interface for keymaps that organize their bindings into sections.
// TUIs implementing this interface get proper section-based help rendering instead of
// the legacy FullHelp() approach.
type SectionedKeyMap interface {
	Sections() []Section
}

// --- Section Builder Functions ---
// Use these to create sections with standard names, selecting only the bindings you need.
// This ensures consistent section naming across all TUIs while allowing each TUI to
// include only the bindings it actually implements.
//
// Example usage in a TUI's Sections() method:
//
//	func (k MyKeyMap) Sections() []keymap.Section {
//	    return []keymap.Section{
//	        keymap.NavigationSection(k.Up, k.Down, k.PageUp, k.PageDown),
//	        keymap.ActionsSection(k.Edit, k.Delete, k.CopyPath),
//	        keymap.NewSection("MyFeature", k.CustomKey1, k.CustomKey2),
//	        keymap.SystemSection(k.Help, k.Quit),
//	    }
//	}

// NewSection creates a section with a custom name.
// Use this for TUI-specific sections that don't fit the standard categories.
func NewSection(name string, bindings ...key.Binding) Section {
	return Section{Name: name, Bindings: bindings}
}

// NavigationSection creates a Navigation section with the specified bindings.
// Common bindings: Up, Down, Left, Right, PageUp, PageDown, Top, Bottom
func NavigationSection(bindings ...key.Binding) Section {
	return Section{Name: SectionNavigation, Bindings: bindings}
}

// ActionsSection creates an Actions section with the specified bindings.
// Common bindings: Confirm, Cancel, Back, Edit, Delete, Yank, Rename, Refresh, CopyPath
func ActionsSection(bindings ...key.Binding) Section {
	return Section{Name: SectionActions, Bindings: bindings}
}

// SearchSection creates a Search section with the specified bindings.
// Common bindings: Search, SearchNext, SearchPrev, ClearSearch, Grep
func SearchSection(bindings ...key.Binding) Section {
	return Section{Name: SectionSearch, Bindings: bindings}
}

// SelectionSection creates a Selection section with the specified bindings.
// Common bindings: Select, SelectAll, SelectNone
func SelectionSection(bindings ...key.Binding) Section {
	return Section{Name: SectionSelection, Bindings: bindings}
}

// ViewSection creates a View section with the specified bindings.
// Common bindings: SwitchView, NextTab, PrevTab, TogglePreview
func ViewSection(bindings ...key.Binding) Section {
	return Section{Name: SectionView, Bindings: bindings}
}

// FoldSection creates a Fold section with the specified bindings.
// Common bindings: FoldOpen, FoldClose, FoldToggle, FoldOpenAll, FoldCloseAll
func FoldSection(bindings ...key.Binding) Section {
	return Section{Name: SectionFold, Bindings: bindings}
}

// SystemSection creates a System section with the specified bindings.
// Common bindings: Help, Quit
func SystemSection(bindings ...key.Binding) Section {
	return Section{Name: SectionSystem, Bindings: bindings}
}

// --- Section Methods ---

// FilterEnabled returns a new slice containing only enabled bindings.
func (s Section) FilterEnabled() []key.Binding {
	var result []key.Binding
	for _, b := range s.Bindings {
		if b.Enabled() {
			result = append(result, b)
		}
	}
	return result
}

// IsEmpty returns true if the section has no enabled bindings.
func (s Section) IsEmpty() bool {
	for _, b := range s.Bindings {
		if b.Enabled() {
			return false
		}
	}
	return true
}

// With returns a new section with additional bindings appended.
// Useful for extending a base section with custom bindings.
func (s Section) With(bindings ...key.Binding) Section {
	combined := make([]key.Binding, len(s.Bindings), len(s.Bindings)+len(bindings))
	copy(combined, s.Bindings)
	combined = append(combined, bindings...)
	return Section{Name: s.Name, Bindings: combined}
}
