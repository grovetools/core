package jsontree

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattsolo1/grove-core/tui/theme"
)

// node represents an element in the JSON tree.
type node struct {
	key       string
	value     interface{}
	valueType string // "object", "array", "string", "number", "boolean", "null"
	depth     int
	children  []*node
	collapsed bool
	isLast    bool // Is this the last child of its parent?
}

// Model is the Bubble Tea model for the JSON tree viewer.
type Model struct {
	viewport        viewport.Model
	root            *node
	nodes           []*node // A flattened list of visible nodes for rendering
	cursor          int
	keys            KeyMap
	width           int
	height          int
	ready           bool
	lastZPress      time.Time // For detecting zR/zM sequences
	lastGPress      time.Time // For detecting gg sequence
	renderedContent string    // Cached rendered content for direct display

	// Search state
	isSearching   bool
	searchInput   textinput.Model
	searchQuery   string // The active search query (after Enter)
	searchResults []int  // Indices of nodes matching the search
	currentResult int    // Index into searchResults (-1 if no results)

	// Status message for yank confirmations
	statusMessage string

	// Original data for YankAll
	originalData interface{}

	// Visual mode state
	visualMode  bool
	visualStart int
	visualEnd   int
}

// BackMsg is sent when the user wants to exit the JSON viewer
type BackMsg struct{}

// New creates a new JSON tree model.
func New(data interface{}) Model {
	// Initialize search input
	ti := textinput.New()
	ti.Placeholder = "Search..."
	ti.Prompt = "/"
	ti.CharLimit = 100
	ti.Width = 30

	m := Model{
		keys:          DefaultKeyMap(),
		cursor:        0,
		searchInput:   ti,
		currentResult: -1,
		originalData:  data,
	}

	if data != nil {
		m.root = buildTree("root", data, 0)
		m.nodes = flattenTree(m.root)
	}

	return m
}

// SetSize sets the size of the component.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
	if m.ready {
		m.viewport.Width = width
		m.viewport.Height = height
	} else {
		m.viewport = viewport.New(width, height)
		m.ready = true
	}
	m.updateContent()
}

// buildTree recursively builds a tree of nodes from JSON data.
func buildTree(key string, value interface{}, depth int) *node {
	n := &node{
		key:   key,
		value: value,
		depth: depth,
	}

	switch v := value.(type) {
	case map[string]interface{}:
		n.valueType = "object"
		n.collapsed = depth > 0 // Start collapsed except root

		// Sort keys for consistent ordering
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for i, k := range keys {
			child := buildTree(k, v[k], depth+1)
			child.isLast = i == len(keys)-1
			n.children = append(n.children, child)
		}

	case []interface{}:
		n.valueType = "array"
		n.collapsed = depth > 0 // Start collapsed except root

		for i, item := range v {
			child := buildTree(fmt.Sprintf("[%d]", i), item, depth+1)
			child.isLast = i == len(v)-1
			n.children = append(n.children, child)
		}

	case string:
		n.valueType = "string"
	case float64:
		n.valueType = "number"
	case bool:
		n.valueType = "boolean"
	case nil:
		n.valueType = "null"
	default:
		n.valueType = "unknown"
	}

	return n
}

// closingBracket is a pseudo-node for rendering closing brackets
type closingBracket struct {
	depth     int
	bracket   string // "}" or "]"
}

// flattenTree creates a flattened list of visible nodes for rendering.
func flattenTree(root *node) []*node {
	if root == nil {
		return nil
	}

	var nodes []*node
	var flatten func(n *node)
	flatten = func(n *node) {
		nodes = append(nodes, n)
		if !n.collapsed && len(n.children) > 0 {
			for _, child := range n.children {
				flatten(child)
			}
			// Add closing bracket node after children
			if n.valueType == "object" || n.valueType == "array" {
				closingNode := &node{
					key:       "", // empty key indicates closing bracket
					depth:     n.depth,
					valueType: "closing_" + n.valueType,
				}
				nodes = append(nodes, closingNode)
			}
		}
	}

	// For root wrapper, show opening bracket, children, then closing bracket
	if root.key == "root" && (root.valueType == "object" || root.valueType == "array") {
		// Add opening bracket
		openingNode := &node{
			key:       "",
			depth:     0,
			valueType: "opening_" + root.valueType,
		}
		nodes = append(nodes, openingNode)

		// Add children
		for _, child := range root.children {
			flatten(child)
		}

		// Add closing bracket
		closingNode := &node{
			key:       "",
			depth:     0,
			valueType: "closing_" + root.valueType,
		}
		nodes = append(nodes, closingNode)
	} else {
		flatten(root)
	}

	return nodes
}

