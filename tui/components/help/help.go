package help

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/groveorg/grove-core/tui/keymap"
	"github.com/groveorg/grove-core/tui/theme"
)

// Model represents an embeddable help component
type Model struct {
	Keys       interface{} // Can be keymap.Base or any extended keymap
	ShowAll    bool
	Width      int
	Height     int
	Theme      *theme.Theme
	CustomHelp [][]key.Binding // Additional custom key bindings to display
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

	// Extract keybindings based on the type of Keys
	switch k := m.Keys.(type) {
	case keymap.Base:
		if m.ShowAll {
			helpGroups = k.FullHelp()
		} else {
			helpGroups = [][]key.Binding{k.ShortHelp()}
		}
	case interface{ FullHelp() [][]key.Binding }:
		if m.ShowAll {
			helpGroups = k.FullHelp()
		}
	case interface{ ShortHelp() []key.Binding }:
		if !m.ShowAll {
			helpGroups = [][]key.Binding{k.ShortHelp()}
		}
	}

	// Add custom help if provided
	if m.CustomHelp != nil {
		helpGroups = append(helpGroups, m.CustomHelp...)
	}

	if len(helpGroups) == 0 {
		return ""
	}

	// Build the help content
	var sections []string

	for _, group := range helpGroups {
		if len(group) == 0 {
			continue
		}

		var pairs []string
		for _, binding := range group {
			if !binding.Enabled() {
				continue
			}

			keys := binding.Help().Key
			desc := binding.Help().Desc

			if keys != "" && desc != "" {
				// Format: "key • description"
				pair := fmt.Sprintf("%s %s %s",
					m.Theme.Highlight.Render(keys),
					m.Theme.Muted.Render("•"),
					m.Theme.Muted.Render(desc),
				)
				pairs = append(pairs, pair)
			}
		}

		if len(pairs) > 0 {
			sections = append(sections, strings.Join(pairs, "  "))
		}
	}

	if len(sections) == 0 {
		return ""
	}

	// Join all sections with line breaks
	content := strings.Join(sections, "\n")

	// Create a box for the help content
	helpBox := m.Theme.Box.
		Width(m.Width - 4).
		BorderForeground(theme.Border)

	if !m.ShowAll {
		// Short help - single line
		helpLine := m.Theme.Muted.Render("Press ") +
			m.Theme.Highlight.Render("?") +
			m.Theme.Muted.Render(" for help • ") +
			sections[0]
		return helpLine
	}

	// Full help - boxed multi-line
	title := m.Theme.Header.Render(" Help ")
	helpBox = helpBox.BorderTop(true).
		BorderLeft(true).
		BorderRight(true).
		BorderBottom(true)

	// Add title to the box
	return lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		helpBox.Render(content),
	)
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

// ShowAll sets whether to show all help initially
func (b *Builder) ShowAll(show bool) *Builder {
	b.model.ShowAll = show
	return b
}

// Build creates the help model
func (b *Builder) Build() Model {
	return b.model
}