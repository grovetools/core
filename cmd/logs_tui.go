package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	stdlog "log"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/hpcloud/tail"
	"github.com/mattsolo1/grove-core/config"
	"github.com/mattsolo1/grove-core/logging"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-core/tui/components/help"
	"github.com/mattsolo1/grove-core/tui/components/jsontree"
	"github.com/mattsolo1/grove-core/tui/keymap"
	"github.com/mattsolo1/grove-core/tui/theme"
)

// paneFocus tracks which pane has focus
type paneFocus int

const (
	listPane paneFocus = iota
	viewportPane
)

// logItem represents a single log entry
type logItem struct {
	workspace string
	level     string
	message   string
	component string
	timestamp time.Time
	rawData   map[string]interface{}
}

// getThemeLevelStyle returns theme-based styling for log levels
func getThemeLevelStyle(level string) lipgloss.Style {
	switch strings.ToLower(level) {
	case "info":
		return theme.DefaultTheme.Success
	case "warning", "warn":
		return theme.DefaultTheme.Warning
	case "error", "fatal", "panic":
		return theme.DefaultTheme.Error
	case "debug", "trace":
		return theme.DefaultTheme.Muted
	default:
		return lipgloss.NewStyle()
	}
}

// Implement list.Item interface
func (i logItem) Title() string {
	// Compact view: [workspace] [LEVEL] date time [component] message
	wsStyle := getWorkspaceStyle(i.workspace)
	levelStyle := getThemeLevelStyle(i.level)
	timeStyle := theme.DefaultTheme.Muted
	componentStyle := theme.DefaultTheme.Muted.Copy().Bold(true)

	return fmt.Sprintf("%s %s %s %s %s",
		wsStyle.Render(fmt.Sprintf("[%s]", i.workspace)),
		levelStyle.Render(fmt.Sprintf("[%s]", strings.ToUpper(i.level))),
		timeStyle.Render(i.timestamp.Format("2006-01-02 15:04:05")),
		componentStyle.Render(fmt.Sprintf("[%s]", i.component)),
		i.message,
	)
}

func (i logItem) Description() string {
	// We don't use description anymore since details are shown in viewport
	return ""
}

func (i logItem) FilterValue() string {
	// Only search the component field
	return i.component
}

