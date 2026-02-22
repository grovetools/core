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
		content := m.viewport.View()

		// Add scroll indicator if viewport is scrollable
		if m.viewport.TotalLineCount() > m.viewport.Height {
			indicator := ""
			if m.viewport.AtTop() {
				indicator = "↓ more"
			} else if m.viewport.AtBottom() {
				indicator = "↑ more"
			} else {
				indicator = "↕ more"
			}

			indicatorStyle := m.Theme.Muted.Align(lipgloss.Right).Width(m.viewport.Width)
			content = lipgloss.JoinVertical(lipgloss.Right, content, indicatorStyle.Render(indicator))
		}

		// Render the viewport, centered on the screen to create a modal effect.
		return lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, content)
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

	var sections []keymap.Section
	var helpGroups [][]key.Binding // fallback for older implementation

	// Prefer SectionedKeyMap interface, fall back to FullHelp
	switch k := m.Keys.(type) {
	case keymap.SectionedKeyMap:
		sections = k.Sections()
	case keymap.Base:
		sections = k.Sections()
	case interface{ FullHelp() [][]key.Binding }:
		helpGroups = k.FullHelp()
	}

	// Handle custom help bindings
	if m.CustomHelp != nil {
		if len(sections) > 0 {
			// Convert custom help to a section
			var customBindings []key.Binding
			for _, group := range m.CustomHelp {
				customBindings = append(customBindings, group...)
			}
			sections = append(sections, keymap.Section{Name: "Custom", Bindings: customBindings})
		} else {
			helpGroups = append(helpGroups, m.CustomHelp...)
		}
	}

	content := m.renderHelpContent(sections, helpGroups, verticalMargin, horizontalMargin, gutterWidth)
	m.viewport.SetContent(content)

	// Set viewport dimensions with a margin. Reserve 1 line for the scroll indicator.
	m.viewport.Width = lipgloss.Width(content)
	m.viewport.Height = m.Height - verticalMargin - 1
}

// renderHelpContent renders help content, preferring multi-column layout when
// content is tall. The viewport handles scrolling automatically.
func (m *Model) renderHelpContent(sections []keymap.Section, groups [][]key.Binding, vMargin, hMargin, gutter int) string {
	blocks := m.collectSectionBlocks(sections, groups)
	if len(blocks) == 0 {
		return ""
	}

	titleText := m.Title
	if titleText == "" {
		titleText = "Help"
	}
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(m.Theme.Colors.Orange).
		MarginBottom(1).
		Align(lipgloss.Center)

	// 1. Try single-column layout first
	singleCol := lipgloss.JoinVertical(lipgloss.Left, blocks...)
	singleColWithTitle := lipgloss.JoinVertical(lipgloss.Center, titleStyle.Width(lipgloss.Width(singleCol)).Render(titleText), singleCol)

	if lipgloss.Height(singleColWithTitle) <= m.Height-vMargin-1 {
		return singleColWithTitle
	}

	// 2. Content is too tall - try three-column layout first (if we have enough blocks)
	if len(blocks) >= 3 {
		threeCol := m.buildMultiColumnLayout(blocks, 3, gutter)
		threeColWithTitle := lipgloss.JoinVertical(lipgloss.Center, titleStyle.Width(lipgloss.Width(threeCol)).Render(titleText), threeCol)

		// Check if three columns fit width-wise (scrolling handles height)
		if lipgloss.Width(threeColWithTitle) <= m.Width-hMargin {
			return threeColWithTitle
		}
	}

	// 3. Try two-column layout
	twoCol := m.buildMultiColumnLayout(blocks, 2, gutter)
	twoColWithTitle := lipgloss.JoinVertical(lipgloss.Center, titleStyle.Width(lipgloss.Width(twoCol)).Render(titleText), twoCol)

	// Check if two columns fit width-wise (scrolling handles height)
	if lipgloss.Width(twoColWithTitle) <= m.Width-hMargin {
		return twoColWithTitle
	}

	// 4. Fall back to single column (viewport will scroll)
	return singleColWithTitle
}

