package keybind

import (
	"strings"
)

// Trace simulates a key sequence through the layer stack and returns the traversal path.
// For single keys, it checks each layer from L0 to L6.
// For sequences (e.g., "C-g p"), it handles state transitions between tables.
func (s *Stack) Trace(keys ...string) *Trace {
	if len(keys) == 0 {
		return &Trace{
			Keys:        keys,
			Steps:       nil,
			FinalResult: "No keys provided",
		}
	}

	trace := &Trace{
		Keys:  keys,
		Steps: make([]TraceStep, 0),
	}

	// Track current context (which table we're in after key transitions)
	currentTable := ""
	consumed := false

	for i, key := range keys {
		normalizedKey := Normalize(key, "")

		// After a table transition, check the custom table first
		if currentTable != "" {
			step := s.traceInTable(normalizedKey, currentTable)
			trace.Steps = append(trace.Steps, step)

			if step.Result == TraceConsumed {
				consumed = true
				trace.FinalResult = formatConsumedResult(step.Binding)
				// Reset table for subsequent keys
				currentTable = ""
				continue
			}
			if step.Result == TraceEntersTable {
				currentTable = step.NextTable
				continue
			}
			// If not found in table, fall back to root behavior
			currentTable = ""
		}

		// Check layers in order: L0 -> L6
		layerOrder := []Layer{
			LayerOS,
			LayerTerminal,
			LayerShell,
			LayerTmuxRoot,
			LayerTmuxPrefix,
			LayerTmuxCustomTable,
			LayerApplication,
		}

		keyConsumed := false
		for _, layer := range layerOrder {
			step := s.traceAtLayer(normalizedKey, layer, currentTable)
			trace.Steps = append(trace.Steps, step)

			if step.Result == TraceConsumed {
				consumed = true
				keyConsumed = true
				trace.FinalResult = formatConsumedResult(step.Binding)
				break
			}
			if step.Result == TraceEntersTable {
				currentTable = step.NextTable
				keyConsumed = true
				if i == len(keys)-1 {
					trace.FinalResult = "Entered table: " + step.NextTable
				}
				break
			}
		}

		// If key wasn't consumed, mark remaining layers as not reached
		if !keyConsumed {
			trace.FinalResult = "Key not bound: " + normalizedKey
		}
	}

	if !consumed && currentTable != "" {
		trace.FinalResult = "In table: " + currentTable + " (waiting for key)"
	}

	return trace
}

// traceAtLayer checks if a key is bound at a specific layer.
func (s *Stack) traceAtLayer(key string, layer Layer, currentTable string) TraceStep {
	step := TraceStep{
		Key:    key,
		Layer:  layer,
		Result: TracePassthrough,
	}

	// For custom table layer, we need the table context
	if layer == LayerTmuxCustomTable {
		if currentTable == "" {
			// Check all custom tables for this key
			for tableName, bindings := range s.CustomTables {
				for _, b := range bindings {
					if Normalize(b.Key, b.Source) == key {
						step.TableName = tableName
						step.Result = TraceConsumed
						step.Binding = &b
						return step
					}
				}
			}
		} else {
			return s.traceInTable(key, currentTable)
		}
		return step
	}

	// Check bindings at this layer
	bindings := s.Layers[layer]
	for _, b := range bindings {
		if Normalize(b.Key, b.Source) == key {
			step.Result = TraceConsumed
			step.Binding = &b

			// Check if this binding enters a table
			if isTableTransition(b.Action) {
				step.Result = TraceEntersTable
				step.NextTable = extractTableName(b.Action)
			}
			return step
		}
	}

	return step
}

// traceInTable checks if a key is bound in a specific custom table.
func (s *Stack) traceInTable(key string, tableName string) TraceStep {
	step := TraceStep{
		Key:       key,
		Layer:     LayerTmuxCustomTable,
		TableName: tableName,
		Result:    TracePassthrough,
	}

	bindings := s.CustomTables[tableName]
	for _, b := range bindings {
		if Normalize(b.Key, b.Source) == key {
			step.Result = TraceConsumed
			step.Binding = &b

			// Check for nested table transition
			if isTableTransition(b.Action) {
				step.Result = TraceEntersTable
				step.NextTable = extractTableName(b.Action)
			}
			return step
		}
	}

	return step
}

// isTableTransition checks if an action transitions to a tmux table.
func isTableTransition(action string) bool {
	return strings.Contains(action, "switch-client -T")
}

// extractTableName extracts the table name from a switch-client command.
func extractTableName(action string) string {
	// Look for "switch-client -T <tablename>"
	if idx := strings.Index(action, "switch-client -T "); idx >= 0 {
		rest := action[idx+len("switch-client -T "):]
		// Table name is until next space or end
		if spaceIdx := strings.Index(rest, " "); spaceIdx >= 0 {
			return rest[:spaceIdx]
		}
		return strings.TrimSpace(rest)
	}
	return ""
}

// formatConsumedResult formats the result for a consumed key.
func formatConsumedResult(b *Binding) string {
	if b == nil {
		return "Consumed (unknown binding)"
	}
	if b.Description != "" {
		return b.Description
	}
	if b.Action != "" {
		return b.Action
	}
	return "Consumed by " + b.Source
}

// TraceSummary returns a human-readable summary of the trace.
func (t *Trace) Summary() string {
	if len(t.Steps) == 0 {
		return "No trace steps"
	}

	var parts []string
	for _, step := range t.Steps {
		switch step.Result {
		case TraceConsumed:
			parts = append(parts, step.Layer.ShortName()+" ("+step.Layer.String()+"): ✓ "+step.Binding.Action)
		case TraceEntersTable:
			parts = append(parts, step.Layer.ShortName()+" ("+step.Layer.String()+"): → enters "+step.NextTable)
		case TracePassthrough:
			parts = append(parts, step.Layer.ShortName()+" ("+step.Layer.String()+"): → passthrough")
		case TraceNotReached:
			parts = append(parts, step.Layer.ShortName()+" ("+step.Layer.String()+"): ─ (not reached)")
		}
	}

	return strings.Join(parts, "\n")
}

// ConsumedAt returns the layer where the key was consumed, or -1 if not consumed.
func (t *Trace) ConsumedAt() Layer {
	for _, step := range t.Steps {
		if step.Result == TraceConsumed {
			return step.Layer
		}
	}
	return Layer(-1)
}

// WasConsumed returns true if the key sequence was consumed by some layer.
func (t *Trace) WasConsumed() bool {
	for _, step := range t.Steps {
		if step.Result == TraceConsumed {
			return true
		}
	}
	return false
}
