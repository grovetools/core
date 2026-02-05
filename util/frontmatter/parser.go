// Package frontmatter provides lightweight YAML frontmatter parsing for markdown files.
// This avoids heavy dependencies on flow/plan packages for simple metadata extraction.
package frontmatter

import (
	"bufio"
	"io"
	"strings"
	"time"
)

// DocMetadata represents common fields found in markdown frontmatter.
type DocMetadata struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	Type      string    `json:"type"`
	StartedAt time.Time `json:"start_time"`
	UpdatedAt time.Time `json:"updated_at"`
	Worktree  string    `json:"worktree"`
}

// Parse extracts metadata from YAML frontmatter in a markdown reader.
// It stops reading after the closing '---' separator.
func Parse(r io.Reader) (DocMetadata, error) {
	scanner := bufio.NewScanner(r)
	meta := DocMetadata{
		Status: "pending",
		Type:   "oneshot",
	}

	inFrontmatter := false
	lineCount := 0

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if trimmed == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			} else {
				break // End of frontmatter
			}
		}

		if !inFrontmatter {
			// Stop if we haven't found frontmatter in the first few lines
			lineCount++
			if lineCount > 5 {
				break
			}
			continue
		}

		// Simple key: value parsing
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, `"'`)

		switch key {
		case "id":
			meta.ID = value
		case "title":
			meta.Title = value
		case "status":
			meta.Status = value
		case "type":
			meta.Type = value
		case "worktree":
			meta.Worktree = value
		case "start_time":
			if t, err := time.Parse(time.RFC3339, value); err == nil {
				meta.StartedAt = t
			}
		case "updated_at":
			if t, err := time.Parse(time.RFC3339, value); err == nil {
				meta.UpdatedAt = t
			}
		}
	}

	if meta.ID == "" {
		meta.ID = meta.Title
	}

	return meta, scanner.Err()
}

// ParseString extracts metadata from a string containing markdown with frontmatter.
func ParseString(content string) (DocMetadata, error) {
	return Parse(strings.NewReader(content))
}
