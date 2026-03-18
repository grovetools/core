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
	Tags      []string  `json:"tags,omitempty"`
	PlanRef   string    `json:"plan_ref,omitempty"`
	Created   time.Time `json:"created,omitempty"`
	Modified  time.Time `json:"modified,omitempty"`
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
	collectingTags := false

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

		// Check for block-sequence tag items (  - item)
		if collectingTags {
			if strings.HasPrefix(line, "  - ") || strings.HasPrefix(line, "\t- ") {
				tag := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- "))
				tag = strings.Trim(tag, `"'`)
				if tag != "" {
					meta.Tags = append(meta.Tags, tag)
				}
				continue
			}
			collectingTags = false
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
		case "plan_ref":
			meta.PlanRef = value
		case "tags":
			if value == "" {
				// Block sequence format — collect on subsequent lines
				collectingTags = true
			} else {
				// Flow array format: [a, b, c]
				meta.Tags = parseFlowArray(value)
			}
		case "created":
			meta.Created = parseTimestamp(value)
		case "modified":
			meta.Modified = parseTimestamp(value)
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

// parseFlowArray parses a YAML flow array like "[a, b, c]" into a string slice.
func parseFlowArray(s string) []string {
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	if s == "" {
		return nil
	}
	var result []string
	for _, item := range strings.Split(s, ",") {
		item = strings.TrimSpace(item)
		item = strings.Trim(item, `"'`)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

// parseTimestamp tries multiple time formats commonly used in note frontmatter.
func parseTimestamp(s string) time.Time {
	formats := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
