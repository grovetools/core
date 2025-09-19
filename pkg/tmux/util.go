package tmux

import "strings"

// SanitizeForTmuxSession creates a valid tmux session name from a string.
// It replaces spaces and special characters with hyphens, converts to lowercase,
// and ensures the name is a reasonable length.
func SanitizeForTmuxSession(title string) string {
	// Replace spaces and special characters with hyphens
	sanitized := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, title)

	// Convert to lowercase for consistency
	sanitized = strings.ToLower(sanitized)

	// Remove consecutive hyphens
	for strings.Contains(sanitized, "--") {
		sanitized = strings.ReplaceAll(sanitized, "--", "-")
	}

	// Trim hyphens from start and end
	sanitized = strings.Trim(sanitized, "-")

	// Ensure it's not empty
	if sanitized == "" {
		sanitized = "session"
	}

	// Tmux session names should not be too long
	if len(sanitized) > 50 {
		sanitized = sanitized[:50]
	}

	return sanitized
}