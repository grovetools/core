package wsnav

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-core/tui/theme"
)

// View renders the TUI.
func (m Model) View() string {
	if m.help.ShowAll {
		return m.help.View()
	}

	// Handle very small terminal sizes
	if m.width < 40 || m.height < 10 {
		return "Terminal too small. Please resize."
	}

	// Define fixed heights for header and footer
	const headerHeight = 3
	const footerHeight = 3
	const topMargin = 1

	// Calculate dynamic dimensions based on terminal size
	mainAreaHeight := m.height - headerHeight - footerHeight - topMargin
	if mainAreaHeight < 5 {
		return "Terminal too small. Please resize."
	}

	// Styles for different components
	headerStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.DefaultColors.Orange).
		Width(m.width - 4).
		Height(headerHeight - 2).
		Align(lipgloss.Center, lipgloss.Center).
		Bold(true)

	mainContentStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.DefaultColors.Border).
		Width(m.width - 4).
		Height(mainAreaHeight - 2).
		Padding(1)

	footerStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.DefaultColors.Orange).
		Width(m.width - 4).
		Height(footerHeight - 2).
		Align(lipgloss.Center, lipgloss.Center)

	// Calculate available height for table content
	// mainAreaHeight already accounts for header, footer, and topMargin
	// The mainContentStyle has Padding(1) which takes 2 lines (top+bottom)
	// We also need space for: table header row (1), separator (1), potential scroll indicator (1)
	// So: mainAreaHeight - 2 (padding) - 1 (header) - 1 (separator) - 1 (scroll indicator) - 2 (safety margin)
	availableTableHeight := mainAreaHeight - 7

	// Create content for each component
	headerContent := "WORKSPACE NAVIGATOR"

	mainContent := m.buildTableView(availableTableHeight)

	footerContent := m.help.View()

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
	if len(m.viewProjects) == 0 {
		return "No workspaces discovered.\n\nTip: Configure search_paths in ~/.grove/config.yml"
	}

	rows := m.buildTableRows()

	var sb strings.Builder

	// Header row
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(theme.DefaultColors.Orange)

	sb.WriteString(headerStyle.Render("KIND"))
	sb.WriteString(strings.Repeat(" ", 10))
	sb.WriteString(headerStyle.Render("WORKSPACE"))
	sb.WriteString(strings.Repeat(" ", 28))
	sb.WriteString(headerStyle.Render("PATH"))
	sb.WriteString("\n")

	// Safe separator line
	separatorWidth := m.width - 6
	if separatorWidth > 0 {
		sb.WriteString(strings.Repeat("─", separatorWidth))
	}
	sb.WriteString("\n")

	// Calculate visible rows based on scroll offset
	startIdx := m.scrollOffset
	endIdx := startIdx + availableHeight
	if endIdx > len(rows) {
		endIdx = len(rows)
	}

	// Data rows (only visible ones)
	for i := startIdx; i < endIdx; i++ {
		row := rows[i]
		rowStyle := lipgloss.NewStyle()
		if i == m.cursor {
			// Highlight selected row
			rowStyle = rowStyle.
				Background(theme.DefaultColors.Orange).
				Foreground(theme.DefaultColors.LightText)
		}

		// Format: KIND  WORKSPACE  PATH
		line := fmt.Sprintf("%-14s %-35s %s", row[0], row[1], row[2])

		if i == m.cursor {
			line = rowStyle.Render(line)
		}

		sb.WriteString(line)
		sb.WriteString("\n")
	}

	// Add scroll indicator if there are more items
	if len(rows) > availableHeight {
		sb.WriteString("\n")
		sb.WriteString(lipgloss.NewStyle().Faint(true).Render(
			fmt.Sprintf("Showing %d-%d of %d workspaces", startIdx+1, endIdx, len(rows)),
		))
	}

	return sb.String()
}

// buildTableRows creates the data rows for the workspace table, including
// indentation to create a hierarchical view.
func (m *Model) buildTableRows() [][]string {
	var rows [][]string

	// Create a map to track the last worktree for each parent.
	lastWorktreeOfParent := make(map[string]string)
	for i := len(m.viewProjects) - 1; i >= 0; i-- {
		p := m.viewProjects[i]
		if p.ParentProjectPath != "" {
			if _, exists := lastWorktreeOfParent[p.ParentProjectPath]; !exists {
				lastWorktreeOfParent[p.ParentProjectPath] = p.Path
			}
		}
	}

	for _, p := range m.viewProjects {
		var indent, prefix string

		// Determine indentation and prefix for tree structure.
		if p.ParentProjectPath != "" {
			// This is a worktree. Let's find its parent.
			var parent *workspace.ProjectInfo
			for _, potentialParent := range m.allProjects {
				if potentialParent.Path == p.ParentProjectPath {
					parent = potentialParent
					break
				}
			}

			if parent != nil && parent.ParentEcosystemPath != "" {
				indent = "  " // Indent one level if parent is in an ecosystem
			}

			isLast := lastWorktreeOfParent[p.ParentProjectPath] == p.Path
			if isLast {
				prefix = "└─ "
			} else {
				prefix = "├─ "
			}
		}

		kind := kindAbbreviation(p.Kind)

		name := fmt.Sprintf("%s%s%s", indent, prefix, p.Name)
		path := shortenPath(p.Path)

		// For ecosystem worktrees, we want to show the path relative to the ecosystem root
		if p.Kind == workspace.KindEcosystemWorktreeSubProject || p.Kind == workspace.KindEcosystemWorktreeSubProjectWorktree {
			if p.ParentEcosystemPath != "" {
				// The parent is the ecosystem worktree, find its parent (the ecosystem root)
				ecoRootPath := filepath.Dir(filepath.Dir(p.ParentEcosystemPath))
				relPath, err := filepath.Rel(ecoRootPath, p.Path)
				if err == nil {
					path = filepath.Join(shortenPath(ecoRootPath), relPath)
				}
			}
		}

		rows = append(rows, []string{
			kind,
			name,
			lipgloss.NewStyle().Faint(true).Render(path),
		})
	}
	return rows
}
