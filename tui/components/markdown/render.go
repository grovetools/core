package markdown

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/grovetools/core/tui/theme"
)

// Render applies basic syntax highlighting to markdown content.
// It accepts a theme parameter for styling and returns the styled string.
func Render(content string, th *theme.Theme) string {
	var styledFrontmatter string
	bodyBlock := content

	// Check for frontmatter and extract it if present
	if strings.HasPrefix(content, "---\n") {
		secondSeparator := strings.Index(content[4:], "\n---\n")
		if secondSeparator != -1 {
			frontmatterEnd := 4 + secondSeparator + 5
			frontmatterBlock := content[:frontmatterEnd]
			bodyBlock = content[frontmatterEnd:]
			styledFrontmatter = th.Muted.Italic(true).Render(frontmatterBlock)
		}
	}

	var bodyBuilder strings.Builder
	h1Style := th.Header.Bold(true).Foreground(th.Colors.Cyan)
	h2Style := th.Header.Foreground(th.Colors.Blue)
	h3Style := th.Header.Foreground(th.Colors.Violet)
	codeBlockStyle := lipgloss.NewStyle().Foreground(th.Colors.Green)
	bulletStyle := lipgloss.NewStyle().Foreground(th.Colors.Orange).Bold(true)

	inCodeBlock := false
	lines := strings.Split(bodyBlock, "\n")

	for _, line := range lines {
		// Handle fenced code blocks
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inCodeBlock = !inCodeBlock
			bodyBuilder.WriteString(codeBlockStyle.Render(line) + "\n")
			continue
		}

		if inCodeBlock {
			bodyBuilder.WriteString(codeBlockStyle.Render(line) + "\n")
			continue
		}

		// Replace Claude todo list markers with themed icons
		line = StyleTodoMarkers(line, th)

		// Handle headers
		if strings.HasPrefix(line, "### ") {
			bodyBuilder.WriteString(h3Style.Render(line) + "\n")
		} else if strings.HasPrefix(line, "## ") {
			bodyBuilder.WriteString(h2Style.Render(line) + "\n")
		} else if strings.HasPrefix(line, "# ") {
			bodyBuilder.WriteString(h1Style.Render(line) + "\n")
		} else {
			// Handle list bullets
			trimmed := strings.TrimLeft(line, " \t")
			if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
				indent := line[:len(line)-len(trimmed)]
				bullet := trimmed[:2]
				rest := trimmed[2:]
				styledRest := StyleInlineMarkdown(rest, th)
				bodyBuilder.WriteString(indent + bulletStyle.Render(bullet) + styledRest + "\n")
			} else if len(trimmed) > 0 && trimmed[0] >= '0' && trimmed[0] <= '9' {
				dotIdx := strings.Index(trimmed, ". ")
				if dotIdx > 0 && dotIdx <= 3 {
					indent := line[:len(line)-len(trimmed)]
					bullet := trimmed[:dotIdx+2]
					rest := trimmed[dotIdx+2:]
					styledRest := StyleInlineMarkdown(rest, th)
					bodyBuilder.WriteString(indent + bulletStyle.Render(bullet) + styledRest + "\n")
				} else {
					styledLine := StyleInlineMarkdown(line, th)
					bodyBuilder.WriteString(styledLine + "\n")
				}
			} else {
				styledLine := StyleInlineMarkdown(line, th)
				bodyBuilder.WriteString(styledLine + "\n")
			}
		}
	}

	return styledFrontmatter + bodyBuilder.String()
}

// StyleStreamingLogLine applies markdown styling to a single log line during streaming.
// It takes a pointer to the inCodeBlock state to track fenced code blocks across lines.
func StyleStreamingLogLine(line string, inCodeBlock *bool, th *theme.Theme) string {
	h1Style := th.Header.Bold(true).Foreground(th.Colors.Cyan)
	h2Style := th.Header.Foreground(th.Colors.Blue)
	h3Style := th.Header.Foreground(th.Colors.Violet)
	codeBlockStyle := lipgloss.NewStyle().Foreground(th.Colors.Green)
	bulletStyle := lipgloss.NewStyle().Foreground(th.Colors.Orange).Bold(true)

	// Handle fenced code blocks
	if strings.HasPrefix(strings.TrimSpace(line), "```") {
		*inCodeBlock = !*inCodeBlock
		return codeBlockStyle.Render(line)
	}

	if *inCodeBlock {
		return codeBlockStyle.Render(line)
	}

	// Replace Claude todo list markers with themed icons
	line = StyleTodoMarkers(line, th)

	// Handle headers
	if strings.HasPrefix(line, "### ") {
		return h3Style.Render(line)
	} else if strings.HasPrefix(line, "## ") {
		return h2Style.Render(line)
	} else if strings.HasPrefix(line, "# ") {
		return h1Style.Render(line)
	}

	// Handle list bullets
	trimmed := strings.TrimLeft(line, " \t")
	if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
		indent := line[:len(line)-len(trimmed)]
		bullet := trimmed[:2]
		rest := trimmed[2:]
		styledRest := StyleInlineMarkdown(rest, th)
		return indent + bulletStyle.Render(bullet) + styledRest
	} else if len(trimmed) > 0 && trimmed[0] >= '0' && trimmed[0] <= '9' {
		dotIdx := strings.Index(trimmed, ". ")
		if dotIdx > 0 && dotIdx <= 3 {
			indent := line[:len(line)-len(trimmed)]
			bullet := trimmed[:dotIdx+2]
			rest := trimmed[dotIdx+2:]
			styledRest := StyleInlineMarkdown(rest, th)
			return indent + bulletStyle.Render(bullet) + styledRest
		}
	}

	return StyleInlineMarkdown(line, th)
}