// FormatDetails returns a formatted string of the log entry details for the viewport
func (i logItem) FormatDetails() string {
	var lines []string

	// Header with basic info
	headerStyle := theme.DefaultTheme.Header
	lines = append(lines, headerStyle.Render("Log Entry Details"))
	lines = append(lines, "")

	// Basic info
	wsStyle := getWorkspaceStyle(i.workspace)
	levelStyle := getThemeLevelStyle(i.level)
	timeStyle := theme.DefaultTheme.Muted
	componentStyle := theme.DefaultTheme.Muted.Copy().Bold(true)

	lines = append(lines, fmt.Sprintf("Workspace:  %s", wsStyle.Render(i.workspace)))
	lines = append(lines, fmt.Sprintf("Level:      %s", levelStyle.Render(strings.ToUpper(i.level))))
	lines = append(lines, fmt.Sprintf("Component:  %s", componentStyle.Render(i.component)))
	lines = append(lines, fmt.Sprintf("Time:       %s", timeStyle.Render(i.timestamp.Format("2006-01-02 15:04:05"))))
	lines = append(lines, fmt.Sprintf("Message:    %s", i.message))

	// Show pretty_ansi output if present (styled CLI output from unified logger)
	if prettyAnsi, ok := i.rawData["pretty_ansi"].(string); ok && prettyAnsi != "" {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("Output:     %s", prettyAnsi))
	}

	lines = append(lines, "")

	// Standard fields we've already shown (including pretty_* which are shown above)
	standardFields := map[string]bool{
		"level": true, "msg": true, "component": true, "time": true, "_verbosity": true,
		"pretty_ansi": true, "pretty_text": true,
	}

	// Special fields to show separately
	var fileInfo, funcInfo string
	if file, ok := i.rawData["file"].(string); ok {
		fileInfo = file
	}
	if fn, ok := i.rawData["func"].(string); ok {
		funcInfo = fn
	}

	// Extract verbosity metadata
	var verbosityMap map[string]int
	if verbosityRaw, exists := i.rawData["_verbosity"]; exists {
		if verbosityMapInterface, ok := verbosityRaw.(map[string]interface{}); ok {
			verbosityMap = make(map[string]int)
			for key, val := range verbosityMapInterface {
				if intVal, ok := val.(float64); ok {
					verbosityMap[key] = int(intVal)
				}
			}
		}
	}

	// Build the display
	fieldStyle := theme.DefaultTheme.Muted
	fileStyle := theme.DefaultTheme.Muted
	borderStyle := theme.DefaultTheme.Muted

	// Add file/func info if present
	if fileInfo != "" || funcInfo != "" {
		lines = append(lines, borderStyle.Render("┌─ Source:"))
		if fileInfo != "" {
			lines = append(lines, fileStyle.Render(fmt.Sprintf("│ %s %s", theme.IconArchive, fileInfo)))
		}
		if funcInfo != "" {
			lines = append(lines, fileStyle.Render(fmt.Sprintf("│ %s %s", theme.IconShell, funcInfo)))
		}
	}

	// Categorize fields by verbosity level
	fieldsByLevel := map[int][]string{
		0: {}, // basic
		1: {}, // verbose
		2: {}, // debug
		3: {}, // metrics
	}

	for key, value := range i.rawData {
		if !standardFields[key] && key != "file" && key != "func" {
			// Format the value
			var formattedValue string
			switch v := value.(type) {
			case map[string]interface{}, []interface{}:
				jsonBytes, err := json.MarshalIndent(v, "", "  ")
				if err == nil {
					// Prepend newline to push JSON block below the field key
					formattedValue = "\n" + string(jsonBytes)
				} else {
					formattedValue = fmt.Sprintf("%v", v) // Fallback
				}
			case string:
				formattedValue = v
			case float64:
				if v == float64(int64(v)) {
					formattedValue = fmt.Sprintf("%.0f", v)
				} else {
					formattedValue = fmt.Sprintf("%.2f", v)
				}
			case bool:
				formattedValue = fmt.Sprintf("%t", v)
			default:
				formattedValue = fmt.Sprintf("%v", v)
			}

			// Determine verbosity level
			verbosityLevel := 0
			if verbosityMap != nil {
				if level, exists := verbosityMap[key]; exists {
					verbosityLevel = level
				}
			}

			if verbosityLevel < 4 {
				fieldsByLevel[verbosityLevel] = append(fieldsByLevel[verbosityLevel], fmt.Sprintf("%-20s %s", key+":", formattedValue))
			}
		}
	}

	// Add fields if present
	hasFields := false
	for _, fields := range fieldsByLevel {
		if len(fields) > 0 {
			hasFields = true
			break
		}
	}

	if hasFields {
		if fileInfo != "" || funcInfo != "" {
			lines = append(lines, borderStyle.Render("├─ Fields:"))
		} else {
			lines = append(lines, borderStyle.Render("┌─ Fields:"))
		}

		// Sort fields within each level for consistency
		for level := 0; level < 4; level++ {
			if fields := fieldsByLevel[level]; len(fields) > 0 {
				sort.Strings(fields)
				for i, field := range fields {
					isLast := (level == 3 || len(fieldsByLevel[level+1]) == 0) && i == len(fields)-1
					// Check if this is truly the last field across all levels
					hasMoreFields := false
					for checkLevel := level + 1; checkLevel < 4; checkLevel++ {
						if len(fieldsByLevel[checkLevel]) > 0 {
							hasMoreFields = true
							break
						}
					}

					if isLast && !hasMoreFields {
						lines = append(lines, fieldStyle.Render(fmt.Sprintf("└─ %s", field)))
					} else {
						lines = append(lines, fieldStyle.Render(fmt.Sprintf("├─ %s", field)))
					}
				}
			}
		}
	}

	return strings.Join(lines, "\n")
}

// Custom item delegate for rendering
type itemDelegate struct {
	model *logModel
}

