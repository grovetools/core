package keymap

import "github.com/charmbracelet/bubbles/key"

// TUIInfo describes a TUI's keybindings for the keys registry.
// Each TUI in the ecosystem exports its keymap metadata via a KeymapInfo() function.
type TUIInfo struct {
	Name        string        // e.g., "flow-status", "nb-browser"
	Package     string        // e.g., "flow", "nb"
	Description string        // Human-readable description
	Sections    []SectionInfo // All keybinding sections
}

// SectionInfo is a serializable representation of a keybinding section.
type SectionInfo struct {
	Name     string        // e.g., "Navigation", "Actions"
	Bindings []BindingInfo // All bindings in this section
}

// BindingInfo is a serializable representation of a single keybinding.
type BindingInfo struct {
	Name        string   // Action name, e.g., "up", "confirm"
	Keys        []string // Key combinations, e.g., ["j", "down"]
	Description string   // Human-readable description
	Enabled     bool     // Whether the binding is active
}

// ExportSection converts a Section to a serializable SectionInfo.
func ExportSection(s Section) SectionInfo {
	bindings := make([]BindingInfo, 0, len(s.Bindings))
	for _, b := range s.Bindings {
		bindings = append(bindings, ExportBinding(b))
	}
	return SectionInfo{
		Name:     s.Name,
		Bindings: bindings,
	}
}

// ExportBinding converts a key.Binding to a serializable BindingInfo.
func ExportBinding(b key.Binding) BindingInfo {
	return BindingInfo{
		Name:        b.Help().Desc,
		Keys:        b.Keys(),
		Description: b.Help().Desc,
		Enabled:     b.Enabled(),
	}
}

// ExportSections converts a slice of Section to serializable SectionInfo.
func ExportSections(sections []Section) []SectionInfo {
	result := make([]SectionInfo, 0, len(sections))
	for _, s := range sections {
		result = append(result, ExportSection(s))
	}
	return result
}

// MakeTUIInfo creates a TUIInfo from a SectionedKeyMap.
// This is the standard way for TUIs to export their keybindings.
func MakeTUIInfo(name, pkg, description string, km SectionedKeyMap) TUIInfo {
	return TUIInfo{
		Name:        name,
		Package:     pkg,
		Description: description,
		Sections:    ExportSections(km.Sections()),
	}
}
