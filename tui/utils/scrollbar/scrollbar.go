package scrollbar

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/mattsolo1/grove-core/tui/theme"
)

// Generate creates scrollbar characters based on viewport position.
// Returns a slice of strings representing the scrollbar for each line of the given height.
func Generate(vp *viewport.Model, height int) []string {
	if height <= 0 {
		return []string{}
	}

	scrollbar := make([]string, height)

	// Count total content lines from the viewport
	totalLines := vp.TotalLineCount()
	if totalLines == 0 {
		// No content, show empty scrollbar
		for i := 0; i < height; i++ {
			scrollbar[i] = theme.DefaultTheme.Muted.Render(" ")
		}
		return scrollbar
	}

	// If content fits entirely in view, show all thumb
	if totalLines <= vp.Height {
		for i := 0; i < height; i++ {
			scrollbar[i] = theme.DefaultTheme.Muted.Render("█")
		}
		return scrollbar
	}

	// Calculate scrollbar thumb size proportional to visible content
	thumbSize := max(1, (height*vp.Height)/totalLines)

	// Get scroll position (0.0 to 1.0)
	scrollPercent := vp.ScrollPercent()
	if scrollPercent < 0 {
		scrollPercent = 0
	}
	if scrollPercent > 1 {
		scrollPercent = 1
	}

	// Calculate thumb position in scrollbar
	maxThumbStart := height - thumbSize
	thumbStart := int(float64(maxThumbStart)*scrollPercent + 0.5)

	// Ensure thumb doesn't go out of bounds
	if thumbStart < 0 {
		thumbStart = 0
	}
	if thumbStart > maxThumbStart {
		thumbStart = maxThumbStart
	}

	// Generate scrollbar characters
	for i := 0; i < height; i++ {
		if i >= thumbStart && i < thumbStart+thumbSize {
			scrollbar[i] = theme.DefaultTheme.Muted.Render("█") // Thumb
		} else {
			scrollbar[i] = theme.DefaultTheme.Muted.Render("░") // Track
		}
	}

	return scrollbar
}

// Overlay adds a scrollbar to the right side of viewport content.
// Returns the viewport's visible content with scrollbar characters appended to each line.
func Overlay(vp *viewport.Model) string {
	content := vp.View()
	lines := strings.Split(content, "\n")
	scrollbar := Generate(vp, len(lines))

	var result []string
	for i := 0; i < len(lines); i++ {
		line := lines[i]

		scrollbarChar := " "
		if i < len(scrollbar) {
			scrollbarChar = scrollbar[i]
		}

		result = append(result, line+scrollbarChar)
	}

	return strings.Join(result, "\n")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