func (d itemDelegate) Height() int                               { return 1 }
func (d itemDelegate) Spacing() int                              { return 0 }
func (d itemDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd { return nil }

func (d itemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	i, ok := item.(logItem)
	if !ok {
		return
	}

	str := i.Title()

	// Check if this item is in visual selection range
	// Note: index here is the index within the visible/filtered items
	isVisuallySelected := false
	if d.model != nil && d.model.visualMode {
		minIdx := d.model.visualStart
		maxIdx := d.model.visualEnd
		if minIdx > maxIdx {
			minIdx, maxIdx = maxIdx, minIdx
		}
		isVisuallySelected = index >= minIdx && index <= maxIdx
	}

	// Apply highlighting
	isSelected := index == m.Index()
	isFocused := d.model == nil || d.model.focus == listPane

	if isVisuallySelected {
		// Visual selection highlighting - use theme style
		str = theme.DefaultTheme.VisualSelection.Render(str)
	} else if isSelected && isFocused {
		// Normal cursor highlighting (only when list pane is focused)
		str = theme.DefaultTheme.Selected.Render(str)
	} else if isSelected && !isFocused {
		// Dimmed cursor when viewport is focused
		str = theme.DefaultTheme.SelectedUnfocused.Render(str)
	}

	fmt.Fprint(w, str)
}

// keyMap defines all key bindings for the TUI
type logKeyMap struct {
	keymap.Base
	PageUp          key.Binding
	PageDown        key.Binding
	HalfUp          key.Binding
	HalfDown        key.Binding
	GotoTop         key.Binding
	GotoEnd         key.Binding
	Expand          key.Binding
	Search          key.Binding
	Clear           key.Binding
	ToggleFollow    key.Binding
	ToggleFilters   key.Binding
	ViewJSON        key.Binding
	VisualModeStart key.Binding
	Yank            key.Binding
	SwitchFocus     key.Binding
}

var logKeys = logKeyMap{
	Base: keymap.NewBase(),
	PageUp: key.NewBinding(
		key.WithKeys("pgup"),
		key.WithHelp("pgup", "page up"),
	),
	PageDown: key.NewBinding(
		key.WithKeys("pgdown"),
		key.WithHelp("pgdn", "page down"),
	),
	HalfUp: key.NewBinding(
		key.WithKeys("ctrl+u"),
		key.WithHelp("ctrl+u", "half page up"),
	),
	HalfDown: key.NewBinding(
		key.WithKeys("ctrl+d"),
		key.WithHelp("ctrl+d", "half page down"),
	),
	GotoTop: key.NewBinding(
		key.WithKeys("g"),
		key.WithHelp("gg", "go to top"),
	),
	GotoEnd: key.NewBinding(
		key.WithKeys("G"),
		key.WithHelp("G", "go to end"),
	),
	Expand: key.NewBinding(
		key.WithKeys(" "),
		key.WithHelp("space", "expand/collapse"),
	),
	Search: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "search"),
	),
	Clear: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "clear search"),
	),
	ToggleFollow: key.NewBinding(
		key.WithKeys("F"),
		key.WithHelp("F", "toggle follow"),
	),
	ToggleFilters: key.NewBinding(
		key.WithKeys("f"),
		key.WithHelp("f", "toggle filters"),
	),
	ViewJSON: key.NewBinding(
		key.WithKeys("J"),
		key.WithHelp("J", "view json"),
	),
	VisualModeStart: key.NewBinding(
		key.WithKeys("V"),
		key.WithHelp("V", "visual line mode"),
	),
	Yank: key.NewBinding(
		key.WithKeys("y"),
		key.WithHelp("y", "yank selection"),
	),
	SwitchFocus: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "switch focus"),
	),
}

// ShortHelp returns keybindings to be shown in the mini help view.
func (k logKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Base.Help, k.Base.Quit, k.ToggleFollow, k.ToggleFilters, k.Search}
}

// FullHelp returns keybindings for the expanded help view.
func (k logKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{ // Navigation column
			k.Base.Up,
			k.Base.Down,
			k.PageUp,
			k.PageDown,
			k.HalfUp,
			k.HalfDown,
			k.GotoTop,
			k.GotoEnd,
		},
		{ // Actions column
			k.SwitchFocus,
			k.ToggleFollow,
			k.ToggleFilters,
			k.Search,
			k.ViewJSON,
			k.VisualModeStart,
			k.Yank,
			k.Base.Help,
			k.Base.Quit,
		},
	}
}

// Main TUI model
type logModel struct {
	list            list.Model
	items           []logItem
	keys            logKeyMap
	spinner         spinner.Model
	viewport        viewport.Model
	help            help.Model
	loading         bool
	err             error
	width           int
	height          int
	followMode      bool
	filtersEnabled  bool // Whether component filters are active
	filteredCount   int  // Count of logs hidden by filters
	logChan         chan TailedLine
	mu              sync.Mutex
	lastGotoG       time.Time
	workspaceColors map[string]lipgloss.Style
	colorIndex      int
	ready           bool      // viewport ready flag
	focus           paneFocus // which pane has focus
	visualMode      bool      // visual selection mode
	visualStart     int       // start of visual selection
	visualEnd       int       // end of visual selection
	statusMessage   string    // status message for copy confirmation
	jsonTree        jsontree.Model
	jsonView        bool
	logConfig       *logging.Config // logging config for component filtering
	overrideOpts    *logging.OverrideOptions
}

// Messages
type newLogMsg struct {
	workspace string
	data      map[string]interface{}
}

type tickMsg time.Time

func (m *logModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.waitForLogs(),
		tick(),
	)
}

func tick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m *logModel) getSelectedContent() string {
	minIdx := m.visualStart
	maxIdx := m.visualEnd
	if minIdx > maxIdx {
		minIdx, maxIdx = maxIdx, minIdx
	}

	// Get the visible items from the list (handles filtering)
	visibleItems := m.list.VisibleItems()

	// Create a JSON array of selected log entries
	var logs []map[string]interface{}
	for i := minIdx; i <= maxIdx && i < len(visibleItems); i++ {
		// Get the actual item from visible items
		if item, ok := visibleItems[i].(logItem); ok {
			// Create a copy of the raw data
			logEntry := make(map[string]interface{})
			for k, v := range item.rawData {
				logEntry[k] = v
			}

			// Ensure workspace is included (might not be in rawData)
			logEntry["workspace"] = item.workspace

			logs = append(logs, logEntry)
		}
	}

	// Convert to pretty JSON
	jsonBytes, err := json.MarshalIndent(logs, "", "  ")
	if err != nil {
		// Fallback to simple format if JSON fails
		return fmt.Sprintf("Error formatting JSON: %v", err)
	}

	return string(jsonBytes)
}

