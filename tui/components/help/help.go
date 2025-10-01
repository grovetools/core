package help

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattsolo1/grove-core/tui/keymap"
	"github.com/mattsolo1/grove-core/tui/theme"
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
}

// New creates a new help model with default settings
func New(keys interface{}) Model {
	return Model{
		Keys:    keys,
		ShowAll: false,
		Theme:   theme.DefaultTheme,
	}
}

// Update handles messages for the help component
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
	}
	return m, nil
}

// View renders the help component
func (m Model) View() string {
	if m.Theme == nil {
		m.Theme = theme.DefaultTheme
	}

	var helpGroups [][]key.Binding
	var shortHelpGroup []key.Binding

	// Extract keybindings based on the type of Keys
	switch k := m.Keys.(type) {
	case keymap.Base:
		helpGroups = k.FullHelp()
		shortHelpGroup = k.ShortHelp()
	case interface {
		FullHelp() [][]key.Binding
		ShortHelp() []key.Binding
	}:
		helpGroups = k.FullHelp()
		shortHelpGroup = k.ShortHelp()
	case interface{ FullHelp() [][]key.Binding }:
		helpGroups = k.FullHelp()
	case interface{ ShortHelp() []key.Binding }:
		shortHelpGroup = k.ShortHelp()
		// If only short help is provided, use it for the full view as well
		if len(helpGroups) == 0 {
			helpGroups = [][]key.Binding{shortHelpGroup}
		}
	}

	// Add custom help if provided
	if m.CustomHelp != nil {
		helpGroups = append(helpGroups, m.CustomHelp...)
	}

	if m.ShowAll {
		return m.viewFull(helpGroups)
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

// viewFull renders the full, multi-column, centered help dialog.
func (m Model) viewFull(groups [][]key.Binding) string {
	if len(groups) == 0 {
		return ""
	}

	// Styles
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.DefaultColors.Border).
		Padding(1, 2).
		Width(80). // Max width for the help box
		Align(lipgloss.Center)

	titleText := m.Title
	if titleText == "" {
		titleText = "Help"
	}
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(m.Theme.Info.GetForeground()).
		MarginBottom(1)

	columnStyle := lipgloss.NewStyle().
		Width(38). // Width for each column
		Align(lipgloss.Left)

	keyStyle := m.Theme.Highlight
	descStyle := m.Theme.Muted
	sectionTitleStyle := lipgloss.NewStyle().Bold(true).MarginTop(1)

	// Build columns from key binding groups
	var renderedColumns []string
	for _, group := range groups {
		if len(group) == 0 {
			continue
		}

		var columnLines []string
		for _, binding := range group {
			if !binding.Enabled() {
				continue
			}

			key := binding.Help().Key
			desc := binding.Help().Desc

			if key == "" && desc != "" { // This is a section title
				if len(columnLines) > 0 {
					columnLines = append(columnLines, "") // Add space before section
				}
				columnLines = append(columnLines, sectionTitleStyle.Render(desc))
			} else if key != "" && desc != "" {
				pair := fmt.Sprintf("%s %s %s",
					keyStyle.Render(key),
					descStyle.Render("•"),
					descStyle.Render(desc),
				)
				columnLines = append(columnLines, pair)
			}
		}

		// Join lines, render column, and add to list of columns
		if len(columnLines) > 0 {
			renderedColumn := columnStyle.Render(strings.Join(columnLines, "\n"))
			renderedColumns = append(renderedColumns, renderedColumn)
		}
	}

	// Join columns horizontally
	columns := lipgloss.JoinHorizontal(lipgloss.Top, renderedColumns...)

	// Combine title and content
	title := titleStyle.Render(titleText)
	fullContent := lipgloss.JoinVertical(lipgloss.Center, title, columns)

	// Render in a box and center on screen
	dialog := boxStyle.Render(fullContent)
	return lipgloss.NewStyle().
		Width(m.Width).
		Height(m.Height).
		Align(lipgloss.Center, lipgloss.Center).
		Render(dialog)
}

// Toggle toggles between showing all help and short help
func (m *Model) Toggle() {
	m.ShowAll = !m.ShowAll
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