// Init initializes the component.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages and user input.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Handle search input mode
	if m.isSearching {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.Type {
			case tea.KeyEnter:
				// Perform search and exit search input mode
				m.searchQuery = m.searchInput.Value()
				m.performSearch()
				m.isSearching = false
				m.updateContent()
				return m, nil
			case tea.KeyEsc:
				// Cancel search
				m.isSearching = false
				m.searchInput.SetValue("")
				return m, nil
			}
		}
		// Update text input
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		keyStr := msg.String()

		// Handle zR and zM sequences
		if keyStr == "z" {
			m.lastZPress = time.Now()
			return m, nil
		}

		// Check for zR/zM within time window
		if time.Since(m.lastZPress) < 500*time.Millisecond {
			switch keyStr {
			case "R", "shift+r":
				// zR - expand all
				m.expandAll()
				m.lastZPress = time.Time{}
				return m, nil
			case "M", "shift+m":
				// zM - collapse all
				m.collapseAll()
				m.lastZPress = time.Time{}
				return m, nil
			}
		}

		// Handle gg sequence (go to top)
		if keyStr == "g" {
			if time.Since(m.lastGPress) < 500*time.Millisecond {
				// Double 'g' pressed - go to top
				m.cursor = 0
				m.updateContent()
				m.lastGPress = time.Time{}
				return m, nil
			}
			m.lastGPress = time.Now()
			return m, nil
		}

		switch {
		case key.Matches(msg, m.keys.Search):
			// Enter search mode
			m.isSearching = true
			m.searchInput.Focus()
			return m, textinput.Blink

		case key.Matches(msg, m.keys.NextResult):
			// Jump to next search result
			if len(m.searchResults) > 0 {
				m.currentResult = (m.currentResult + 1) % len(m.searchResults)
				m.cursor = m.searchResults[m.currentResult]
				m.updateContent()
			}
			return m, nil

		case key.Matches(msg, m.keys.PrevResult):
			// Jump to previous search result
			if len(m.searchResults) > 0 {
				m.currentResult--
				if m.currentResult < 0 {
					m.currentResult = len(m.searchResults) - 1
				}
				m.cursor = m.searchResults[m.currentResult]
				m.updateContent()
			}
			return m, nil

		case key.Matches(msg, m.keys.VisualMode):
			// Toggle visual mode
			if !m.visualMode {
				m.visualMode = true
				m.visualStart = m.cursor
				m.visualEnd = m.cursor
				m.statusMessage = "-- VISUAL --"
			} else {
				m.visualMode = false
				m.statusMessage = ""
			}
			m.updateContent()
			return m, nil

		case key.Matches(msg, m.keys.YankValue):
			// Copy visual selection or current node's value to clipboard
			if m.visualMode {
				// Yank visual selection
				content := m.getVisualSelectionString()
				if err := m.copyToClipboard(content); err != nil {
					m.statusMessage = fmt.Sprintf("Copy failed: %v", err)
				} else {
					minIdx, maxIdx := m.visualStart, m.visualEnd
					if minIdx > maxIdx {
						minIdx, maxIdx = maxIdx, minIdx
					}
					count := maxIdx - minIdx + 1
					m.statusMessage = fmt.Sprintf("Copied %d nodes", count)
				}
				m.visualMode = false
				m.updateContent()
				return m, m.clearStatusAfter()
			}
			// Single node yank
			if m.cursor < len(m.nodes) {
				n := m.nodes[m.cursor]
				content := m.getNodeValueString(n)
				if err := m.copyToClipboard(content); err != nil {
					m.statusMessage = fmt.Sprintf("Copy failed: %v", err)
				} else {
					m.statusMessage = fmt.Sprintf("Copied: %s", truncateString(content, 30))
				}
				m.updateContent()
				return m, m.clearStatusAfter()
			}
			return m, nil

		case key.Matches(msg, m.keys.YankAll):
			// Copy entire JSON to clipboard
			content, err := json.MarshalIndent(m.originalData, "", "  ")
			if err != nil {
				m.statusMessage = fmt.Sprintf("Marshal failed: %v", err)
			} else if err := m.copyToClipboard(string(content)); err != nil {
				m.statusMessage = fmt.Sprintf("Copy failed: %v", err)
			} else {
				m.statusMessage = "Copied entire JSON to clipboard"
			}
			m.updateContent()
			return m, m.clearStatusAfter()

		case key.Matches(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
				if m.visualMode {
					m.visualEnd = m.cursor
				}
				m.updateContent()
			}
			return m, nil

		case key.Matches(msg, m.keys.Down):
			if m.cursor < len(m.nodes)-1 {
				m.cursor++
				if m.visualMode {
					m.visualEnd = m.cursor
				}
				m.updateContent()
			}
			return m, nil

		case key.Matches(msg, m.keys.HalfPageUp):
			// Move cursor up by half page
			halfPage := m.viewport.Height / 2
			m.cursor -= halfPage
			if m.cursor < 0 {
				m.cursor = 0
			}
			m.updateContent()
			return m, nil

		case key.Matches(msg, m.keys.HalfPageDown):
			// Move cursor down by half page
			halfPage := m.viewport.Height / 2
			m.cursor += halfPage
			if m.cursor >= len(m.nodes) {
				m.cursor = len(m.nodes) - 1
			}
			m.updateContent()
			return m, nil

		case key.Matches(msg, m.keys.GotoEnd):
			// G - go to end
			m.cursor = len(m.nodes) - 1
			m.updateContent()
			return m, nil

		case key.Matches(msg, m.keys.Toggle):
			if m.cursor < len(m.nodes) {
				n := m.nodes[m.cursor]
				if len(n.children) > 0 {
					n.collapsed = !n.collapsed
					m.nodes = flattenTree(m.root)
					// Ensure cursor is still valid
					if m.cursor >= len(m.nodes) {
						m.cursor = len(m.nodes) - 1
					}
					// Re-run search to update result indices after tree change
					if m.searchQuery != "" {
						m.performSearch()
					}
					m.updateContent()
				}
			}
			return m, nil

		case key.Matches(msg, m.keys.Fold):
			// h - fold/collapse current node (vim-style)
			if m.cursor < len(m.nodes) {
				n := m.nodes[m.cursor]
				if len(n.children) > 0 && !n.collapsed {
					n.collapsed = true
					m.nodes = flattenTree(m.root)
					// Re-run search to update result indices after tree change
					if m.searchQuery != "" {
						m.performSearch()
					}
					m.updateContent()
				}
			}
			return m, nil

		case key.Matches(msg, m.keys.Back):
			// If in visual mode, exit visual mode first
			if m.visualMode {
				m.visualMode = false
				m.statusMessage = ""
				m.updateContent()
				return m, nil
			}
			// Clear search when exiting
			m.searchQuery = ""
			m.searchResults = nil
			m.currentResult = -1
			m.searchInput.SetValue("")
			return m, func() tea.Msg { return BackMsg{} }
		}

	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
		return m, nil

	case clearStatusMsg:
		m.statusMessage = ""
		return m, nil
	}

	return m, nil
}