func (m *logModel) copyToClipboard(content string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		// Try xclip first, then xsel
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else {
			return fmt.Errorf("no clipboard utility found (install xclip or xsel)")
		}
	case "windows":
		cmd = exec.Command("cmd", "/c", "clip")
	default:
		return fmt.Errorf("unsupported platform")
	}

	cmd.Stdin = strings.NewReader(content)
	return cmd.Run()
}

func (m *logModel) clearStatusMessageAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return clearStatusMsg{}
	})
}

type clearStatusMsg struct{}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func (m *logModel) waitForLogs() tea.Cmd {
	return func() tea.Msg {
		line, ok := <-m.logChan
		if !ok {
			return nil
		}

		// Parse the JSON log entry
		var rawEntry map[string]interface{}
		if err := json.Unmarshal([]byte(line.Line), &rawEntry); err != nil {
			// Skip non-JSON lines
			return m.waitForLogs()
		}

		return newLogMsg{
			workspace: line.Workspace,
			data:      rawEntry,
		}
	}
}

func (m *logModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Handle jsontree.BackMsg to exit JSON view
	if _, ok := msg.(jsontree.BackMsg); ok {
		m.jsonView = false
		return m, nil
	}

	// If help is showing, handle ESC to close it
	if m.help.ShowAll {
		if msg, ok := msg.(tea.KeyMsg); ok {
			if key.Matches(msg, logKeys.Base.Quit) {
				return m, tea.Quit
			}
			if key.Matches(msg, logKeys.Clear) || msg.String() == "esc" {
				m.help.Toggle()
				return m, nil
			}
		}
		return m, nil
	}

	// If in JSON view, delegate updates to the JSON tree component
	if m.jsonView {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			// Allow quitting even in JSON view
			if key.Matches(msg, logKeys.Base.Quit) {
				return m, tea.Quit
			}
			// Allow help in JSON view
			if key.Matches(msg, logKeys.Base.Help) {
				m.help.Toggle()
				return m, nil
			}
			// Tab switches focus (full-screen toggle) in JSON view too
			if key.Matches(msg, logKeys.SwitchFocus) {
				if m.focus == listPane {
					m.focus = viewportPane
					// Expand JSON tree to full height
					m.jsonTree.SetSize(m.width-4, m.height-3)
				} else {
					m.focus = listPane
					// Restore JSON tree to split height
					listHeight := m.height / 2
					viewportHeight := m.height - listHeight - 3
					m.jsonTree.SetSize(m.width-4, viewportHeight)
				}
				return m, nil
			}
			// Pass ESC and other keys to JSON tree - it will send BackMsg when ready to exit
		case tea.WindowSizeMsg:
			// Handle window size changes
			if m.focus == viewportPane {
				m.jsonTree.SetSize(msg.Width-4, m.height-3)
			} else {
				listHeight := m.height / 2
				viewportHeight := m.height - listHeight - 3
				m.jsonTree.SetSize(msg.Width-4, viewportHeight)
			}
		}
		newTree, cmd := m.jsonTree.Update(msg)
		m.jsonTree = newTree.(jsontree.Model)
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Don't intercept keys when filtering is active except for our special ones
		if m.list.FilterState() == list.Filtering {
			switch {
			case key.Matches(msg, logKeys.Base.Quit):
				// Allow quitting even during search
				return m, tea.Quit
			case key.Matches(msg, logKeys.Clear):
				// Escape clears the filter
				m.list.ResetFilter()
				return m, nil
			}
			// Let the list handle other keys when filtering
		} else {
			// Handle double 'g' for goto top (only when not filtering)
			if msg.String() == "g" {
				if time.Since(m.lastGotoG) < 500*time.Millisecond {
					// Double 'g' pressed - go to top
					m.list.Select(0)
					m.lastGotoG = time.Time{}
					return m, nil
				}
				m.lastGotoG = time.Now()
				return m, nil
			}

			switch {
			case key.Matches(msg, logKeys.Base.Quit):
				return m, tea.Quit

			case key.Matches(msg, logKeys.Base.Help):
				m.help.Toggle()
				return m, nil

			case key.Matches(msg, logKeys.SwitchFocus) || msg.String() == "enter":
				if m.focus == listPane {
					m.focus = viewportPane
					// Expand viewport to full height (minus status line)
					m.viewport.Height = m.height - 3
				} else {
					m.focus = listPane
					// Restore viewport to split height
					listHeight := m.height / 2
					m.viewport.Height = m.height - listHeight - 3
				}
				return m, nil
			}

			// Delegate input based on focus
			if m.focus == viewportPane {
				// Allow h/l to navigate to prev/next log entry when viewport is focused
				if key.Matches(msg, logKeys.Base.Left) {
					currentIndex := m.list.Index()
					if currentIndex > 0 {
						m.list.Select(currentIndex - 1)
						// Update viewport with new selection
						if selectedItem := m.list.SelectedItem(); selectedItem != nil {
							if logItem, ok := selectedItem.(logItem); ok {
								m.viewport.SetContent(logItem.FormatDetails())
								m.viewport.GotoTop()
							}
						}
					}
					return m, nil
				}

				if key.Matches(msg, logKeys.Base.Right) {
					currentIndex := m.list.Index()
					visibleItems := len(m.list.VisibleItems())
					if currentIndex < visibleItems-1 {
						m.list.Select(currentIndex + 1)
						// Update viewport with new selection
						if selectedItem := m.list.SelectedItem(); selectedItem != nil {
							if logItem, ok := selectedItem.(logItem); ok {
								m.viewport.SetContent(logItem.FormatDetails())
								m.viewport.GotoTop()
							}
						}
					}
					return m, nil
				}

				// ESC returns focus to list pane
				if key.Matches(msg, logKeys.Clear) || msg.String() == "esc" {
					m.focus = listPane
					// Restore viewport to split height
					listHeight := m.height / 2
					m.viewport.Height = m.height - listHeight - 3
					return m, nil
				}
				// Allow J to enter JSON view from full-screen viewport
				if key.Matches(msg, logKeys.ViewJSON) {
					if selectedItem := m.list.SelectedItem(); selectedItem != nil {
						if logItem, ok := selectedItem.(logItem); ok {
							// Find the first JSON-like object to display
							var jsonData interface{}
							for _, v := range logItem.rawData {
								switch v.(type) {
								case map[string]interface{}, []interface{}:
									jsonData = v
									break
								}
								if jsonData != nil {
									break
								}
							}
							if jsonData != nil {
								m.jsonTree = jsontree.New(jsonData)
								// Use full-screen size since we're in viewportPane
								m.jsonTree.SetSize(m.width-4, m.height-3)
								m.jsonView = true
							} else {
								m.statusMessage = "No JSON data in this log entry"
							}
						}
					}
					return m, nil
				}
				// Pass other input to the viewport for scrolling
				var cmd tea.Cmd
				m.viewport, cmd = m.viewport.Update(msg)
				return m, cmd
			}

			// List pane is focused - handle list-specific keys
			switch {
			case key.Matches(msg, logKeys.VisualModeStart):
				if !m.visualMode {
					m.visualMode = true
					m.visualStart = m.list.Index()
					m.visualEnd = m.list.Index()
				} else {
					// Exit visual mode
					m.visualMode = false
				}
				// Force list to re-render with new highlighting
				m.list.SetDelegate(itemDelegate{model: m})
				// Force a full refresh by re-setting items
				items := m.list.Items()
				m.list.SetItems(items)
				return m, nil

			case key.Matches(msg, logKeys.Yank):
				if m.visualMode {
					// Copy selected items to clipboard
					content := m.getSelectedContent()
					if err := m.copyToClipboard(content); err == nil {
						lineCount := abs(m.visualEnd-m.visualStart) + 1
						m.statusMessage = fmt.Sprintf("Copied %d log entries as JSON", lineCount)
					} else {
						m.statusMessage = fmt.Sprintf("Copy failed: %v", err)
					}
					m.visualMode = false
					// Force re-render to clear highlighting
					m.list.SetDelegate(itemDelegate{model: m})
					// Clear status message after 2 seconds
					return m, m.clearStatusMessageAfter(2 * time.Second)
				}
				return m, nil

			case key.Matches(msg, logKeys.Clear): // Escape to exit visual mode
				if m.visualMode {
					m.visualMode = false
					m.statusMessage = ""
					// Force re-render to clear highlighting
					m.list.SetDelegate(itemDelegate{model: m})
					return m, nil
				}

			case key.Matches(msg, logKeys.GotoEnd):
				m.list.Select(len(m.items) - 1)
				return m, nil

			case key.Matches(msg, logKeys.HalfUp):
				// Calculate half page
				visibleHeight := m.height - 4 // Account for header/footer
				halfPage := visibleHeight / 2
				currentIndex := m.list.Index()
				newIndex := currentIndex - halfPage
				if newIndex < 0 {
					newIndex = 0
				}
				m.list.Select(newIndex)
				return m, nil

			case key.Matches(msg, logKeys.HalfDown):
				// Calculate half page
				visibleHeight := m.height - 4 // Account for header/footer
				halfPage := visibleHeight / 2
				currentIndex := m.list.Index()
				newIndex := currentIndex + halfPage
				if newIndex >= len(m.items) {
					newIndex = len(m.items) - 1
				}
				m.list.Select(newIndex)
				return m, nil

			case key.Matches(msg, logKeys.Search):
				// Let the list handle the "/" key to start filtering
				// Don't return here, let it fall through to list.Update

			case key.Matches(msg, logKeys.ToggleFollow):
				m.followMode = !m.followMode
				if m.followMode {
					m.statusMessage = "Follow mode enabled"
				} else {
					m.statusMessage = "Follow mode disabled"
				}
				return m, m.clearStatusMessageAfter(2 * time.Second)

			case key.Matches(msg, logKeys.ToggleFilters):
				m.filtersEnabled = !m.filtersEnabled
				if m.filtersEnabled {
					m.statusMessage = "Filters enabled"
				} else {
					m.statusMessage = "Filters disabled (showing all)"
				}
				// Note: This does not re-process old logs, only affects new ones.
				return m, m.clearStatusMessageAfter(2 * time.Second)

			case key.Matches(msg, logKeys.ViewJSON):
				// Enter JSON view mode
				if selectedItem := m.list.SelectedItem(); selectedItem != nil {
					if logItem, ok := selectedItem.(logItem); ok {
						// Find the first JSON-like object to display
						var jsonData interface{}
						for _, v := range logItem.rawData {
							switch v.(type) {
							case map[string]interface{}, []interface{}:
								jsonData = v
								break
							}
							if jsonData != nil {
								break
							}
						}

						if jsonData != nil {
							m.jsonTree = jsontree.New(jsonData)
							listHeight := m.height / 2
							viewportHeight := m.height - listHeight - 3
							m.jsonTree.SetSize(m.width-4, viewportHeight)
							m.jsonView = true
						} else {
							m.statusMessage = "No JSON data in this log entry"
							return m, m.clearStatusMessageAfter(2 * time.Second)
						}
					}
				}
				return m, nil
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Update help model size
		m.help.SetSize(msg.Width, msg.Height)

		// Split the view: 1/2 for list, 1/2 for details
		listHeight := m.height / 2
		viewportHeight := m.height - listHeight - 3 // -3 for borders and status line

		// Update list size
		m.list.SetSize(msg.Width, listHeight)

		// Update viewport size
		// Account for: border (2), padding (4), margins (6) = 12 total horizontal
		viewportWidth := msg.Width - 12
		if !m.ready {
			m.viewport = viewport.New(viewportWidth, viewportHeight)
			m.viewport.YPosition = listHeight + 1
			m.ready = true
		} else {
			m.viewport.Width = viewportWidth
			m.viewport.Height = viewportHeight
		}

		// Update content in viewport if we have a selected item
		if selectedItem := m.list.SelectedItem(); selectedItem != nil {
			if logItem, ok := selectedItem.(logItem); ok {
				m.viewport.SetContent(logItem.FormatDetails())
			}
		}

		return m, nil

	case newLogMsg:
		// Process new log entry
		level, _ := msg.data["level"].(string)
		message, _ := msg.data["msg"].(string)
		component, _ := msg.data["component"].(string)
		timeStr, _ := msg.data["time"].(string)

		// Filter based on component visibility config if filters are enabled
		if m.filtersEnabled {
			if m.logConfig != nil {
				// Pass CLI override options to the visibility check.
				visibilityResult := logging.GetComponentVisibility(component, m.logConfig, m.overrideOpts)
				if !visibilityResult.Visible {
					m.filteredCount++
					// Skip this log entry and continue waiting for more logs
					return m, m.waitForLogs()
				}
			}
		}

		var logTime time.Time
		if parsedTime, err := time.Parse(time.RFC3339, timeStr); err == nil {
			logTime = parsedTime
		}

		newItem := logItem{
			workspace: msg.workspace,
			level:     level,
			message:   message,
			component: component,
			timestamp: logTime,
			rawData:   msg.data,
		}

		m.mu.Lock()
		// Find insertion point to maintain sorted order by timestamp.
		// `sort.Search` finds the first index `i` where m.items[i].timestamp is after newItem.timestamp.
		i := sort.Search(len(m.items), func(j int) bool {
			return m.items[j].timestamp.After(newItem.timestamp)
		})

		// Insert newItem at the correct position 'i'.
		if i == len(m.items) {
			m.items = append(m.items, newItem)
		} else {
			m.items = append(m.items, logItem{}) // Grow slice by one.
			copy(m.items[i+1:], m.items[i:])     // Shift elements to the right.
			m.items[i] = newItem                 // Insert the new item.
		}

		// Update list items
		items := make([]list.Item, len(m.items))
		for i := range m.items {
			items[i] = m.items[i]
		}
		m.list.SetItems(items)

		// Auto-scroll to bottom if in follow mode
		if m.followMode {
			m.list.Select(len(m.items) - 1)
			// Update viewport with the new selection
			if selectedItem := m.list.SelectedItem(); selectedItem != nil {
				if logItem, ok := selectedItem.(logItem); ok {
					m.viewport.SetContent(logItem.FormatDetails())
					m.viewport.GotoTop()
				}
			}
		}
		m.mu.Unlock()

		// Continue waiting for more logs
		return m, m.waitForLogs()

	case tickMsg:
		// Check for any rendering updates needed
		return m, tick()

	case clearStatusMsg:
		m.statusMessage = ""
		return m, nil

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	// Update the list
	prevIndex := m.list.Index()
	newListModel, cmd := m.list.Update(msg)
	m.list = newListModel
	cmds = append(cmds, cmd)

	// Update visual selection end if in visual mode
	if m.visualMode && m.list.Index() != prevIndex {
		m.visualEnd = m.list.Index()
		// Force re-render to update highlighting
		m.list.SetDelegate(itemDelegate{model: m})
	}

	// Update viewport content if selection changed
	if m.list.Index() != prevIndex {
		if selectedItem := m.list.SelectedItem(); selectedItem != nil {
			if logItem, ok := selectedItem.(logItem); ok {
				m.viewport.SetContent(logItem.FormatDetails())
				m.viewport.GotoTop()
			}
		}
	}

	// Allow scrolling in viewport with arrow keys when viewport is focused
	// (we can add viewport focus mode later if needed)

	return m, tea.Batch(cmds...)
}

func (m *logModel) View() string {
	if m.help.ShowAll {
		return m.help.View()
	}

	// If not ready, show loading
	if !m.ready {
		return "Initializing..."
	}

	// Status line components
	statusStyle := theme.DefaultTheme.Muted

	followIndicator := " [Follow:OFF]"
	if m.followMode {
		followIndicator = " [Follow:ON]"
	}

	filtersIndicator := " [Filters:OFF]"
	if m.filtersEnabled {
		filtersIndicator = " [Filters:ON]"
	}

	filteredCountIndicator := ""
	if m.filteredCount > 0 {
		filteredCountIndicator = fmt.Sprintf(" [%d hidden]", m.filteredCount)
	}

	filterIndicator := ""
	searchStyle := theme.DefaultTheme.Warning
	if m.list.FilterState() == list.Filtering {
		filterTerm := m.list.FilterValue()
		if filterTerm == "" {
			filterIndicator = " [SEARCHING: type to filter]"
		} else {
			filterIndicator = fmt.Sprintf(" [SEARCHING: %s]", searchStyle.Render(filterTerm))
		}
	} else if m.list.FilterState() == list.FilterApplied {
		filterTerm := m.list.FilterValue()
		filterIndicator = fmt.Sprintf(" [FILTERED: %s]", searchStyle.Render(filterTerm))
	}

	// Show current position in status
	visibleItems := len(m.list.VisibleItems())
	currentIndex := m.list.Index()

	// Ensure index is valid
	if currentIndex < 0 {
		currentIndex = 0
	}

	var position string
	if visibleItems == 0 {
		position = "0/0"
	} else {
		// When filtered, show position within filtered results
		position = fmt.Sprintf("%d/%d", currentIndex+1, visibleItems)
		if m.list.FilterState() != list.Unfiltered && visibleItems < len(m.items) {
			// Also show total when filtered
			position = fmt.Sprintf("%d/%d (of %d)", currentIndex+1, visibleItems, len(m.items))
		}
	}

	// Add visual mode or status message
	modeIndicator := ""
	if m.jsonView {
		modeIndicator = " [JSON VIEW - esc to exit]"
	} else if m.focus == viewportPane {
		modeIndicator = " [SCROLLING - tab to return]"
	} else if m.visualMode {
		modeIndicator = " [VISUAL]"
	} else if m.statusMessage != "" {
		modeIndicator = fmt.Sprintf(" [%s]", m.statusMessage)
	}

	status := statusStyle.Render(fmt.Sprintf(" Logs: %s%s%s%s%s%s | ? for help | q to quit",
		position, followIndicator, filtersIndicator, filteredCountIndicator, filterIndicator, modeIndicator))

	// Full-screen details view when viewport is focused
	if m.focus == viewportPane {
		detailsStyle := theme.DefaultTheme.DetailsBox.Copy().
			Padding(0, 2).
			BorderForeground(theme.DefaultTheme.Highlight.GetForeground())

		var detailsContent string
		if m.jsonView {
			detailsContent = m.jsonTree.View()
		} else {
			detailsContent = m.viewport.View()
		}

		detailsView := detailsStyle.Render(detailsContent)

		return lipgloss.JoinVertical(
			lipgloss.Left,
			detailsView,
			status,
		)
	}

	// Split view when list is focused
	// Main list view with error recovery
	var listView string
	func() {
		defer func() {
			if r := recover(); r != nil {
				// If list view panics, show an error message
				listView = fmt.Sprintf("Error rendering list: %v", r)
			}
		}()
		listView = m.list.View()
	}()

	// Details view with rounded border (serves as visual separation from list)
	detailsStyle := theme.DefaultTheme.DetailsBox.Copy().
		Padding(0, 2).
		MarginLeft(1).
		Width(m.width - 3)

	var detailsContent string
	if m.jsonView {
		detailsContent = m.jsonTree.View()
	} else {
		detailsContent = m.viewport.View()
	}

	detailsView := detailsStyle.Render(detailsContent)

	// Combine all views
	return lipgloss.JoinVertical(
		lipgloss.Left,
		listView,
		detailsView,
		status,
	)
}

// Workspace color management - uses theme's AccentColors palette
var (
	workspaceColorMap   = make(map[string]lipgloss.Style)
	workspaceColorIndex = 0
)

func getWorkspaceStyle(workspace string) lipgloss.Style {
	if style, ok := workspaceColorMap[workspace]; ok {
		return style
	}

	color := theme.DefaultTheme.AccentColors[workspaceColorIndex%len(theme.DefaultTheme.AccentColors)]
	style := lipgloss.NewStyle().Foreground(color).Bold(true)
	workspaceColorMap[workspace] = style
	workspaceColorIndex++

	return style
}

// Run the logs TUI
func runLogsTUI(workspaces []*workspace.WorkspaceNode, follow bool, overrideOpts *logging.OverrideOptions) error {
	logger := logging.NewLogger("logs-tui")

	// Load logging config for component filtering, starting with defaults
	logCfg := logging.GetDefaultLoggingConfig()
	if cfg, err := config.LoadDefault(); err == nil {
		_ = cfg.UnmarshalExtension("logging", &logCfg)
	}

	// Create channel for log lines
	logChan := make(chan TailedLine, 100)

	// Track which files we're already tailing
	tailedFiles := make(map[string]bool)
	var tailedFilesMu sync.Mutex

	// Helper to start tailing a file
	startTailing := func(path, wsName string) {
		tailedFilesMu.Lock()
		if tailedFiles[path] {
			tailedFilesMu.Unlock()
			return
		}
		tailedFiles[path] = true
		tailedFilesMu.Unlock()

		go func() {
			config := tail.Config{
				Follow: true,
				ReOpen: true,
				// Always start from beginning to get all logs
				Location: &tail.SeekInfo{Offset: 0, Whence: io.SeekStart},
				Logger:   stdlog.New(ioutil.Discard, "", 0), // Suppress tail library debug output
			}

			t, err := tail.TailFile(path, config)
			if err != nil {
				logger.Debugf("Cannot tail file %s: %v", path, err)
				return
			}

			for line := range t.Lines {
				if line.Err != nil {
					logger.Debugf("Error reading line from %s: %v", path, line.Err)
					continue
				}
				logChan <- TailedLine{Workspace: wsName, Line: line.Text}
			}
		}()
	}

	// Helper to discover and tail log files for all workspaces
	discoverAndTailFiles := func() {
		for _, ws := range workspaces {
			logDir := filepath.Join(ws.Path, ".grove", "logs")
			files, err := filepath.Glob(filepath.Join(logDir, "*.log"))
			if err != nil {
				continue
			}

			for _, file := range files {
				startTailing(file, ws.Name)
			}
		}
	}

	// Perform initial file discovery BEFORE starting the TUI
	// This ensures existing log files are loaded immediately
	discoverAndTailFiles()

	// Start watching for NEW log files that may be created later
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			<-ticker.C
			discoverAndTailFiles()
		}
	}()

	// Create list
	l := list.New([]list.Item{}, itemDelegate{}, 0, 0)
	l.Title = "Grove Logs"
	l.SetShowStatusBar(false)
	l.SetShowTitle(false)
	l.SetShowHelp(false)
	// Don't disable filtering - we want to toggle it with '/'
	l.SetShowPagination(true)   // Show pagination to help track position
	l.InfiniteScrolling = false // Disable infinite scrolling for better control
	l.DisableQuitKeybindings()  // We handle quit ourselves

	// Configure pagination style
	l.Styles.PaginationStyle = theme.DefaultTheme.Muted.Copy().
		PaddingLeft(2)

	// Create spinner
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = theme.DefaultTheme.Highlight

	// Initialize model
	model := &logModel{
		list:            l,
		items:           []logItem{},
		keys:            logKeys,
		spinner:         s,
		help:            help.New(logKeys),
		loading:         true,
		followMode:      follow,
		filtersEnabled:  false, // Filters are off by default for backward compatibility
		logChan:         logChan,
		workspaceColors: make(map[string]lipgloss.Style),
		ready:           false,
		logConfig:       &logCfg,
		overrideOpts:    overrideOpts,
	}

	// Set the delegate with model reference
	l.SetDelegate(itemDelegate{model: model})

	// Run the TUI
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running TUI: %w", err)
	}

	return nil
}
