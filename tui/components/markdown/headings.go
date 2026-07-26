package markdown

import "strings"

// Heading is an ATX heading extracted from Markdown. Line is one-based so it
// can be passed directly to editor APIs such as nvim_win_set_cursor.
type Heading struct {
	Level int
	Text  string
	Line  int
}

// ExtractHeadings extracts the deliberately small v1 Markdown outline syntax:
// ATX headings at levels 1-6, outside YAML frontmatter and backtick/tilde
// fenced code blocks. Setext headings, indented code, and blockquote headings
// are intentionally outside this version's scope.
func ExtractHeadings(lines []string) []Heading {
	frontmatterEnd := -1
	if len(lines) > 0 && trimCR(lines[0]) == "---" {
		for i := 1; i < len(lines); i++ {
			if trimCR(lines[i]) == "---" {
				frontmatterEnd = i
				break
			}
		}
	}

	var headings []Heading
	var fence byte
	for i, raw := range lines {
		line := trimCR(raw)
		if frontmatterEnd >= 0 && i <= frontmatterEnd {
			continue
		}

		fenceLine, validFenceIndent := fenceContent(line)
		marker := byte(0)
		if validFenceIndent {
			marker = fenceMarker(strings.TrimSpace(fenceLine))
		}
		if fence != 0 {
			if marker == fence {
				fence = 0
			}
			continue
		}
		if marker != 0 {
			fence = marker
			continue
		}

		content := line
		indent := 0
		for indent < len(content) && indent < 4 && content[indent] == ' ' {
			indent++
		}
		if indent > 3 {
			continue
		}
		content = content[indent:]

		level := 0
		for level < len(content) && level < 6 && content[level] == '#' {
			level++
		}
		if level == 0 || level >= len(content) || content[level] != ' ' {
			continue
		}
		// Seven or more leading hashes are not an ATX heading.
		if level == 6 && level < len(content) && content[level] == '#' {
			continue
		}

		text := strings.TrimSpace(content[level+1:])
		if end := trailingHashRun(text); end >= 0 {
			text = strings.TrimSpace(text[:end])
		}
		headings = append(headings, Heading{Level: level, Text: text, Line: i + 1})
	}
	return headings
}

func trimCR(line string) string {
	return strings.TrimSuffix(line, "\r")
}

// fenceContent accepts CommonMark's 0-3 leading spaces. Tabs and 4+ spaces
// belong to indented-code syntax, which ExtractHeadings deliberately does not
// interpret as fenced code in v1.
func fenceContent(line string) (string, bool) {
	indent := 0
	for indent < len(line) && line[indent] == ' ' {
		indent++
	}
	if indent > 3 || (indent < len(line) && line[indent] == '\t') {
		return "", false
	}
	return line[indent:], true
}

func fenceMarker(trimmed string) byte {
	if strings.HasPrefix(trimmed, "```") {
		return '`'
	}
	if strings.HasPrefix(trimmed, "~~~") {
		return '~'
	}
	return 0
}

// trailingHashRun returns the start of an optional closing hash sequence. A
// closing sequence must be separated from heading text by whitespace.
func trailingHashRun(text string) int {
	i := len(text)
	for i > 0 && text[i-1] == '#' {
		i--
	}
	if i == len(text) || (i > 0 && text[i-1] != ' ') {
		return -1
	}
	return i
}
