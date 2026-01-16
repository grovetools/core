package help

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	ltable "github.com/charmbracelet/lipgloss/table"
	"github.com/grovetools/core/tui/keymap"
	"github.com/grovetools/core/tui/theme"
)

// Model represents an embeddable help component
type Model struct {
	Keys       interface{} // Can be keymap.Base or any extended keymap
	ShowAll    bool
	Width      int
	Height     int
	Theme      *theme.Theme
	CustomHelp [][]key.Binding // Additional custom key bindings to display
	Title      string      // Title for the full help view
	viewport   viewport.Model
}

// New creates a new help model with default settings
func New(keys interface{}) Model {
	vp := viewport.New(0, 0)
	// Disable mouse events for the viewport by default, as it can interfere
	// with the main application's mouse handling.
	vp.MouseWheelEnabled = false
	return Model{
		Keys:     keys,
		ShowAll:  false,
		Theme:    theme.DefaultTheme,
		viewport: vp,
	}
}

// Update handles messages for the help component
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		if m.ShowAll {
			m.setViewportContent()
		}

	case tea.KeyMsg:
		if m.ShowAll {
			// Get standard keys for closing the view
			helpBinding := m.getHelpBinding()
			quitBinding := m.getQuitBinding()

			// Close on help, quit, or escape keys
			if key.Matches(msg, helpBinding) || key.Matches(msg, quitBinding) || msg.Type == tea.KeyEsc {
				m.Toggle()
				return m, nil
			}

			// Pass all other messages to the viewport for scrolling
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

// View renders the help component
func (m Model) View() string {
	if m.Theme == nil {
		m.Theme = theme.DefaultTheme
	}

	if m.ShowAll {
		// Render the viewport, centered on the screen to create a modal effect.
		return lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center,
			m.viewport.View(),
		)
	}

	// For the short view, get the appropriate key group
	var shortHelpGroup []key.Binding
	switch k := m.Keys.(type) {
	case interface {
		ShortHelp() []key.Binding
	}:
		shortHelpGroup = k.ShortHelp()
	}

	return m.viewShort(shortHelpGroup)
}

// viewShort renders the compact, single-line help view.
func (m Model) viewShort(group []key.Binding) string {
	if len(group) == 0 {
		return ""
	}

	var pairs []string
	for _, binding := range group {
		if !binding.Enabled() {
			continue
		}
		keys := binding.Help().Key
		desc := binding.Help().Desc
		if keys != "" && desc != "" {
			pair := fmt.Sprintf("%s %s %s",
				m.Theme.Highlight.Render(keys),
				m.Theme.Muted.Render("•"),
				m.Theme.Muted.Render(desc),
			)
			pairs = append(pairs, pair)
		}
	}

	if len(pairs) == 0 {
		return ""
	}

	// Default help prompt
	helpPrompt := m.Theme.Muted.Render("Press ") +
		m.Theme.Highlight.Render("?") +
		m.Theme.Muted.Render(" for help")

	return helpPrompt + " • " + strings.Join(pairs, " • ")
}

// setViewportContent renders the help content, determines the best layout
// (single or dual column), and sets it in the viewport.
func (m *Model) setViewportContent() {
	const (
		verticalMargin   = 4
		horizontalMargin = 4
		gutterWidth      = 4
	)

	// Get all keybinding groups
	var helpGroups [][]key.Binding
	switch k := m.Keys.(type) {
	case keymap.Base:
		helpGroups = k.FullHelp()
	case interface{ FullHelp() [][]key.Binding }:
		helpGroups = k.FullHelp()
	}
	if m.CustomHelp != nil {
		helpGroups = append(helpGroups, m.CustomHelp...)
	}

	content := m.renderHelpContent(helpGroups, verticalMargin, horizontalMargin, gutterWidth)
	m.viewport.SetContent(content)

	// Set viewport dimensions with a margin
	m.viewport.Width = lipgloss.Width(content)
	m.viewport.Height = m.Height - verticalMargin
}

// renderHelpContent tries to fit the help text into the available space,
// first as a single column, then two columns, before falling back to a
// scrollable single column view.
func (m *Model) renderHelpContent(groups [][]key.Binding, vMargin, hMargin, gutter int) string {
	// 1. Try single-column layout
	singleCol := m.renderTable(groups)
	if lipgloss.Height(singleCol) <= m.Height-vMargin {
		return singleCol
	}

	// 2. Try two-column layout if content is too tall
	// Collect all rows first to split them evenly
	allRows := m.collectRows(groups)
	if len(allRows) == 0 {
		return ""
	}

	// Split rows in half for balanced columns
	mid := len(allRows) / 2
	leftRows := allRows[:mid]
	rightRows := allRows[mid:]

	leftCol := m.renderTableFromRows(leftRows)
	rightCol := m.renderTableFromRows(rightRows)
	twoCol := lipgloss.JoinHorizontal(lipgloss.Top, leftCol, strings.Repeat(" ", gutter), rightCol)

	// Add title above the two-column layout
	titleText := m.Title
	if titleText == "" {
		titleText = "Help"
	}
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(m.Theme.Info.GetForeground()).
		MarginBottom(1).
		Align(lipgloss.Center)

	twoColWithTitle := lipgloss.JoinVertical(lipgloss.Center, titleStyle.Render(titleText), twoCol)

	if lipgloss.Height(twoColWithTitle) <= m.Height-vMargin && lipgloss.Width(twoColWithTitle) <= m.Width-hMargin {
		return twoColWithTitle
	}

	// 3. Fallback to scrollable single-column layout
	return singleCol
}