// StyleInlineMarkdown applies styling for bold, italic, and inline code markdown syntax.
func StyleInlineMarkdown(line string, th *theme.Theme) string {
	boldStyle := lipgloss.NewStyle().Bold(true)
	italicStyle := lipgloss.NewStyle().Italic(true)
	codeStyle := lipgloss.NewStyle().
		Background(th.Colors.SubtleBackground).
		Foreground(th.Colors.Cyan)
	result := line

	// Handle inline `code` first (before other styling to prevent interference)
	for {
		start := strings.Index(result, "`")
		if start == -1 {
			break
		}
		// Skip if this is part of a code fence (```)
		if start+2 < len(result) && result[start:start+3] == "```" {
			result = result[:start] + "\x00\x00\x00" + result[start+3:] // Placeholder
			continue
		}
		end := strings.Index(result[start+1:], "`")
		if end == -1 {
			break
		}
		end += start + 1
		// Skip if end is part of a code fence
		if end+2 < len(result) && result[end:end+3] == "```" {
			break
		}
		codeContent := result[start+1 : end]
		result = result[:start] + codeStyle.Render(codeContent) + result[end+1:]
	}
	// Restore triple backticks
	result = strings.ReplaceAll(result, "\x00\x00\x00", "```")

	// Handle **bold**
	for {
		start := strings.Index(result, "**")
		if start == -1 {
			break
		}
		end := strings.Index(result[start+2:], "**")
		if end == -1 {
			break
		}
		end += start + 2
		result = result[:start] + boldStyle.Render(result[start+2:end]) + result[end+2:]
	}

	// Handle __bold__
	for {
		start := strings.Index(result, "__")
		if start == -1 {
			break
		}
		end := strings.Index(result[start+2:], "__")
		if end == -1 {
			break
		}
		end += start + 2
		result = result[:start] + boldStyle.Render(result[start+2:end]) + result[end+2:]
	}

	// Handle *italic*
	for {
		start := strings.Index(result, "*")
		if start == -1 {
			break
		}
		if start > 0 && result[start-1] == '*' {
			result = result[:start] + result[start+1:]
			continue
		}
		end := strings.Index(result[start+1:], "*")
		if end == -1 {
			break
		}
		end += start + 1
		if end+1 < len(result) && result[end+1] == '*' {
			result = result[:start] + result[start+1:]
			continue
		}
		result = result[:start] + italicStyle.Render(result[start+1:end]) + result[end+1:]
	}

	// Handle _italic_
	for {
		start := strings.Index(result, "_")
		if start == -1 {
			break
		}
		if start > 0 && result[start-1] == '_' {
			result = result[:start] + result[start+1:]
			continue
		}
		end := strings.Index(result[start+1:], "_")
		if end == -1 {
			break
		}
		end += start + 1
		if end+1 < len(result) && result[end+1] == '_' {
			result = result[:start] + result[start+1:]
			continue
		}
		result = result[:start] + italicStyle.Render(result[start+1:end]) + result[end+1:]
	}

	return result
}

// StyleTodoMarkers replaces Claude's todo list markers with themed icons.
// [*] -> completed (green checkmark)
// [→] -> in progress (running icon)
// [ ] -> pending (empty checkbox)
// ☒ -> completed
// ☐ -> pending
func StyleTodoMarkers(line string, th *theme.Theme) string {
	if strings.Contains(line, "[*]") {
		line = strings.Replace(line, "[*]", th.Success.Render(theme.IconStatusCompleted), -1)
	}
	if strings.Contains(line, "[→]") {
		line = strings.Replace(line, "[→]", th.Info.Render(theme.IconStatusRunning), -1)
	}
	if strings.Contains(line, "[ ]") {
		line = strings.Replace(line, "[ ]", th.Muted.Render(theme.IconStatusTodo), -1)
	}
	if strings.Contains(line, "☒") {
		line = strings.Replace(line, "☒", th.Success.Render(theme.IconStatusCompleted), -1)
	}
	if strings.Contains(line, "☐") {
		line = strings.Replace(line, "☐", th.Muted.Render(theme.IconStatusTodo), -1)
	}
	return line
}

// WrapForViewport wraps content lines to fit within the given width.
func WrapForViewport(content string, width int) string {
	if width <= 0 {
		return content
	}

	lines := strings.Split(content, "\n")
	wrapStyle := lipgloss.NewStyle().Width(width)

	var wrappedLines []string
	for _, line := range lines {
		wrappedLines = append(wrappedLines, wrapStyle.Render(line))
	}

	return strings.Join(wrappedLines, "\n")
}
