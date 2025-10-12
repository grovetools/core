package wsnav

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-core/pkg/workspace/filter"
	"github.com/mattsolo1/grove-core/tui/components/table"
	"github.com/mattsolo1/grove-core/tui/theme"
)

// View renders the TUI with custom table-based display.
// Overrides the navigator's default View to provide hierarchical table rendering.
func (m Model) View() string {
	width := m.navigator.GetWidth()
	height := m.navigator.GetHeight()

	// Handle very small terminal sizes
	if width < 40 || height < 10 {
		return "Terminal too small. Please resize."
	}

	// Define fixed heights for header and footer
	const headerHeight = 3
	const footerHeight = 3
	const topMargin = 1

	// Calculate dynamic dimensions based on terminal size
	mainAreaHeight := height - headerHeight - footerHeight - topMargin
	if mainAreaHeight < 5 {
		return "Terminal too small. Please resize."
	}

	// Styles for different components
	headerStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.DefaultColors.Orange).
		Width(width - 4).
		Height(headerHeight - 2).
		Align(lipgloss.Center, lipgloss.Center).
		Bold(true)

	mainContentStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.DefaultColors.Border).
		Width(width - 4).
		Height(mainAreaHeight - 2).
		Padding(1)

	footerStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.DefaultColors.Orange).
		Width(width - 4).
		Height(footerHeight - 2).
		Align(lipgloss.Center, lipgloss.Center)

	// Calculate available height for table content
	availableTableHeight := mainAreaHeight - 9
	if availableTableHeight < 1 {
		availableTableHeight = 1
	}

	// Create content for each component
	headerContent := "WORKSPACE NAVIGATOR"

	mainContent := m.buildTableView(availableTableHeight)

	var footerParts []string
	if focused := m.navigator.GetFocusedProject(); focused != nil {
		footerParts = append(footerParts, fmt.Sprintf("[Focus: %s]", focused.Name))
	}
	filterVal := m.navigator.GetFilterInput()
	if filterVal != "" {
		footerParts = append(footerParts, fmt.Sprintf("Filter: %s", filterVal))
	}
	if len(footerParts) == 0 {
		footerParts = append(footerParts, "Press / to filter, ctrl+f to focus")
	}
	footerContent := strings.Join(footerParts, " > ")

	// Render each component
	header := headerStyle.Render(headerContent)
	mainContentBox := mainContentStyle.Render(mainContent)
	footer := footerStyle.Render(footerContent)

	// Stack header, main area, and footer vertically
	fullLayout := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		mainContentBox,
		footer,
	)

	// Add top margin to prevent border cutoff
	return "\n" + fullLayout
}

// buildTableView constructs and renders the main table of workspaces.
func (m *Model) buildTableView(availableHeight int) string {
	// Get filtered projects from navigator and convert to pointers
	filtered := m.navigator.GetFiltered()
	if len(filtered) == 0 {
		return "No workspaces discovered.\n\nTip: Configure search_paths in ~/.grove/config.yml"
	}

	// Convert to pointers for filter functions
	filteredPtrs := make([]*workspace.WorkspaceNode, len(filtered))
	for i := range filtered {
		filteredPtrs[i] = &filtered[i]
	}

	// Apply hierarchical grouping
	hierarchical := filter.GroupHierarchically(filteredPtrs, false)

	// Build table rows
	allRows := m.buildTableRows(hierarchical)

	cursor := m.navigator.GetCursor()

	// Calculate visible rows based on scroll offset
	startIdx := m.scrollOffset
	endIdx := startIdx + availableHeight
	if endIdx > len(allRows) {
		endIdx = len(allRows)
	}
	if startIdx >= len(allRows) {
		startIdx = 0
		endIdx = len(allRows)
		if endIdx > availableHeight {
			endIdx = availableHeight
		}
	}

	// Ensure cursor is visible
	if cursor < startIdx {
		m.scrollOffset = cursor
		startIdx = cursor
		endIdx = startIdx + availableHeight
		if endIdx > len(allRows) {
			endIdx = len(allRows)
		}
	}
	if cursor >= endIdx {
		endIdx = cursor + 1
		startIdx = endIdx - availableHeight
		if startIdx < 0 {
			startIdx = 0
		}
		m.scrollOffset = startIdx
	}

	visibleRows := allRows[startIdx:endIdx]

	// Adjust cursor to be relative to the visible window
	relativeCursor := cursor - m.scrollOffset
	if relativeCursor < 0 {
		relativeCursor = 0
	}
	if relativeCursor >= len(visibleRows) {
		relativeCursor = len(visibleRows) - 1
	}

	// Use the selectable table component for rendering.
	mainContent := table.SelectableTableWithOptions(
		[]string{"K", "●", "WORKSPACE", "PATH"},
		visibleRows,
		relativeCursor,
		table.SelectableTableOptions{HighlightColumn: 2},
	)

	// Add scroll indicator if there are more items
	if len(allRows) > availableHeight {
		mainContent += "\n" + lipgloss.NewStyle().Faint(true).Render(
			fmt.Sprintf("Showing %d-%d of %d workspaces", startIdx+1, endIdx, len(allRows)),
		)
	}

	return mainContent
}

// buildTableRows creates the data rows for the workspace table, including
// indentation to create a hierarchical view based on depth.
func (m *Model) buildTableRows(projects []*workspace.WorkspaceNode) [][]string {
	var rows [][]string

	// Build a map of parent path -> children to determine if a node is the last child
	childrenMap := make(map[string][]*workspace.WorkspaceNode)
	for _, p := range projects {
		parent := p.GetHierarchicalParent()
		if parent != "" {
			childrenMap[parent] = append(childrenMap[parent], p)
		}
	}

	// Determine if a node is the last child of its parent
	isLastChild := func(node *workspace.WorkspaceNode) bool {
		parent := node.GetHierarchicalParent()
		if parent == "" {
			return false
		}
		children := childrenMap[parent]
		return len(children) > 0 && children[len(children)-1].Path == node.Path
	}

	for _, p := range projects {
		depth := p.GetDepth()

		// Build indentation string based on depth
		var indent string
		var prefix string

		if depth > 0 {
			// Add indentation (2 spaces per level)
			indent = strings.Repeat("  ", depth-1)

			// Add tree connector
			if isLastChild(p) {
				prefix = "└─ "
			} else {
				prefix = "├─ "
			}
		}

		kind := kindAbbreviation(p.Kind)
		name := fmt.Sprintf("%s%s%s", indent, prefix, p.Name)
		path := shortenPath(p.Path)

		rows = append(rows, []string{
			kind,
			"●", // Placeholder for status
			name,
			lipgloss.NewStyle().Faint(true).Render(path),
		})
	}
	return rows
}
