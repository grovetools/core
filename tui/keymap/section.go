package keymap

import "github.com/charmbracelet/bubbles/key"

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
