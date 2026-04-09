package pager

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	core_theme "github.com/grovetools/core/tui/theme"
)

// numericIcons returns the glyphs used to prefix each tab label in
// the order 1..9. It pulls from the theme package so ASCII and nerd
// font modes stay in sync with the rest of the ecosystem.
func numericIcons() []string {
	return []string{
		core_theme.IconNumeric1CircleOutline,
		core_theme.IconNumeric2CircleOutline,
		core_theme.IconNumeric3CircleOutline,
		core_theme.IconNumeric4CircleOutline,
		core_theme.IconNumeric5CircleOutline,
		core_theme.IconNumeric6CircleOutline,
		core_theme.IconNumeric7CircleOutline,
		core_theme.IconNumeric8CircleOutline,
		core_theme.IconNumeric9CircleOutline,
	}
}

// RenderTabBar returns the horizontal tab bar for the current page
// set. It is exported so hosts can compose it with their own padding
// or chrome; View() calls it under the hood.
//
// Even a single-page pager renders the bar, per the "consistency over
// visual noise" decision agreed in Phase 2 of the plan. A lone tab
// still reads as a title row and avoids layout jumping when a second
// tab is added later.
func (m Model) RenderTabBar() string {
	th := core_theme.DefaultTheme
	icons := numericIcons()

	activeNumStyle := lipgloss.NewStyle().
		Foreground(th.Colors.Violet).
		Bold(true)
	activeNameStyle := lipgloss.NewStyle().
		Foreground(th.Colors.LightText).
		Bold(true)
	inactiveNumStyle := lipgloss.NewStyle().
		Foreground(th.Colors.MutedText)
	inactiveNameStyle := th.Muted

	var parts []string
	for i, p := range m.pages {
		icon := ""
		if i < len(icons) {
			icon = icons[i]
		}
		name := p.Name()
		if i == m.activePage {
			parts = append(parts, fmt.Sprintf("%s %s",
				activeNumStyle.Render(icon),
				activeNameStyle.Render(name)))
		} else {
			parts = append(parts, fmt.Sprintf("%s %s",
				inactiveNumStyle.Render(icon),
				inactiveNameStyle.Render(name)))
		}
	}

	separator := th.Muted.Faint(true).Render("  •  ")
	return strings.Join(parts, separator)
}

// View renders the tab bar above the active page's view, joined
// vertically with a single blank line between them. The total
// vertical footprint matches tabBarHeight (2 rows) so WindowSizeMsg
// deductions stay consistent.
func (m Model) View() string {
	if len(m.pages) == 0 {
		return ""
	}
	bar := m.RenderTabBar()
	body := m.pages[m.activePage].View()
	return lipgloss.JoinVertical(lipgloss.Left, bar, "", body)
}
