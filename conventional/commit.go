package conventional

import (
	"fmt"
	"regexp"
	"strings"
)

// Commit represents a parsed conventional commit message.
type Commit struct {
	Type       string
	Scope      string
	Subject    string
	Body       string
	Footer     map[string]string
	IsBreaking bool
}

// Regex to parse a conventional commit message.
// It captures: 1: type, 2: scope (optional), 3: breaking change indicator (!), 4: subject
var commitRegex = regexp.MustCompile(`^(\w+)(?:\(([^)]+)\))?(!?):\s(.*)$`)

// Parse parses a raw git commit message string into a Commit struct.
func Parse(message string) (*Commit, error) {
	lines := strings.SplitN(strings.TrimSpace(message), "\n", 2)
	header := lines[0]

	matches := commitRegex.FindStringSubmatch(header)
	if len(matches) < 5 {
		return nil, fmt.Errorf("invalid commit message format: %s", header)
	}

	commit := &Commit{
		Type:       strings.ToLower(matches[1]),
		Scope:      matches[2],
		IsBreaking: matches[3] == "!",
		Subject:    matches[4],
		Footer:     make(map[string]string),
	}

	if len(lines) > 1 {
		bodyAndFooter := strings.TrimSpace(lines[1])
		// Simple logic to find BREAKING CHANGE: in body/footer
		if strings.Contains(bodyAndFooter, "BREAKING CHANGE:") || strings.Contains(bodyAndFooter, "BREAKING-CHANGE:") {
			commit.IsBreaking = true
		}
		commit.Body = bodyAndFooter // For now, treat everything after header as body
	}

	return commit, nil
}