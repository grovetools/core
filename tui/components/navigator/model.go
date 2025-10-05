package navigator

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-core/tui/components/help"
	core_theme "github.com/mattsolo1/grove-core/tui/theme"
)

// Model is the generic navigator component model
type Model struct {
	projects        []workspace.ProjectInfo
	filtered        []workspace.ProjectInfo
	selected        workspace.ProjectInfo
	cursor          int
	filterInput     textinput.Model
	width           int
	height          int
	help            help.Model
	pathDisplayMode int // 0=no paths, 1=compact (~), 2=full paths

	// Focus mode state
	ecosystemPickerMode bool                       // True when showing only ecosystems for selection
	focusedProject      *workspace.ProjectInfo
	worktreesFolded     bool // Whether worktrees are hidden/collapsed

	// --- Callbacks for customization ---
	// OnSelect is called when a project is selected (Enter). The returned command is executed.
	OnSelect func(workspace.ProjectInfo) tea.Cmd

	// RenderGutter allows the parent to define what status indicators appear
	// to the left of each project name. It is passed the project and whether it's the currently selected item.
	RenderGutter func(project workspace.ProjectInfo, isSelected bool) string

	// CustomKeyHandler allows the parent to intercept and handle key presses
	// before the navigator's default keymap. If it returns a non-nil command, the navigator executes it.
	CustomKeyHandler func(m Model, msg tea.KeyMsg) (Model, tea.Cmd)

	// ProjectsLoader is a function that the navigator calls to refresh its project list.
	ProjectsLoader func() ([]workspace.ProjectInfo, error)

	// RefreshInterval controls how often the navigator auto-refreshes (in seconds). 0 = no auto-refresh.
	RefreshInterval int

	// -- Internal State --
	initialProjects []workspace.ProjectInfo // The full, unfiltered list of projects
}

// Config defines the configuration for the navigator.
type Config struct {
	Projects []workspace.ProjectInfo
}

// New creates a new navigator model with the given configuration.
func New(cfg Config) Model {
	ti := textinput.New()
	ti.Placeholder = "Press / to filter..."
	ti.CharLimit = 256
	ti.Width = 50

	helpModel := help.NewBuilder().
		WithKeys(defaultKeyMap). // Use the new generic keymap
		WithTitle("Project Navigator").
		Build()

	m := Model{
		initialProjects: cfg.Projects,
		projects:        cfg.Projects,
		filtered:        cfg.Projects,
		filterInput:     ti,
		cursor:          0,
		help:            helpModel,
		pathDisplayMode: 1, // Default to compact paths (~)
	}
	m.updateFiltered()
	return m
}