// expandAll expands all nodes in the tree.
func (m *Model) expandAll() {
	var expand func(n *node)
	expand = func(n *node) {
		n.collapsed = false
		for _, child := range n.children {
			expand(child)
		}
	}
	if m.root != nil {
		expand(m.root)
		m.nodes = flattenTree(m.root)
		m.updateContent()
	}
}

// collapseAll collapses all nodes in the tree.
func (m *Model) collapseAll() {
	var collapse func(n *node)
	collapse = func(n *node) {
		if n.depth > 0 {
			n.collapsed = true
		}
		for _, child := range n.children {
			collapse(child)
		}
	}
	if m.root != nil {
		collapse(m.root)
		m.nodes = flattenTree(m.root)
		// Reset cursor to start
		m.cursor = 0
		// Re-run search to update result indices after tree change
		if m.searchQuery != "" {
			m.performSearch()
		}
		m.updateContent()
	}
}

// performSearch finds all nodes matching the current search query.
func (m *Model) performSearch() {
	query := strings.ToLower(m.searchQuery)
	if query == "" {
		m.searchResults = nil
		m.currentResult = -1
		return
	}

	m.searchResults = nil
	for i, n := range m.nodes {
		// Skip bracket nodes
		if strings.HasPrefix(n.valueType, "opening_") || strings.HasPrefix(n.valueType, "closing_") {
			continue
		}

		// Check if key matches
		if strings.Contains(strings.ToLower(n.key), query) {
			m.searchResults = append(m.searchResults, i)
			continue
		}

		// Check if value matches (for leaf nodes)
		if n.value != nil {
			valueStr := fmt.Sprintf("%v", n.value)
			if strings.Contains(strings.ToLower(valueStr), query) {
				m.searchResults = append(m.searchResults, i)
			}
		}
	}

	// Jump to first result if we have any
	if len(m.searchResults) > 0 {
		m.currentResult = 0
		m.cursor = m.searchResults[0]
	} else {
		m.currentResult = -1
	}
}

