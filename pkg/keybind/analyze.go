package keybind

import (
	"fmt"
	"sort"
	"strings"
)

// Available finds keys that are unbound at the specified layers.
// If no layers are specified, checks all layers.
func (s *Stack) Available(layers ...Layer) []string {
	if len(layers) == 0 {
		layers = []Layer{
			LayerOS,
			LayerTerminal,
			LayerShell,
			LayerTmuxRoot,
			LayerTmuxPrefix,
			LayerTmuxCustomTable,
			LayerApplication,
		}
	}

	// Collect all bound keys at the specified layers
	boundKeys := make(map[string]bool)
	for _, layer := range layers {
		for _, binding := range s.Layers[layer] {
			normalizedKey := Normalize(binding.Key, binding.Source)
			boundKeys[normalizedKey] = true
		}
	}

	// Generate all possible keys and filter out bound ones
	allKeys := generateAllKeys()
	available := make([]string, 0)
	for _, key := range allKeys {
		if !boundKeys[key] {
			available = append(available, key)
		}
	}

	return available
}

// AvailableInTable finds keys that are unbound in a specific tmux custom table.
func (s *Stack) AvailableInTable(tableName string) []string {
	boundKeys := make(map[string]bool)
	for _, binding := range s.CustomTables[tableName] {
		normalizedKey := Normalize(binding.Key, binding.Source)
		boundKeys[normalizedKey] = true
	}

	// For tables, typically only single keys and simple modifiers are used
	allKeys := generateTableKeys()
	available := make([]string, 0)
	for _, key := range allKeys {
		if !boundKeys[key] {
			available = append(available, key)
		}
	}

	return available
}

// Conflicts detects bindings that conflict across layers.
// A conflict occurs when a key is bound at a layer that shadows a lower-priority layer.
func (s *Stack) Conflicts() []Conflict {
	// Group bindings by normalized key
	keyBindings := make(map[string][]Binding)
	for _, bindings := range s.Layers {
		for _, b := range bindings {
			normalizedKey := Normalize(b.Key, b.Source)
			keyBindings[normalizedKey] = append(keyBindings[normalizedKey], b)
		}
	}

	var conflicts []Conflict
	for key, bindings := range keyBindings {
		if len(bindings) < 2 {
			continue
		}

		// Sort by layer (lower layer = higher priority = shadows others)
		sort.Slice(bindings, func(i, j int) bool {
			return bindings[i].Layer < bindings[j].Layer
		})

		conflict := analyzeConflict(key, bindings)
		if conflict != nil {
			conflicts = append(conflicts, *conflict)
		}
	}

	// Sort conflicts by severity (errors first) then by key
	sort.Slice(conflicts, func(i, j int) bool {
		if conflicts[i].Severity != conflicts[j].Severity {
			return conflicts[i].Severity > conflicts[j].Severity
		}
		return conflicts[i].Key < conflicts[j].Key
	})

	return conflicts
}

// analyzeConflict determines the conflict severity and description.
func analyzeConflict(key string, bindings []Binding) *Conflict {
	if len(bindings) < 2 {
		return nil
	}

	// Find the shadowing relationship
	shadowingLayer := bindings[0].Layer
	shadowedLayers := make([]Layer, 0)
	for i := 1; i < len(bindings); i++ {
		shadowedLayers = append(shadowedLayers, bindings[i].Layer)
	}

	// Determine severity based on conflict type
	severity := SeverityInfo
	var description string

	switch {
	case shadowingLayer == LayerShell && containsLayer(shadowedLayers, LayerTmuxRoot):
		// Shell shadowing tmux is a warning - common and often intentional
		severity = SeverityWarning
		description = fmt.Sprintf("Shell binding '%s' shadows tmux root binding", bindings[0].Action)

	case shadowingLayer == LayerOS:
		// OS shortcuts can't be overridden - just informational
		severity = SeverityInfo
		description = "OS shortcut - cannot be overridden by terminal apps"

	case shadowingLayer == LayerTmuxRoot && containsLayer(shadowedLayers, LayerShell):
		// Tmux shadowing shell is typically intentional
		severity = SeverityInfo
		description = fmt.Sprintf("Tmux root binding shadows shell readline '%s'", bindings[1].Action)

	default:
		// Other conflicts might be problematic
		if isProblematicConflict(bindings) {
			severity = SeverityWarning
			description = fmt.Sprintf("%s binding shadows %s binding",
				bindings[0].Layer.String(), bindings[1].Layer.String())
		} else {
			severity = SeverityInfo
			description = "Multiple bindings for same key at different layers"
		}
	}

	return &Conflict{
		Key:         key,
		Bindings:    bindings,
		Severity:    severity,
		Description: description,
	}
}

// isProblematicConflict checks if the conflict will cause user confusion.
func isProblematicConflict(bindings []Binding) bool {
	// Check if both bindings are from user configs (not defaults)
	userConfigCount := 0
	for _, b := range bindings {
		if b.Provenance == ProvenanceUserConfig || b.Provenance == ProvenanceGrove {
			userConfigCount++
		}
	}
	return userConfigCount > 1
}