// Init initializes the navigator.
// The parent application is now responsible for sending the initial ProjectsLoadedMsg.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages and updates the model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.SetSize(msg.Width, msg.Height)
		return m, nil

	case ProjectsLoadedMsg:
		// Save the current selected project path
		selectedPath := ""
		if m.cursor < len(m.filtered) {
			selectedPath = m.filtered[m.cursor].Path
		}

		// Update the main project list
		m.projects = msg.Projects

		// Update the filtered list
		m.updateFiltered()

		// Try to restore cursor position
		if selectedPath != "" {
			for i, p := range m.filtered {
				if p.Path == selectedPath {
					m.cursor = i
					break
				}
			}
		}

		// Clamp cursor to valid range
		if m.cursor >= len(m.filtered) {
			m.cursor = len(m.filtered) - 1
		}
		if m.cursor < 0 {
			m.cursor = 0
		}

		return m, nil

	case tickMsg:
		// The parent can choose to send ticks to trigger a refresh
		return m, m.RefreshProjectsCmd()

	case tea.KeyMsg:
		// If help is visible, it consumes all key presses
		if m.help.ShowAll {
			m.help.Toggle() // Any key closes help
			return m, nil
		}

		// Give the parent a chance to handle the key press first.
		// If the handler returns a command, we execute it and stop processing the key here.
		if m.CustomKeyHandler != nil {
			if newModel, cmd := m.CustomKeyHandler(m, msg); cmd != nil {
				return newModel, cmd
			}
		}

		// Handle main key bindings before specific modes
		switch {
		case key.Matches(msg, defaultKeyMap.FocusEcosystem):
			// Enter ecosystem picker mode
			m.ecosystemPickerMode = true
			m.updateFiltered() // Will filter to ecosystems only
			m.cursor = 0
			return m, nil

		case key.Matches(msg, defaultKeyMap.ToggleWorktrees):
			m.worktreesFolded = !m.worktreesFolded
			m.updateFiltered()
			return m, nil

		case key.Matches(msg, defaultKeyMap.ClearFocus):
			if m.focusedProject != nil {
				m.focusedProject = nil
				m.updateFiltered()
				m.cursor = 0
			}
			return m, nil
		}

		// Check if filter input is focused and handle special keys
		if m.filterInput.Focused() {
			switch msg.Type {
			case tea.KeyEsc:
				if m.ecosystemPickerMode {
					m.ecosystemPickerMode = false
					m.updateFiltered()
					return m, nil
				}
				m.filterInput.Blur()
				return m, nil
			case tea.KeyEnter:
				// Handle ecosystem picker mode
				if m.ecosystemPickerMode {
					if m.cursor < len(m.filtered) {
						// Make a copy to avoid pointer issues
						selected := m.filtered[m.cursor]
						m.focusedProject = &selected
						m.ecosystemPickerMode = false
						m.updateFiltered() // Now filter to focused ecosystem
						m.cursor = 0
					}
					return m, nil
				}
				// Select current project even while filtering
				if m.cursor < len(m.filtered) {
					m.selected = m.filtered[m.cursor]
					if m.OnSelect != nil {
						// Delegate the selection action to the parent component.
						return m, m.OnSelect(m.selected)
					}
					// Default behavior is to quit, returning the selected item.
					return m, tea.Quit
				}
				return m, nil
			case tea.KeyUp:
				// Navigate up while filtering
				if m.cursor > 0 {
					m.cursor--
				}
				return m, nil
			case tea.KeyDown:
				// Navigate down while filtering
				if m.cursor < len(m.filtered)-1 {
					m.cursor++
				}
				return m, nil
			default:
				// Let filter input handle all other keys when focused
				prevValue := m.filterInput.Value()
				m.filterInput, cmd = m.filterInput.Update(msg)

				// If the filter changed, update filtered list
				if m.filterInput.Value() != prevValue {
					m.updateFiltered()
					m.cursor = 0
				}
				return m, cmd
			}
		}

		// Normal mode (when filter is not focused)
		switch msg.Type {
		case tea.KeyUp, tea.KeyCtrlP:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyDown, tea.KeyCtrlN:
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
		case tea.KeyCtrlU:
			// Page up (vim-style)
			pageSize := 10
			m.cursor -= pageSize
			if m.cursor < 0 {
				m.cursor = 0
			}
		case tea.KeyCtrlD:
			// Page down (vim-style)
			pageSize := 10
			m.cursor += pageSize
			if m.cursor >= len(m.filtered) {
				m.cursor = len(m.filtered) - 1
			}
			if m.cursor < 0 {
				m.cursor = 0
			}
		case tea.KeyRunes:
			switch msg.String() {
			case "j":
				// Vim-style down navigation
				if m.cursor < len(m.filtered)-1 {
					m.cursor++
				}
				return m, nil
			case "k":
				// Vim-style up navigation
				if m.cursor > 0 {
					m.cursor--
				}
				return m, nil
			case "g":
				// Go to top
				m.cursor = 0
				return m, nil
			case "G":
				// Go to bottom
				m.cursor = len(m.filtered) - 1
				if m.cursor < 0 {
					m.cursor = 0
				}
				return m, nil
			case "?":
				m.help.Toggle()
				return m, nil
			case "/":
				// Focus filter input for search
				m.filterInput.Focus()
				return m, textinput.Blink
			}
		case tea.KeyEnter:
			// Handle ecosystem picker mode
			if m.ecosystemPickerMode {
				if m.cursor < len(m.filtered) {
					// Make a copy to avoid pointer issues
					selected := m.filtered[m.cursor]
					m.focusedProject = &selected
					m.ecosystemPickerMode = false
					m.updateFiltered() // Now filter to focused ecosystem
					m.cursor = 0
				}
				return m, nil
			}
			// Normal mode - select project and quit
			if m.cursor < len(m.filtered) {
				m.selected = m.filtered[m.cursor]
				if m.OnSelect != nil {
					// Delegate the selection action to the parent component.
					return m, m.OnSelect(m.selected)
				}
				// Default behavior is to quit, returning the selected item.
				return m, tea.Quit
			}
		case tea.KeyEsc, tea.KeyCtrlC:
			return m, tea.Quit
		default:
			// Handle other keys normally
			return m, nil
		}
	}

	return m, nil
}