// isSearchResult checks if a node index is a search result.
func (m *Model) isSearchResult(idx int) bool {
	for _, r := range m.searchResults {
		if r == idx {
			return true
		}
	}
	return false
}

// copyToClipboard writes the given string to the system clipboard.
func (m *Model) copyToClipboard(content string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
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

// getNodeValueString returns a string representation of a node's value.
func (m *Model) getNodeValueString(n *node) string {
	if n == nil {
		return ""
	}

	switch n.valueType {
	case "object", "array":
		// For collapsed containers, marshal the entire subtree
		if n.value != nil {
			jsonBytes, err := json.MarshalIndent(n.value, "", "  ")
			if err == nil {
				return string(jsonBytes)
			}
		}
		return fmt.Sprintf("%v", n.value)
	case "string":
		return fmt.Sprintf("%v", n.value)
	case "number", "boolean", "null":
		return fmt.Sprintf("%v", n.value)
	case "opening_object", "closing_object":
		return "{}"
	case "opening_array", "closing_array":
		return "[]"
	default:
		return fmt.Sprintf("%v", n.value)
	}
}

// getVisualSelectionString returns the string representation of the visual selection as valid JSON.
func (m *Model) getVisualSelectionString() string {
	minIdx, maxIdx := m.visualStart, m.visualEnd
	if minIdx > maxIdx {
		minIdx, maxIdx = maxIdx, minIdx
	}

	// Collect selected nodes into a map/slice for JSON serialization
	result := make(map[string]interface{})
	for i := minIdx; i <= maxIdx && i < len(m.nodes); i++ {
		n := m.nodes[i]
		// Skip bracket-only nodes
		if strings.HasPrefix(n.valueType, "opening_") || strings.HasPrefix(n.valueType, "closing_") {
			continue
		}
		// Use the node's value directly for proper JSON types
		if n.value != nil {
			result[n.key] = n.value
		}
	}

	// Marshal to pretty JSON
	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		// Fallback to simple format
		var lines []string
		for i := minIdx; i <= maxIdx && i < len(m.nodes); i++ {
			n := m.nodes[i]
			if strings.HasPrefix(n.valueType, "opening_") || strings.HasPrefix(n.valueType, "closing_") {
				continue
			}
			lines = append(lines, fmt.Sprintf("%s: %v", n.key, m.getNodeValueString(n)))
		}
		return strings.Join(lines, "\n")
	}
	return string(jsonBytes)
}

// isVisuallySelected checks if a node index is in the visual selection range.
func (m *Model) isVisuallySelected(idx int) bool {
	if !m.visualMode {
		return false
	}
	minIdx, maxIdx := m.visualStart, m.visualEnd
	if minIdx > maxIdx {
		minIdx, maxIdx = maxIdx, minIdx
	}
	return idx >= minIdx && idx <= maxIdx
}

// truncateString truncates a string to maxLen and adds ellipsis if needed.
func truncateString(s string, maxLen int) string {
	// Remove newlines for display
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// clearStatusMsg is sent to clear the status message after a delay.
type clearStatusMsg struct{}

// clearStatusAfter returns a command that clears the status after 2 seconds.
func (m *Model) clearStatusAfter() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return clearStatusMsg{}
	})
}

