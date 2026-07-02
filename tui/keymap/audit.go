package keymap

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/charmbracelet/bubbles/key"
)

// Gap kinds reported by AuditCoverage.
const (
	GapMissingFromSections = "missing-from-sections"
	GapHelpKeyMismatch     = "help-key-mismatch"
	GapEmptyHelp           = "empty-help"
)

// Gap describes a single keymap coverage problem found by AuditCoverage.
type Gap struct {
	Field  string // struct field path, e.g. "Base.Grep" or "ViewGit"
	Kind   string // "missing-from-sections" | "help-key-mismatch" | "empty-help"
	Detail string
}

// AuditCoverage reflects over every key.Binding field of the concrete keymap
// struct behind km (recursing into embedded structs such as Base, and
// following pointers) and reports coverage gaps:
//
//   - "missing-from-sections": an enabled binding with keys that appears in
//     none of km.Sections(). Bindings are matched by signature — the exact
//     Keys() slice plus the Help() key/desc pair — because key.Binding is not
//     comparable. Two distinct fields with identical keys AND identical help
//     are therefore treated as the same binding, which is the desired
//     behavior for aliased fields.
//   - "help-key-mismatch": a Help().Key label that clearly names a different
//     key than any of Keys(). Only "simple" labels are checked (no "/" or
//     spaces, so alternate lists like "q/ctrl+c" and synthetic labels like
//     "g + 1-9" never false-positive), and labels are normalized before
//     comparison (case-insensitive; "C-u" == "ctrl+u", "space" == " ", etc.).
//   - "empty-help": an enabled binding with keys but an empty Help().Desc —
//     such a binding is silently dropped from help rendering.
//
// Disabled bindings and bindings with no keys are skipped entirely.
func AuditCoverage(km SectionedKeyMap) []Gap {
	var gaps []Gap

	// Build the set of binding signatures reachable via Sections().
	inSections := make(map[string]bool)
	for _, s := range km.Sections() {
		for _, b := range s.Bindings {
			inSections[bindingSignature(b)] = true
		}
	}

	for _, f := range collectBindingFields(reflect.ValueOf(km), "") {
		b := f.Binding
		if !b.Enabled() || len(b.Keys()) == 0 {
			continue
		}

		if !inSections[bindingSignature(b)] {
			gaps = append(gaps, Gap{
				Field:  f.Path,
				Kind:   GapMissingFromSections,
				Detail: fmt.Sprintf("binding %v (%q) appears in no section", b.Keys(), b.Help().Desc),
			})
		}

		if b.Help().Desc == "" {
			gaps = append(gaps, Gap{
				Field:  f.Path,
				Kind:   GapEmptyHelp,
				Detail: fmt.Sprintf("binding %v has keys but no help description; it will be dropped from help rendering", b.Keys()),
			})
		}

		if label := b.Help().Key; label != "" && isSimpleKeyLabel(label) && !labelMatchesAnyKey(label, b.Keys()) {
			gaps = append(gaps, Gap{
				Field:  f.Path,
				Kind:   GapHelpKeyMismatch,
				Detail: fmt.Sprintf("help label %q matches none of keys %v", label, b.Keys()),
			})
		}
	}

	return gaps
}

// bindingSignature returns a stable identity string for a key.Binding.
// key.Binding is not comparable, so bindings are matched by their Keys()
// slice plus Help() key/desc pair. Used by AuditCoverage and MakeTUIInfo.
func bindingSignature(b key.Binding) string {
	h := b.Help()
	return strings.Join(b.Keys(), "\x00") + "\x1f" + h.Key + "\x1f" + h.Desc
}

// fieldBinding is a key.Binding struct field discovered via reflection.
type fieldBinding struct {
	Path    string // full field path, e.g. "Base.Grep"
	Name    string // leaf field name, e.g. "Grep"
	Binding key.Binding
}

// collectBindingFields walks a keymap struct (following pointers and
// interfaces) and returns all key.Binding fields in declaration order,
// recursing into embedded and nested structs. prefix is prepended to the
// field path, so fields of an embedded Base surface as "Base.Up" etc.
func collectBindingFields(v reflect.Value, prefix string) []fieldBinding {
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}

	bindingType := reflect.TypeOf(key.Binding{})
	var out []fieldBinding
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		ft := t.Field(i)
		fv := v.Field(i)

		path := ft.Name
		if prefix != "" {
			path = prefix + "." + ft.Name
		}

		if ft.Type == bindingType {
			if !fv.CanInterface() {
				continue
			}
			out = append(out, fieldBinding{Path: path, Name: ft.Name, Binding: fv.Interface().(key.Binding)})
			continue
		}

		// Recurse into embedded structs (e.g. Base) and named nested structs.
		k := fv.Kind()
		if k == reflect.Struct || ((k == reflect.Ptr || k == reflect.Interface) && !fv.IsNil()) {
			out = append(out, collectBindingFields(fv, path)...)
		}
	}
	return out
}

// isSimpleKeyLabel reports whether a help label names a single concrete key.
// Labels containing "/" or spaces are alternates or synthetic descriptions
// (e.g. "q/ctrl+c", "g + 1-9", "space/enter") and are never audited.
func isSimpleKeyLabel(label string) bool {
	return !strings.ContainsAny(label, "/ ")
}

// labelMatchesAnyKey reports whether a normalized help label equals any of
// the binding's normalized keys.
func labelMatchesAnyKey(label string, keys []string) bool {
	norm := normalizeKeyLabel(label)
	for _, k := range keys {
		if norm == normalizeKeyLabel(k) {
			return true
		}
	}
	return false
}

// normalizeKeyLabel canonicalizes a key name or help label for comparison:
// lowercases, expands display abbreviations ("C-u" -> "ctrl+u", "M-<" ->
// "alt+<", "S-tab" -> "shift+tab"), and maps common display aliases
// ("space" -> " ", "del" -> "delete", "pgdn" -> "pgdown", arrows).
func normalizeKeyLabel(s string) string {
	switch s {
	case "←":
		return "left"
	case "→":
		return "right"
	case "↑":
		return "up"
	case "↓":
		return "down"
	case " ":
		// The literal space key — must not be trimmed away.
		return " "
	}

	s = strings.ToLower(strings.TrimSpace(s))

	// Modifier display abbreviations.
	switch {
	case strings.HasPrefix(s, "c-"):
		s = "ctrl+" + s[len("c-"):]
	case strings.HasPrefix(s, "m-"):
		s = "alt+" + s[len("m-"):]
	case strings.HasPrefix(s, "s-"):
		s = "shift+" + s[len("s-"):]
	}

	// Common display aliases.
	switch s {
	case "space", "spc":
		return " "
	case "del":
		return "delete"
	case "pgdn":
		return "pgdown"
	case "return":
		return "enter"
	}
	return s
}
