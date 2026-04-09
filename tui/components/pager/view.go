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
// Disabled tabs are rendered faintly.
func (m Model) RenderTabBar() string {
	th := core_theme.DefaultTheme
	icons := numericIcons()

	activeNum := lipgloss.NewStyle().Foreground(th.Colors.Violet).Bold(true)
	activeName := lipgloss.NewStyle().Foreground(th.Colors.LightText).Bold(true)
	inactiveNum := lipgloss.NewStyle().Foreground(th.Colors.MutedText)
	inactiveName := th.Muted
	disabledNum := lipgloss.NewStyle().Foreground(th.Colors.MutedText).Faint(true)
	disabledName := th.Muted.Faint(true)

	var parts []string
	for i, p := range m.pages {
		icon := ""
		if i < len(icons) {
			icon = icons[i]
		}
		name := p.Name()
		switch {
		case i == m.activePage:
			parts = append(parts, fmt.Sprintf("%s %s", activeNum.Render(icon), activeName.Render(name)))
		case !m.isTabEnabled(i):
			parts = append(parts, fmt.Sprintf("%s %s", disabledNum.Render(icon), disabledName.Render(name)))
		default:
			parts = append(parts, fmt.Sprintf("%s %s", inactiveNum.Render(icon), inactiveName.Render(name)))
		}
	}

	separator := th.Muted.Faint(true).Render("  •  ")
	return strings.Join(parts, separator)
}

// View renders outer padding → tab bar → blank → optional title
// row → body. Total chrome matches ChromeRows() so sub-models sized
// via SubSize fit exactly. When the active page implements
// PageWithReady and reports not ready, a centered loading message
// replaces the body.
func (m Model) View() string {
	if len(m.pages) == 0 {
		return ""
	}

	th := core_theme.DefaultTheme
	bar := m.RenderTabBar()

	active := m.pages[m.activePage]

	// Body: loading placeholder if the page is async-not-ready,
	// otherwise the page's own View().
	var body string
	if r, ok := active.(PageWithReady); ok {
		if ready, loadingMsg := r.Ready(); !ready {
			sub := m.SubSize(m.width, m.height)
			if loadingMsg == "" {
				loadingMsg = "Loading…"
			}
			body = lipgloss.Place(sub.Width, sub.Height,
				lipgloss.Center, lipgloss.Center,
				th.Muted.Render(loadingMsg))
		} else {
			body = active.View()
		}
	} else {
		body = active.View()
	}

	// Title row (optional). When enabled, the row is always present
	// to keep vertical geometry constant across tab switches — a
	// page without a title renders a single space so lipgloss
	// doesn't collapse the row.
	var content string
	if m.cfg.ShowTitleRow {
		title := " "
		if t, ok := active.(PageWithTitle); ok {
			if s := t.Title(); s != "" {
				title = lipgloss.NewStyle().Bold(true).
					Foreground(th.Colors.LightText).Render(s)
			}
		}
		content = lipgloss.JoinVertical(lipgloss.Left, bar, "", title, body)
	} else {
		content = lipgloss.JoinVertical(lipgloss.Left, bar, "", body)
	}

	// Outer padding (top, right, bottom, left). Zero values render
	// without any extra style, preserving legacy behavior.
	pad := m.cfg.OuterPadding
	if pad[0] == 0 && pad[1] == 0 && pad[2] == 0 && pad[3] == 0 {
		return content
	}
	return lipgloss.NewStyle().
		Padding(pad[0], pad[1], pad[2], pad[3]).
		Render(content)
}