// updateContent renders the tree and updates the viewport.
func (m *Model) updateContent() {
	if !m.ready {
		return
	}

	var lines []string
	for i, n := range m.nodes {
		isResult := m.isSearchResult(i)
		isVisual := m.isVisuallySelected(i)
		line := m.renderNode(n, i == m.cursor, isResult, isVisual)
		lines = append(lines, line)
	}

	// Join lines without extra spacing
	content := strings.Join(lines, "\n")
	m.viewport.SetContent(content)
	m.renderedContent = content // Cache for direct access

	// Auto-scroll to keep cursor visible
	if m.cursor < m.viewport.YOffset {
		m.viewport.SetYOffset(m.cursor)
	} else if m.cursor >= m.viewport.YOffset+m.viewport.Height {
		m.viewport.SetYOffset(m.cursor - m.viewport.Height + 1)
	}
}

// renderNode renders a single node line.
func (m *Model) renderNode(n *node, selected bool, isResult bool, isVisual bool) string {
	// Build indentation
	indent := strings.Repeat("  ", n.depth)
	valueStyle := theme.DefaultTheme.Muted

	// Visual selection style - distinct violet background
	visualStyle := lipgloss.NewStyle().
		Background(theme.DefaultTheme.Colors.Violet).
		Foreground(theme.DefaultTheme.Colors.DarkText).
		Bold(true)

	// Handle opening brackets
	if n.valueType == "opening_object" {
		line := indent + valueStyle.Render("{")
		if isVisual {
			line = visualStyle.Render(indent + "{")
		} else if selected {
			line = theme.DefaultTheme.Selected.Render(line)
		}
		return line
	}
	if n.valueType == "opening_array" {
		line := indent + valueStyle.Render("[")
		if isVisual {
			line = visualStyle.Render(indent + "[")
		} else if selected {
			line = theme.DefaultTheme.Selected.Render(line)
		}
		return line
	}

	// Handle closing brackets
	if n.valueType == "closing_object" {
		line := indent + valueStyle.Render("}")
		if isVisual {
			line = visualStyle.Render(indent + "}")
		} else if selected {
			line = theme.DefaultTheme.Selected.Render(line)
		}
		return line
	}
	if n.valueType == "closing_array" {
		line := indent + valueStyle.Render("]")
		if isVisual {
			line = visualStyle.Render(indent + "]")
		} else if selected {
			line = theme.DefaultTheme.Selected.Render(line)
		}
		return line
	}

	// Build prefix (fold icon or bullet)
	var prefix string
	if len(n.children) > 0 {
		if n.collapsed {
			prefix = theme.IconFolderPlus + " "
		} else {
			prefix = theme.IconFolderOpen + " "
		}
	} else {
		prefix = "  " // Two spaces for leaf alignment
	}

	// Build key display - highlight search matches
	keyStyle := theme.DefaultTheme.Info
	keyDisplay := n.key
	if isResult && m.searchQuery != "" && strings.Contains(strings.ToLower(n.key), strings.ToLower(m.searchQuery)) {
		keyDisplay = m.highlightMatch(n.key, m.searchQuery, keyStyle)
	} else {
		keyDisplay = keyStyle.Render(n.key)
	}

	// Build value display
	var valueDisplay string

	switch n.valueType {
	case "object":
		if n.collapsed {
			valueDisplay = valueStyle.Render(fmt.Sprintf("{...} (%d fields)", len(n.children)))
		} else {
			valueDisplay = valueStyle.Render("{")
		}
	case "array":
		if n.collapsed {
			valueDisplay = valueStyle.Render(fmt.Sprintf("[...] (%d items)", len(n.children)))
		} else {
			valueDisplay = valueStyle.Render("[")
		}
	case "string":
		stringStyle := theme.DefaultTheme.Success
		valStr := fmt.Sprintf("\"%v\"", n.value)
		if isResult && m.searchQuery != "" && strings.Contains(strings.ToLower(valStr), strings.ToLower(m.searchQuery)) {
			valueDisplay = m.highlightMatch(valStr, m.searchQuery, stringStyle)
		} else {
			valueDisplay = stringStyle.Render(valStr)
		}
	case "number":
		numStyle := theme.DefaultTheme.Warning
		var valStr string
		if v, ok := n.value.(float64); ok {
			if v == float64(int64(v)) {
				valStr = fmt.Sprintf("%.0f", v)
			} else {
				valStr = fmt.Sprintf("%v", v)
			}
		} else {
			valStr = fmt.Sprintf("%v", n.value)
		}
		if isResult && m.searchQuery != "" && strings.Contains(strings.ToLower(valStr), strings.ToLower(m.searchQuery)) {
			valueDisplay = m.highlightMatch(valStr, m.searchQuery, numStyle)
		} else {
			valueDisplay = numStyle.Render(valStr)
		}
	case "boolean":
		boolStyle := theme.DefaultTheme.Accent
		valStr := fmt.Sprintf("%v", n.value)
		if isResult && m.searchQuery != "" && strings.Contains(strings.ToLower(valStr), strings.ToLower(m.searchQuery)) {
			valueDisplay = m.highlightMatch(valStr, m.searchQuery, boolStyle)
		} else {
			valueDisplay = boolStyle.Render(valStr)
		}
	case "null":
		nullStyle := theme.DefaultTheme.Error
		valueDisplay = nullStyle.Render("null")
	default:
		valueDisplay = valueStyle.Render(fmt.Sprintf("%v", n.value))
	}

	// Combine parts
	line := fmt.Sprintf("%s%s%s: %s", indent, prefix, keyDisplay, valueDisplay)

	// Apply selection styling - visual mode takes priority
	if isVisual {
		line = visualStyle.Render(line)
	} else if selected {
		line = theme.DefaultTheme.Selected.Render(line)
	}

	return line
}

