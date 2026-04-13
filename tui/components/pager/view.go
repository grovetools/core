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
// row → body → optional footer. When dimensions are known the body
// is force-expanded to fill remaining vertical space, pinning the
// footer to the bottom of the pane. Total chrome matches
// ChromeRows() so sub-models sized via SubSize fit exactly. When
// the active page implements PageWithReady and reports not ready, a
// centered loading message replaces the body.
func (m Model) View() string {
	if len(m.pages) == 0 {
		return ""
	}

	th := core_theme.DefaultTheme
	bar := m.RenderTabBar()

	active := m.pages[m.activePage]

	// If the active page supplies a footer, use it. This lets pages
	// pin help text at the bottom without embedding it in their body.
	if fp, ok := active.(PageWithFooter); ok {
		m.footer = fp.Footer()
	}

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

	// Force body to fill remaining vertical space when dimensions
	// are known, so any footer pins to the bottom of the pane.
	if m.height > 0 {
		pad := m.cfg.OuterPadding
		headerRows := tabBarHeight
		if m.cfg.ShowTitleRow {
			headerRows++
		}
		// Use rendered footer height when present, otherwise fall
		// back to the static Config.FooterHeight so hosts that
		// render their own footer externally still get correct
		// body sizing.
		footerRows := m.cfg.FooterHeight
		if m.footer != "" {
			footerRows = lipgloss.Height(m.footer)
		}
		bodyHeight := m.height - headerRows - footerRows - pad[0] - pad[2]
		if bodyHeight < 1 {
			bodyHeight = 1
		}
		bodyWidth := m.width - pad[1] - pad[3]
		if bodyWidth < 1 {
			bodyWidth = 1
		}
		body = lipgloss.NewStyle().
			Height(bodyHeight).
			MaxHeight(bodyHeight).
			MaxWidth(bodyWidth).
			Render(body)
	}

	// Compose: tab bar → blank spacer → [title] → body → [footer].
	parts := []string{bar, ""}
	if m.cfg.ShowTitleRow {
		title := " "
		if t, ok := active.(PageWithTitle); ok {
			if s := t.Title(); s != "" {
				title = lipgloss.NewStyle().Bold(true).
					Foreground(th.Colors.LightText).Render(s)
			}
		}
		parts = append(parts, title)
	}
	parts = append(parts, body)
	if m.footer != "" {
		parts = append(parts, m.footer)
	}
	content := lipgloss.JoinVertical(lipgloss.Left, parts...)

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