// buildMultiColumnLayout distributes blocks across n columns using a greedy algorithm.
func (m *Model) buildMultiColumnLayout(blocks []string, numCols, gutter int) string {
	columns := make([][]string, numCols)
	heights := make([]int, numCols)

	// Greedy distribution: add each block to the shortest column
	for _, block := range blocks {
		h := lipgloss.Height(block)

		// Find the shortest column
		minIdx := 0
		for i := 1; i < numCols; i++ {
			if heights[i] < heights[minIdx] {
				minIdx = i
			}
		}

		columns[minIdx] = append(columns[minIdx], block)
		heights[minIdx] += h
	}

	// Render each column
	rendered := make([]string, numCols)
	for i := 0; i < numCols; i++ {
		if len(columns[i]) > 0 {
			rendered[i] = lipgloss.JoinVertical(lipgloss.Left, columns[i]...)
		}
	}

	// Join columns horizontally with gutters
	gutterStr := strings.Repeat(" ", gutter)
	result := rendered[0]
	for i := 1; i < numCols; i++ {
		result = lipgloss.JoinHorizontal(lipgloss.Top, result, gutterStr, rendered[i])
	}

	return result
}

// collectSectionBlocks collects all sections and renders them as individual block strings.
func (m *Model) collectSectionBlocks(sections []keymap.Section, groups [][]key.Binding) []string {
	var blocks []string

	// Style for keys - blue bold to match CLI help styling
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(m.Theme.Colors.Blue)

	// Process sections first (preferred)
	for _, section := range sections {
		if section.IsEmpty() {
			continue
		}
		var rows [][]string
		for _, binding := range section.Bindings {
			if !binding.Enabled() {
				continue
			}
			keyStr := binding.Help().Key
			desc := binding.Help().Desc
			if keyStr != "" && desc != "" {
				rows = append(rows, []string{
					keyStyle.Render(keyStr),
					m.Theme.Muted.Italic(true).Render(desc),
				})
			}
		}
		if len(rows) > 0 {
			blocks = append(blocks, m.renderSectionBox(section.Name, section.Icon, rows))
		}
	}

	// Process legacy groups (fallback for keymaps not using sections)
	for i, group := range groups {
		if len(group) == 0 {
			continue
		}
		var rows [][]string
		sectionName := "Custom"

		for _, binding := range group {
			if !binding.Enabled() {
				continue
			}
			keyStr := binding.Help().Key
			desc := binding.Help().Desc
			// Empty key with description = section header (legacy format)
			if keyStr == "" && desc != "" {
				sectionName = desc
			} else if keyStr != "" && desc != "" {
				rows = append(rows, []string{
					keyStyle.Render(keyStr),
					m.Theme.Muted.Italic(true).Render(desc),
				})
			}
		}
		if len(rows) > 0 {
			if sectionName == "Custom" && i > 0 {
				sectionName = fmt.Sprintf("Custom %d", i+1)
			}
			blocks = append(blocks, m.renderSectionBox(sectionName, "", rows))
		}
	}

	return blocks
}

// renderSectionBox renders a single section into a styled box with a title.
func (m *Model) renderSectionBox(title, icon string, rows [][]string) string {
	table := ltable.New().
		Border(lipgloss.HiddenBorder()).
		StyleFunc(func(row, col int) lipgloss.Style {
			return lipgloss.NewStyle().Padding(0, 1)
		})

	for _, row := range rows {
		table = table.Row(row...)
	}

	// Use provided icon, or fall back to standard icon lookup
	sectionIcon := icon
	if sectionIcon == "" {
		sectionIcon = getSectionIcon(title)
	}

	titleText := fmt.Sprintf("%s %s", sectionIcon, title)
	titleStyle := lipgloss.NewStyle().
		Foreground(m.Theme.Colors.Orange).
		Italic(true).
		MarginBottom(1)

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.Theme.Colors.Border).
		Padding(0, 1).
		MarginBottom(1)

	content := lipgloss.JoinVertical(lipgloss.Left, titleStyle.Render(titleText), table.String())
	return boxStyle.Render(content)
}

// getSectionIcon returns the appropriate icon for a section name.
func getSectionIcon(name string) string {
	switch name {
	// Standard sections
	case keymap.SectionNavigation:
		return theme.IconArrowUpDownBold
	case keymap.SectionActions:
		return theme.IconLightbulb
	case keymap.SectionSearch:
		return theme.IconFolderSearch
	case keymap.SectionSelection:
		return theme.IconSelectAll
	case keymap.SectionView:
		return theme.IconViewDashboard
	case keymap.SectionFold:
		return theme.IconFileTree
	case keymap.SectionSystem:
		return theme.IconGear
	// Common custom sections
	case keymap.SectionFocus:
		return theme.IconFolderEye
	case keymap.SectionFilter:
		return theme.IconFilter
	case keymap.SectionToggle:
		return theme.IconViewDashboard
	case keymap.SectionGit:
		return theme.IconGit
	case keymap.SectionSettings:
		return theme.IconGear
	case keymap.SectionContext:
		return theme.IconCode
	case keymap.SectionRules:
		return theme.IconChecklist
	default:
		return theme.IconSparkle
	}
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