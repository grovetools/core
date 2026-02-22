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
	Name        string   // Action name, e.g., "up", "confirm"
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
func MakeTUIInfo(name, pkg, description string, km SectionedKeyMap) TUIInfo {
	info := TUIInfo{
		Name:        name,
		Package:     pkg,
		Description: description,
		Sections:    ExportSections(km.Sections()),
	}

	// Extract config keys via reflection on the keymap struct
	configKeys := make(map[string]string)
	extractConfigKeys(reflect.ValueOf(km), configKeys)

	// Map config keys back to the bindings by matching help descriptions
	for i := range info.Sections {
		for j := range info.Sections[i].Bindings {
			desc := info.Sections[i].Bindings[j].Description
			if cKey, ok := configKeys[desc]; ok {
				info.Sections[i].Bindings[j].ConfigKey = cKey
			} else {
				// Fallback: convert description to snake_case
				info.Sections[i].Bindings[j].ConfigKey = toSnakeCase(strings.ReplaceAll(desc, " ", "_"))
			}
		}
	}

	return info
}

// extractConfigKeys uses reflection to find all key.Binding fields in a struct
// and maps their help description to their snake_cased field name.
func extractConfigKeys(v reflect.Value, m map[string]string) {
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() == reflect.Interface {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return
	}
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		val := v.Field(i)

		// Handle embedded/anonymous fields by recursing
		if field.Anonymous {
			extractConfigKeys(val, m)
			continue
		}

		// Check if this is a key.Binding field
		if field.Type.String() == "key.Binding" {
			if val.CanInterface() {
				binding, ok := val.Interface().(key.Binding)
				if ok && binding.Help().Desc != "" {
					m[binding.Help().Desc] = toSnakeCase(field.Name)
				}
			}
			continue
		}

		// Recurse into nested structs (but not slices/maps)
		if val.Kind() == reflect.Struct {
			extractConfigKeys(val, m)
		}
	}
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