// View renders the navigator.
func (m Model) View() string {
	// If help is visible, show it and return
	if m.help.ShowAll {
		return m.help.View()
	}

	var b strings.Builder

	// Header with filter input (always at top)
	var header strings.Builder
	if m.ecosystemPickerMode {
		header.WriteString(core_theme.DefaultTheme.Info.Render("[Select Ecosystem to Focus]"))
		header.WriteString(" ")
	} else if m.focusedProject != nil {
		focusIndicator := core_theme.DefaultTheme.Info.Render("[Focus: " + m.focusedProject.Name + "]")
		header.WriteString(focusIndicator)
		header.WriteString(" ")
	}
	header.WriteString(m.filterInput.View())
	b.WriteString(header.String())
	b.WriteString("\n\n")

	// Calculate visible items based on terminal height
	visibleHeight := m.height - 6
	if visibleHeight < 5 {
		visibleHeight = 5 // Minimum visible items
	}

	// Determine visible range with scrolling
	start := 0
	end := len(m.filtered)

	// Implement scrolling if there are too many items
	if end > visibleHeight {
		// Center the cursor in the visible area when possible
		if m.cursor < visibleHeight/2 {
			// Near the top
			start = 0
		} else if m.cursor >= len(m.filtered)-visibleHeight/2 {
			// Near the bottom
			start = len(m.filtered) - visibleHeight
		} else {
			// Middle - center the cursor
			start = m.cursor - visibleHeight/2
		}

		end = start + visibleHeight
		if end > len(m.filtered) {
			end = len(m.filtered)
		}
		if start < 0 {
			start = 0
		}
	}

	// Render visible projects
	for i := start; i < end && i < len(m.filtered); i++ {
		project := m.filtered[i]
		isSelected := i == m.cursor

		var line strings.Builder

		// Cursor
		if isSelected {
			line.WriteString(core_theme.DefaultTheme.Highlight.Render("▶ "))
		} else {
			line.WriteString("  ")
		}

		// Gutter (customizable by parent)
		if m.RenderGutter != nil {
			line.WriteString(m.RenderGutter(project, isSelected))
		}

		// Project Name
		displayName := project.Name

		// Highlight matching search terms
		filter := strings.ToLower(m.filterInput.Value())
		if filter != "" {
			displayName = highlightMatch(displayName, filter)
		}

		// Apply selection style to the whole line
		if isSelected {
			line.WriteString(core_theme.DefaultTheme.Selected.Render(displayName))
		} else {
			// Style based on project type
			if project.IsWorktree {
				line.WriteString(core_theme.DefaultTheme.Info.Render(displayName))
			} else {
				line.WriteString(core_theme.DefaultTheme.Highlight.Copy().Bold(true).Render(displayName))
			}
		}

		b.WriteString(line.String())
		b.WriteString("\n")
	}

	// Show scroll indicators if needed
	if start > 0 || end < len(m.filtered) {
		b.WriteString(core_theme.DefaultTheme.Muted.Render(" (") +
			strings.Join([]string{
				strings.Join([]string{
					strings.Join([]string{string(rune(start + 1)), string(rune(end))}, "-"),
					"of",
					string(rune(len(m.filtered))),
				}, " "),
			}, "") + ")")
	}

	// Help text at bottom
	if len(m.filtered) == 0 {
		if len(m.projects) == 0 {
			b.WriteString("\n" + core_theme.DefaultTheme.Muted.Render("No projects found"))
		} else {
			b.WriteString("\n" + core_theme.DefaultTheme.Muted.Render("No matching projects"))
		}
	}

	// Help text
	helpStyle := core_theme.DefaultTheme.Muted
	b.WriteString("\n")

	if m.ecosystemPickerMode {
		b.WriteString(helpStyle.Render("Enter to select • Esc to cancel"))
	} else if m.focusedProject != nil {
		b.WriteString(helpStyle.Render("Press ? for help • Press ctrl+g to clear focus"))
	} else {
		b.WriteString(helpStyle.Render("Press ? for help"))
	}

	return b.String()
}