// containsLayer checks if a layer is in the list.
func containsLayer(layers []Layer, target Layer) bool {
	for _, l := range layers {
		if l == target {
			return true
		}
	}
	return false
}

// generateAllKeys generates a comprehensive list of possible key combinations.
func generateAllKeys() []string {
	var keys []string

	// Single letters
	for c := 'A'; c <= 'Z'; c++ {
		keys = append(keys, string(c))
	}

	// Numbers
	for c := '0'; c <= '9'; c++ {
		keys = append(keys, string(c))
	}

	// Function keys
	for i := 1; i <= 12; i++ {
		keys = append(keys, fmt.Sprintf("F%d", i))
	}

	// Control + letter
	for c := 'A'; c <= 'Z'; c++ {
		keys = append(keys, "C-"+string(c))
	}

	// Meta/Alt + letter
	for c := 'A'; c <= 'Z'; c++ {
		keys = append(keys, "M-"+string(c))
	}

	// Control + Meta + letter
	for c := 'A'; c <= 'Z'; c++ {
		keys = append(keys, "C-M-"+string(c))
	}

	// Special keys
	specialKeys := []string{
		"Enter", "Escape", "Tab", "Space", "Backspace", "Delete",
		"Up", "Down", "Left", "Right", "Home", "End", "PageUp", "PageDown",
	}
	keys = append(keys, specialKeys...)

	// Control + special keys
	for _, special := range specialKeys {
		keys = append(keys, "C-"+special)
	}

	// Meta + special keys
	for _, special := range specialKeys {
		keys = append(keys, "M-"+special)
	}

	return keys
}

// generateTableKeys generates keys typically used in tmux custom tables.
func generateTableKeys() []string {
	var keys []string

	// Single letters (most common in tables)
	for c := 'a'; c <= 'z'; c++ {
		keys = append(keys, strings.ToUpper(string(c)))
	}

	// Numbers
	for c := '0'; c <= '9'; c++ {
		keys = append(keys, string(c))
	}

	// Common special keys in tables
	keys = append(keys, "Enter", "Escape", "Tab", "Space", "?")

	return keys
}

// SuggestAlternatives finds unbound keys similar to the requested one.
func (s *Stack) SuggestAlternatives(key string, count int, layers ...Layer) []string {
	normalizedKey := Normalize(key, "")
	suggestions := make([]string, 0)

	// Extract the base key (letter/number) and modifiers
	base, mods := parseModifiers(normalizedKey)

	// Try different modifier combinations
	modifierCombos := []string{"", "C-", "M-", "C-M-", "S-"}
	for _, mod := range modifierCombos {
		if mod == mods {
			continue // Skip the same modifier
		}
		candidate := mod + base
		if !s.isKeyBound(candidate, layers...) {
			suggestions = append(suggestions, candidate)
			if len(suggestions) >= count {
				return suggestions
			}
		}
	}

	// Also suggest nearby letters
	if len(base) == 1 && base[0] >= 'A' && base[0] <= 'Z' {
		nearby := []byte{base[0] - 1, base[0] + 1}
		for _, b := range nearby {
			if b >= 'A' && b <= 'Z' {
				candidate := mods + string(b)
				if !s.isKeyBound(candidate, layers...) {
					suggestions = append(suggestions, candidate)
					if len(suggestions) >= count {
						return suggestions
					}
				}
			}
		}
	}

	return suggestions
}

// parseModifiers extracts modifiers and base key from a normalized key.
func parseModifiers(key string) (base, mods string) {
	if strings.HasPrefix(key, "C-M-") {
		return key[4:], "C-M-"
	}
	if strings.HasPrefix(key, "C-") {
		return key[2:], "C-"
	}
	if strings.HasPrefix(key, "M-") {
		return key[2:], "M-"
	}
	if strings.HasPrefix(key, "S-") {
		return key[2:], "S-"
	}
	return key, ""
}

// isKeyBound checks if a key is bound at the specified layers.
func (s *Stack) isKeyBound(key string, layers ...Layer) bool {
	if len(layers) == 0 {
		layers = []Layer{LayerShell, LayerTmuxRoot, LayerTmuxPrefix, LayerTmuxCustomTable}
	}

	normalizedKey := Normalize(key, "")
	for _, layer := range layers {
		for _, binding := range s.Layers[layer] {
			if Normalize(binding.Key, binding.Source) == normalizedKey {
				return true
			}
		}
	}
	return false
}

// FindBindingForKey returns the binding for a key at the first matching layer.
func (s *Stack) FindBindingForKey(key string) *Binding {
	normalizedKey := Normalize(key, "")

	// Check layers in order
	layerOrder := []Layer{
		LayerOS,
		LayerTerminal,
		LayerShell,
		LayerTmuxRoot,
		LayerTmuxPrefix,
		LayerTmuxCustomTable,
		LayerApplication,
	}

	for _, layer := range layerOrder {
		for _, binding := range s.Layers[layer] {
			if Normalize(binding.Key, binding.Source) == normalizedKey {
				return &binding
			}
		}
	}

	return nil
}