// collectRows collects all rows from all keybinding groups.
func (m *Model) collectRows(groups [][]key.Binding) [][]string {
	var rows [][]string
	for _, group := range groups {
		if len(group) == 0 {
			continue
		}
		for _, binding := range group {
			if !binding.Enabled() {
				continue
			}
			keyStr := binding.Help().Key
			desc := binding.Help().Desc
			if keyStr == "" && desc != "" {
				rows = append(rows, []string{m.Theme.Info.Bold(true).Render(desc), ""})
			} else if keyStr != "" && desc != "" {
				rows = append(rows, []string{
					m.Theme.Highlight.Render(keyStr),
					m.Theme.Muted.Render(desc),
				})
			}
		}
	}
	return rows
}

// renderTableFromRows renders a set of pre-collected rows into a styled table.
func (m *Model) renderTableFromRows(rows [][]string) string {
	if len(rows) == 0 {
		return ""
	}

	table := ltable.New().
		Border(lipgloss.HiddenBorder()).
		StyleFunc(func(row, col int) lipgloss.Style {
			return lipgloss.NewStyle().Padding(0, 1)
		})

	for _, row := range rows {
		table = table.Row(row...)
	}

	return table.String()
}

// renderTable renders a set of keybinding groups into a styled table string.
func (m *Model) renderTable(groups [][]key.Binding) string {
	if len(groups) == 0 {
		return ""
	}

	titleText := m.Title
	if titleText == "" {
		titleText = "Help"
	}
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(m.Theme.Info.GetForeground()).
		MarginBottom(1).
		Align(lipgloss.Left) // Align left for table headers

	rows := m.collectRows(groups)
	if len(rows) == 0 {
		return ""
	}

	return lipgloss.JoinVertical(lipgloss.Left, titleStyle.Render(titleText), m.renderTableFromRows(rows))
}

// getHelpBinding retrieves the help keybinding from the model's Keys interface.
func (m *Model) getHelpBinding() key.Binding {
	switch k := m.Keys.(type) {
	case keymap.Base:
		return k.Help
	case interface{ GetHelp() key.Binding }:
		return k.GetHelp()
	}
	return key.NewBinding(key.WithKeys("?")) // Fallback
}

// getQuitBinding retrieves the quit keybinding from the model's Keys interface.
func (m *Model) getQuitBinding() key.Binding {
	switch k := m.Keys.(type) {
	case keymap.Base:
		return k.Quit
	case interface{ GetQuit() key.Binding }:
		return k.GetQuit()
	}
	return key.NewBinding(key.WithKeys("q")) // Fallback
}

// Toggle toggles between showing all help and short help. When showing, it
// recalculates content layout and resets the scroll position.
func (m *Model) Toggle() {
	m.ShowAll = !m.ShowAll
	if m.ShowAll {
		m.setViewportContent()
		m.viewport.GotoTop()
	}
}

// SetSize sets the dimensions of the help view
func (m *Model) SetSize(width, height int) {
	m.Width = width
	m.Height = height
}

// SetKeys updates the keymap for the help view
func (m *Model) SetKeys(keys interface{}) {
	m.Keys = keys
}

// SetCustomHelp sets additional custom key bindings to display
func (m *Model) SetCustomHelp(customHelp [][]key.Binding) {
	m.CustomHelp = customHelp
}

// Builder provides a fluent interface for creating help models
type Builder struct {
	model Model
}

// NewBuilder creates a new help builder
func NewBuilder() *Builder {
	return &Builder{
		model: Model{
			Theme: theme.DefaultTheme,
		},
	}
}

// WithKeys sets the keymap
func (b *Builder) WithKeys(keys interface{}) *Builder {
	b.model.Keys = keys
	return b
}

// WithTheme sets the theme
func (b *Builder) WithTheme(t *theme.Theme) *Builder {
	b.model.Theme = t
	return b
}

// WithSize sets the initial size
func (b *Builder) WithSize(width, height int) *Builder {
	b.model.Width = width
	b.model.Height = height
	return b
}

// WithCustomHelp adds custom help bindings
func (b *Builder) WithCustomHelp(customHelp [][]key.Binding) *Builder {
	b.model.CustomHelp = customHelp
	return b
}

// WithTitle sets the title for the full help view dialog
func (b *Builder) WithTitle(title string) *Builder {
	b.model.Title = title
	return b
}

// ShowAll sets whether to show all help initially
func (b *Builder) ShowAll(show bool) *Builder {
	b.model.ShowAll = show
	return b
}

// Build creates the help model
func (b *Builder) Build() Model {
	return b.model
}