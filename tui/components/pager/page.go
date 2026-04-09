// Package pager provides a reusable tabbed meta-panel component:
// active-tab state, tab bar rendering, numeric 1-9 jump keys, and
// cross-page auto-switch via embed.SwitchTabMsg.
package pager

import tea "github.com/charmbracelet/bubbletea"

// Page is one tab of a pager.
type Page interface {
	Name() string
	Init() tea.Cmd
	Update(tea.Msg) (Page, tea.Cmd)
	View() string
	Focus() tea.Cmd
	Blur()
	SetSize(width, height int)
}
