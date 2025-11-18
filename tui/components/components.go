package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattsolo1/grove-core/tui/theme"
)

// RenderHeader creates a consistent header for TUIs
func RenderHeader(title string, subtitle ...string) string {
	t := theme.DefaultTheme

	header := t.Header.Render(fmt.Sprintf("%s %s", theme.IconTree, title))

	if len(subtitle) > 0 && subtitle[0] != "" {
		sub := t.Muted.Render(subtitle[0])
		return lipgloss.JoinVertical(lipgloss.Left, header, sub)
	}

	return header
}

// RenderFooter creates a consistent footer for TUIs
func RenderFooter(content string, width int) string {
	footerStyle := lipgloss.NewStyle().
		Foreground(theme.MutedText).
		Width(width).
		Align(lipgloss.Center).
		BorderStyle(lipgloss.NormalBorder()).
		BorderTop(true).
		BorderForeground(theme.Border).
		MarginTop(1)

	return footerStyle.Render(content)
}

// RenderBreadcrumb creates a breadcrumb navigation display
func RenderBreadcrumb(items ...string) string {
	t := theme.DefaultTheme

	if len(items) == 0 {
		return ""
	}

	parts := make([]string, 0, len(items)*2-1)
	for i, item := range items {
		if i == len(items)-1 {
			// Last item is highlighted
			parts = append(parts, t.Highlight.Render(item))
		} else {
			parts = append(parts, t.Muted.Render(item))
			parts = append(parts, t.Muted.Render(theme.IconArrow))
		}
	}

	return strings.Join(parts, " ")
}

// RenderStatusBar creates a status bar with multiple sections
func RenderStatusBar(left, center, right string, width int) string {
	// Calculate space for each section
	totalContent := len(stripANSI(left)) + len(stripANSI(center)) + len(stripANSI(right))
	if totalContent >= width {
		// Not enough space, just show left aligned content
		return left
	}

	// Calculate padding
	remainingSpace := width - totalContent
	leftPad := remainingSpace / 3
	rightPad := remainingSpace / 3
	centerPad := remainingSpace - leftPad - rightPad

	// Build the status bar
	var parts []string

	if left != "" {
		parts = append(parts, left)
	}

	if center != "" {
		if len(parts) > 0 {
			parts = append(parts, strings.Repeat(" ", leftPad))
		}
		parts = append(parts, center)
	}

	if right != "" {
		if len(parts) > 0 {
			if center != "" {
				parts = append(parts, strings.Repeat(" ", centerPad))
			} else {
				parts = append(parts, strings.Repeat(" ", width-len(stripANSI(left))-len(stripANSI(right))))
			}
		}
		parts = append(parts, right)
	}

	statusBar := strings.Join(parts, "")

	// Apply styling
	return lipgloss.NewStyle().
		Width(width).
		Background(theme.SubtleBackground).
		Foreground(theme.LightText).
		Render(statusBar)
}

// RenderDivider creates a horizontal divider
func RenderDivider(width int) string {
	return lipgloss.NewStyle().
		Width(width).
		Foreground(theme.Border).
		Render(strings.Repeat("─", width))
}

// RenderBox renders content in a styled box
func RenderBox(title, content string, width int) string {
	t := theme.DefaultTheme

	// Create the box
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Width(width - 2).
		Padding(1, 2)

	// Add title if provided
	if title != "" {
		titleStyle := t.Highlight.
			Background(theme.SubtleBackground).
			Padding(0, 1)

		// Render box with title
		boxContent := box.Render(content)
		lines := strings.Split(boxContent, "\n")

		if len(lines) > 0 {
			// Insert title into the top border
			topLine := lines[0]
			titleStr := titleStyle.Render(title)

			// Replace part of the top border with the title
			if len(topLine) > len(titleStr)+4 {
				lines[0] = topLine[:2] + titleStr + topLine[2+len(stripANSI(titleStr)):]
			}
		}

		return strings.Join(lines, "\n")
	}

	return box.Render(content)
}

// RenderList creates a styled list
func RenderList(items []string, ordered bool) string {
	t := theme.DefaultTheme

	if len(items) == 0 {
		return ""
	}

	var lines []string
	for i, item := range items {
		var prefix string
		if ordered {
			prefix = t.Highlight.Render(fmt.Sprintf("%2d.", i+1))
		} else {
			prefix = t.Highlight.Render(theme.IconBullet)
		}
		lines = append(lines, fmt.Sprintf("%s %s", prefix, item))
	}

	return strings.Join(lines, "\n")
}

// RenderProgress creates a progress bar
func RenderProgress(current, total int, width int) string {
	t := theme.DefaultTheme

	if total <= 0 {
		return ""
	}

	// Calculate percentage
	percentage := float64(current) / float64(total)
	if percentage > 1.0 {
		percentage = 1.0
	}

	// Calculate filled width
	barWidth := width - 10 // Leave space for percentage text
	filledWidth := int(percentage * float64(barWidth))

	// Create progress bar
	filled := strings.Repeat("█", filledWidth)
	empty := strings.Repeat("░", barWidth-filledWidth)

	bar := t.Success.Render(filled) + t.Muted.Render(empty)

	// Add percentage
	percentText := fmt.Sprintf(" %3d%%", int(percentage*100))
	return bar + t.Muted.Render(percentText)
}

// RenderSpinner creates a simple spinner animation frame
func RenderSpinner(frame int) string {
	spinners := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	return theme.DefaultTheme.Info.Render(spinners[frame%len(spinners)])
}

// RenderTabs creates a tab bar
func RenderTabs(tabs []string, activeIndex int) string {
	if len(tabs) == 0 {
		return ""
	}

	var renderedTabs []string
	for i, tab := range tabs {
		var style lipgloss.Style
		if i == activeIndex {
			// Active tab
			style = lipgloss.NewStyle().
				Background(theme.SelectedBackground).
				Foreground(theme.LightText).
				Padding(0, 2).
				Bold(true)
		} else {
			// Inactive tab
			style = lipgloss.NewStyle().
				Background(theme.SubtleBackground).
				Foreground(theme.MutedText).
				Padding(0, 2)
		}
		renderedTabs = append(renderedTabs, style.Render(tab))
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, renderedTabs...)
}

// RenderKeyValue creates a key-value display
func RenderKeyValue(key, value string) string {
	t := theme.DefaultTheme
	return fmt.Sprintf("%s %s", t.Muted.Render(key+":"), value)
}

// RenderSection creates a section with a title and content
func RenderSection(title, content string) string {
	t := theme.DefaultTheme
	titleLine := t.Header.Render(fmt.Sprintf("%s %s", theme.IconSelect, title))
	contentLines := lipgloss.NewStyle().
		MarginLeft(2).
		Render(content)

	return lipgloss.JoinVertical(lipgloss.Left, titleLine, contentLines)
}

// Utility function to strip ANSI codes (simplified version)
func stripANSI(str string) string {
	// This is a simplified version. In production, use a proper ANSI stripping library
	return lipgloss.NewStyle().Render(str)
}