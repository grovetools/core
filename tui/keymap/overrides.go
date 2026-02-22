package keymap

import (
	"reflect"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/key"
	"github.com/grovetools/core/config"
)

// ApplyOverrides applies keybinding overrides from config to any KeyMap struct.
// It uses reflection to automatically map config keys (snake_case) to struct fields (CamelCase).
// Only fields of type key.Binding are processed. Embedded structs are recursively processed.
//
// Example:
//
//	km := KeyMap{ViewLogs: key.NewBinding(...), ...}
//	ApplyOverrides(&km, overrides) // overrides["view_logs"] -> km.ViewLogs
func ApplyOverrides(km interface{}, overrides config.KeybindingSectionConfig) {
	if overrides == nil {
		return
	}

	v := reflect.ValueOf(km)
	if v.Kind() != reflect.Ptr {
		return
	}
	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return
	}

	applyOverridesRecursive(v, overrides)
}

// applyOverridesRecursive applies overrides to struct fields, recursing into embedded structs.
func applyOverridesRecursive(v reflect.Value, overrides config.KeybindingSectionConfig) {
	t := v.Type()
	bindingType := reflect.TypeOf(key.Binding{})

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)

		// Skip unexported fields
		if !field.CanSet() {
			continue
		}

		// If it's an embedded struct, recurse into it
		if fieldType.Anonymous && field.Kind() == reflect.Struct {
			applyOverridesRecursive(field, overrides)
			continue
		}

		// Only process key.Binding fields
		if fieldType.Type != bindingType {
			continue
		}

		// Convert CamelCase field name to snake_case config key
		configKey := camelToSnake(fieldType.Name)

		// Look up override in config
		if keys, ok := overrides[configKey]; ok && len(keys) > 0 {
			// Get the current binding to preserve the help description
			currentBinding := field.Interface().(key.Binding)
			helpDesc := currentBinding.Help().Desc

			// Create new binding with overridden keys
			newBinding := key.NewBinding(
				key.WithKeys(keys...),
				key.WithHelp(keys[0], helpDesc),
			)
			field.Set(reflect.ValueOf(newBinding))
		}
	}
}

// ApplyTUIOverrides applies TUI-specific keybinding overrides from config.
// This is a convenience wrapper that handles the nested config lookup.
//
// Example:
//
//	km := MyKeyMap{Base: keymap.Load(cfg, "flow.status"), ...}
//	keymap.ApplyTUIOverrides(cfg, "flow", "status", &km)
func ApplyTUIOverrides(cfg *config.Config, pkg, tui string, km interface{}) {
	if cfg == nil || cfg.TUI == nil || cfg.TUI.Keybindings == nil {
		return
	}
	tuiOverrides := cfg.TUI.Keybindings.GetTUIOverrides()
	if pkgOverrides, ok := tuiOverrides[pkg]; ok {
		if overrides, ok := pkgOverrides[tui]; ok {
			ApplyOverrides(km, overrides)
		}
	}
}

// camelToSnake converts a CamelCase string to snake_case.
// Examples: ViewLogs -> view_logs, GoToTop -> go_to_top, HTTPServer -> http_server
func camelToSnake(s string) string {
	var result strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				result.WriteRune('_')
			}
			result.WriteRune(unicode.ToLower(r))
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}
