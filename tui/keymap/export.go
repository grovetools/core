package keymap

import (
	"reflect"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/key"
)

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
	Name        string   // Binding identity: struct field name, e.g., "Up", "Confirm" (falls back to Help().Desc for bindings not backed by a struct field)
	Keys        []string // Key combinations, e.g., ["j", "down"]
	Description string   // Human-readable description
	Enabled     bool     // Whether the binding is active
	ConfigKey   string   // Configuration key for grove.toml override (snake_case field name)
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
//
// Binding identity is derived from the keymap's struct FIELD NAMES, not from
// help descriptions (which may collide across fields): each exported binding
// is matched back to its struct field by signature (Keys() + Help(), see
// bindingSignature), then Name is set to the field name (e.g. "PageUp") and
// ConfigKey to its snake_case form (e.g. "page_up") using the same
// CamelCase->snake_case conversion as the overrides system, so ConfigKey
// round-trips with ApplyOverrides. Description remains Help().Desc.
//
// Bindings that cannot be matched to a field (e.g. constructed inline in a
// Sections() method) fall back to the legacy description-derived Name and
// ConfigKey.
func MakeTUIInfo(name, pkg, description string, km SectionedKeyMap) TUIInfo {
	info := TUIInfo{
		Name:        name,
		Package:     pkg,
		Description: description,
		Sections:    ExportSections(km.Sections()),
	}

	// Map binding signatures to struct field names via reflection. On a
	// signature collision (two fields with identical keys AND help) the
	// first field in declaration order wins, keeping output deterministic.
	fieldBySig := make(map[string]string)
	for _, f := range collectBindingFields(reflect.ValueOf(km), "") {
		sig := bindingSignature(f.Binding)
		if _, ok := fieldBySig[sig]; !ok {
			fieldBySig[sig] = f.Name
		}
	}

	// ExportSections preserves section/binding order, so indices align with
	// km.Sections() and each exported binding can be matched by signature.
	sections := km.Sections()
	for i := range info.Sections {
		for j := range info.Sections[i].Bindings {
			b := &info.Sections[i].Bindings[j]
			if fieldName, ok := fieldBySig[bindingSignature(sections[i].Bindings[j])]; ok {
				b.Name = fieldName
				b.ConfigKey = camelToSnake(fieldName)
				continue
			}
			// Fallback: derive ConfigKey from the description (legacy behavior).
			b.ConfigKey = toSnakeCase(strings.ReplaceAll(b.Description, " ", "_"))
		}
	}

	return info
}

// toSnakeCase converts a PascalCase or camelCase string to snake_case.
func toSnakeCase(s string) string {
	var sb strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				prev := rune(s[i-1])
				// Add underscore before uppercase if previous is lowercase
				// or if previous is uppercase and next is lowercase (for acronyms)
				if unicode.IsLower(prev) {
					sb.WriteRune('_')
				} else if i+1 < len(s) && unicode.IsLower(rune(s[i+1])) {
					sb.WriteRune('_')
				}
			}
			sb.WriteRune(unicode.ToLower(r))
		} else if r == ' ' || r == '-' {
			sb.WriteRune('_')
		} else {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}
