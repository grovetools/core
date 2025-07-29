package sanitize

import (
	"regexp"
	"strings"
)

var (
	// dockerLabelReplacer handles common replacements for Docker labels
	dockerLabelReplacer = strings.NewReplacer(
		".", "_",
		"-", "_",
		" ", "_",
	)

	// domainPartReplacer handles replacements for domain parts
	domainPartReplacer = strings.NewReplacer(
		"_", "-",
		" ", "-",
		".", "-",
	)

	// nonAlphanumericRegex matches non-alphanumeric characters
	nonAlphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

	// multiDashRegex matches multiple consecutive dashes
	multiDashRegex = regexp.MustCompile(`-+`)

	// multiUnderscoreRegex matches multiple consecutive underscores
	multiUnderscoreRegex = regexp.MustCompile(`_+`)
)

// ForDockerLabel sanitizes a string for use as a Docker label
// Docker labels must contain only alphanumeric characters, periods, hyphens, and underscores
func ForDockerLabel(s string) string {
	if s == "" {
		return ""
	}

	// Replace common separators with underscores
	s = dockerLabelReplacer.Replace(s)

	// Remove any remaining non-alphanumeric characters
	s = nonAlphanumericRegex.ReplaceAllString(s, "_")

	// Collapse multiple underscores
	s = multiUnderscoreRegex.ReplaceAllString(s, "_")

	// Trim underscores from start and end
	s = strings.Trim(s, "_")

	// Convert to lowercase for consistency
	return strings.ToLower(s)
}

// ForDomainPart sanitizes a string for use as part of a domain name
// Domain parts can contain lowercase letters, numbers, and hyphens
func ForDomainPart(s string) string {
	if s == "" {
		return ""
	}

	// Convert to lowercase first
	s = strings.ToLower(s)

	// Replace common separators with hyphens
	s = domainPartReplacer.Replace(s)

	// Remove any remaining non-alphanumeric characters (except hyphens)
	s = regexp.MustCompile(`[^a-z0-9-]+`).ReplaceAllString(s, "-")

	// Collapse multiple hyphens
	s = multiDashRegex.ReplaceAllString(s, "-")

	// Trim hyphens from start and end
	s = strings.Trim(s, "-")

	// Ensure it doesn't start or end with a hyphen (RFC compliance)
	if len(s) > 0 && (s[0] == '-' || s[len(s)-1] == '-') {
		s = strings.Trim(s, "-")
	}

	return s
}

// ForProjectName sanitizes a string for use as a Docker Compose project name
// Project names must start with a letter and contain only lowercase letters, numbers, and underscores
func ForProjectName(s string) string {
	if s == "" {
		return ""
	}

	// Convert to lowercase
	s = strings.ToLower(s)

	// Replace hyphens and dots with underscores
	s = strings.NewReplacer("-", "_", ".", "_", " ", "_").Replace(s)

	// Remove any remaining non-alphanumeric characters (except underscores)
	s = regexp.MustCompile(`[^a-z0-9_]+`).ReplaceAllString(s, "")

	// Ensure it starts with a letter
	if len(s) > 0 && !regexp.MustCompile(`^[a-z]`).MatchString(s) {
		s = "grove_" + s
	}

	// Truncate if too long (Docker Compose limit)
	if len(s) > 63 {
		s = s[:63]
	}

	return s
}

// ForServiceName sanitizes a string for use as a service name
// Service names can contain letters, numbers, underscores, and hyphens
func ForServiceName(s string) string {
	if s == "" {
		return ""
	}

	// Replace spaces and dots with hyphens
	s = strings.NewReplacer(" ", "-", ".", "-").Replace(s)

	// Remove any remaining invalid characters
	s = regexp.MustCompile(`[^a-zA-Z0-9_-]+`).ReplaceAllString(s, "")

	// Ensure it starts with a letter or number
	if len(s) > 0 && !regexp.MustCompile(`^[a-zA-Z0-9]`).MatchString(s) {
		s = "service-" + s
	}

	return s
}

// ForEnvironmentKey sanitizes a string for use as an environment variable key
// Environment keys must contain only uppercase letters, numbers, and underscores
func ForEnvironmentKey(s string) string {
	if s == "" {
		return ""
	}

	// Convert to uppercase
	s = strings.ToUpper(s)

	// Replace common separators with underscores
	s = strings.NewReplacer("-", "_", ".", "_", " ", "_").Replace(s)

	// Remove any remaining non-alphanumeric characters (except underscores)
	s = regexp.MustCompile(`[^A-Z0-9_]+`).ReplaceAllString(s, "_")

	// Collapse multiple underscores
	s = multiUnderscoreRegex.ReplaceAllString(s, "_")

	// Trim underscores
	s = strings.Trim(s, "_")

	// Ensure it starts with a letter
	if len(s) > 0 && !regexp.MustCompile(`^[A-Z]`).MatchString(s) {
		s = "ENV_" + s
	}

	return s
}

// ForFilename sanitizes a string for use in a filename (kebab-case).
func ForFilename(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	// Remove non-alphanumeric characters, except hyphens
	s = regexp.MustCompile(`[^a-z0-9-]+`).ReplaceAllString(s, "")
	// Collapse multiple hyphens
	s = multiDashRegex.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 50 { // Truncate long names
		s = s[:50]
	}
	return s
}