// highlightMatch highlights the matching substring in the text.
func (m *Model) highlightMatch(text, query string, baseStyle lipgloss.Style) string {
	lowerText := strings.ToLower(text)
	lowerQuery := strings.ToLower(query)
	highlightStyle := lipgloss.NewStyle().Background(lipgloss.Color("226")).Foreground(lipgloss.Color("0"))

	var result strings.Builder
	start := 0
	for {
		idx := strings.Index(lowerText[start:], lowerQuery)
		if idx == -1 {
			result.WriteString(baseStyle.Render(text[start:]))
			break
		}
		actualIdx := start + idx
		result.WriteString(baseStyle.Render(text[start:actualIdx]))
		result.WriteString(highlightStyle.Render(text[actualIdx : actualIdx+len(query)]))
		start = actualIdx + len(query)
	}
	return result.String()
}

// View renders the JSON tree.
func (m Model) View() string {
	if !m.ready {
		return "Initializing JSON viewer..."
	}

	if m.root == nil {
		return theme.DefaultTheme.Muted.Render("No JSON data to display")
	}

	// Build the status/search bar
	var statusBar string
	if m.visualMode {
		// Show visual mode indicator with selection count
		minIdx, maxIdx := m.visualStart, m.visualEnd
		if minIdx > maxIdx {
			minIdx, maxIdx = maxIdx, minIdx
		}
		count := maxIdx - minIdx + 1
		statusBar = theme.DefaultTheme.Warning.Render(fmt.Sprintf("-- VISUAL -- (%d selected, y to yank, Esc to cancel)", count))
	} else if m.statusMessage != "" {
		// Show status message (yank confirmation, etc.)
		statusBar = theme.DefaultTheme.Success.Render(m.statusMessage)
	} else if m.isSearching {
		statusBar = m.searchInput.View()
	} else if m.searchQuery != "" {
		if len(m.searchResults) > 0 {
			statusBar = fmt.Sprintf("/%s [%d/%d] (n/N to navigate, / to search again)",
				m.searchQuery, m.currentResult+1, len(m.searchResults))
		} else {
			statusBar = fmt.Sprintf("/%s (no results)", m.searchQuery)
		}
		statusBar = theme.DefaultTheme.Muted.Render(statusBar)
	}

	// Combine viewport and status bar
	if statusBar != "" {
		return lipgloss.JoinVertical(lipgloss.Left, m.viewport.View(), statusBar)
	}

	// Just return viewport content - the logs_tui handles outer styling
	return m.viewport.View()
}