// GetSelected returns the currently selected project.
func (m Model) GetSelected() workspace.ProjectInfo {
	return m.selected
}

// updateFiltered applies the current filter and focus mode to update the filtered project list.
func (m *Model) updateFiltered() {
	filter := strings.ToLower(m.filterInput.Value())

	// Handle ecosystem picker mode - show ecosystems with their worktrees in a tree
	if m.ecosystemPickerMode {
		m.filtered = []workspace.ProjectInfo{}

		for _, p := range m.projects {
			if !p.IsEcosystem {
				continue
			}

			// Apply filter
			matchesFilter := filter == "" ||
				strings.Contains(strings.ToLower(p.Name), filter) ||
				strings.Contains(strings.ToLower(p.Path), filter)

			if matchesFilter {
				m.filtered = append(m.filtered, p)
			}
		}
		return
	}

	// Create a working list of projects, either all projects or just the focused ecosystem
	var projectsToFilter []workspace.ProjectInfo
	if m.focusedProject != nil && m.focusedProject.IsEcosystem {
		// Focus is active on an ecosystem - show it and all its children
		projectsToFilter = append(projectsToFilter, *m.focusedProject)

		// Include child projects
		for _, p := range m.projects {
			if p.ParentEcosystemPath == m.focusedProject.Path && p.Path != m.focusedProject.Path {
				projectsToFilter = append(projectsToFilter, p)
			}
		}
	} else if m.focusedProject != nil {
		// Focused on something that is not an ecosystem (a regular project)
		projectsToFilter = append(projectsToFilter, *m.focusedProject)
	} else {
		// No focus, use all projects
		projectsToFilter = m.projects
	}

	if filter == "" {
		// No filter - just use the working list
		m.filtered = projectsToFilter
	} else {
		// Apply filter
		m.filtered = []workspace.ProjectInfo{}
		for _, p := range projectsToFilter {
			lowerName := strings.ToLower(p.Name)
			if lowerName == filter || strings.HasPrefix(lowerName, filter) ||
				strings.Contains(lowerName, filter) {
				m.filtered = append(m.filtered, p)
			}
		}
	}
}

// highlightMatch highlights the matching portion of a string
func highlightMatch(s, filter string) string {
	if filter == "" {
		return s
	}

	lowerS := strings.ToLower(s)
	idx := strings.Index(lowerS, filter)
	if idx == -1 {
		return s
	}

	before := s[:idx]
	match := s[idx : idx+len(filter)]
	after := s[idx+len(filter):]

	return before + core_theme.DefaultTheme.Success.Render(match) + after
}
