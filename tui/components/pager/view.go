package pager

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	core_theme "github.com/grovetools/core/tui/theme"
)

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
// set. Single-page pagers still render the bar (consistency).
func (m Model) RenderTabBar() string {
	th := core_theme.DefaultTheme
	icons := numericIcons()

	activeNum := lipgloss.NewStyle().Foreground(th.Colors.Violet).Bold(true)
	activeName := lipgloss.NewStyle().Foreground(th.Colors.LightText).Bold(true)
	inactiveNum := lipgloss.NewStyle().Foreground(th.Colors.MutedText)
	inactiveName := th.Muted

	var parts []string
	for i, p := range m.pages {
		icon := ""
		if i < len(icons) {
			icon = icons[i]
		}
		name := p.Name()
		if i == m.activePage {
			parts = append(parts, fmt.Sprintf("%s %s", activeNum.Render(icon), activeName.Render(name)))
		} else {
			parts = append(parts, fmt.Sprintf("%s %s", inactiveNum.Render(icon), inactiveName.Render(name)))
		}
	}

	separator := th.Muted.Faint(true).Render("  •  ")
	return strings.Join(parts, separator)
}

// View renders bar + blank + body. Total vertical footprint matches
// tabBarHeight so WindowSizeMsg deduction stays consistent.
func (m Model) View() string {
	if len(m.pages) == 0 {
		return ""
	}
	bar := m.RenderTabBar()
	body := m.pages[m.activePage].View()
	return lipgloss.JoinVertical(lipgloss.Left, bar, "", body)
}
