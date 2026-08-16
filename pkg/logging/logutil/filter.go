package logutil

import "strings"

// PassesEventsFilter reports whether a log entry belongs in the lifecycle
// events view. Structured events always pass; otherwise warning-and-higher
// entries pass so operational failures are never hidden by events-only mode.
func PassesEventsFilter(eventValue interface{}, level string) bool {
	if event, ok := eventValue.(string); ok && event != "" {
		return true
	}
	switch strings.ToLower(level) {
	case "warn", "warning", "error", "fatal", "panic":
		return true
	default:
		return false
	}
}
