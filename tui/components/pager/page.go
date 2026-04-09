// Package pager provides a reusable tabbed meta-panel component for
// grovetools TUIs. It owns the activePage state, tab bar rendering,
// numeric jump keybindings (1-9), and cross-panel auto-switch
// semantics, letting individual TUIs focus on the content of their
// sub-pages rather than reimplementing tab plumbing every time.
package pager

import tea "github.com/charmbracelet/bubbletea"

// Page is the interface every tab of a pager must satisfy. It mirrors
// the ad-hoc Page interfaces that cx and memory grew independently,
// elevated into core so both (and nb, skills, flow) can share a single
// tab dispatcher.
//
// Update returns Page (not the concrete type) so pagerModel can store
// the updated page back into its slice without a type switch. Focus
// returns a tea.Cmd so pages can kick off async work (e.g. refresh
// loads) when they become active; Blur is side-effectful only.
type Page interface {
	Name() string
	Init() tea.Cmd
	Update(tea.Msg) (Page, tea.Cmd)
	View() string
	Focus() tea.Cmd
	Blur()
	SetSize(width, height int)
}
