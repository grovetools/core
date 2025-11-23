package jsontree

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
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
}

// BackMsg is sent when the user wants to exit the JSON viewer
type BackMsg struct{}

// New creates a new JSON tree model.
func New(data interface{}) Model {
	m := Model{
		keys:   DefaultKeyMap(),
		cursor: 0,
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
		case key.Matches(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
				m.updateContent()
			}
			return m, nil

		case key.Matches(msg, m.keys.Down):
			if m.cursor < len(m.nodes)-1 {
				m.cursor++
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
					m.updateContent()
				}
			}
			return m, nil

		case key.Matches(msg, m.keys.Back):
			return m, func() tea.Msg { return BackMsg{} }
		}

	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
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
		m.updateContent()
	}
}

// updateContent renders the tree and updates the viewport.
func (m *Model) updateContent() {
	if !m.ready {
		return
	}

	var lines []string
	for i, n := range m.nodes {
		line := m.renderNode(n, i == m.cursor)
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
func (m *Model) renderNode(n *node, selected bool) string {
	// Build indentation
	indent := strings.Repeat("  ", n.depth)
	valueStyle := theme.DefaultTheme.Muted

	// Handle opening brackets
	if n.valueType == "opening_object" {
		line := indent + valueStyle.Render("{")
		if selected {
			line = theme.DefaultTheme.Selected.Render(line)
		}
		return line
	}
	if n.valueType == "opening_array" {
		line := indent + valueStyle.Render("[")
		if selected {
			line = theme.DefaultTheme.Selected.Render(line)
		}
		return line
	}

	// Handle closing brackets
	if n.valueType == "closing_object" {
		line := indent + valueStyle.Render("}")
		if selected {
			line = theme.DefaultTheme.Selected.Render(line)
		}
		return line
	}
	if n.valueType == "closing_array" {
		line := indent + valueStyle.Render("]")
		if selected {
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

	// Build key display
	keyStyle := theme.DefaultTheme.Info
	keyDisplay := keyStyle.Render(n.key)

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
		valueDisplay = stringStyle.Render(fmt.Sprintf("\"%v\"", n.value))
	case "number":
		numStyle := theme.DefaultTheme.Warning
		if v, ok := n.value.(float64); ok {
			if v == float64(int64(v)) {
				valueDisplay = numStyle.Render(fmt.Sprintf("%.0f", v))
			} else {
				valueDisplay = numStyle.Render(fmt.Sprintf("%v", v))
			}
		} else {
			valueDisplay = numStyle.Render(fmt.Sprintf("%v", n.value))
		}
	case "boolean":
		boolStyle := theme.DefaultTheme.Accent
		valueDisplay = boolStyle.Render(fmt.Sprintf("%v", n.value))
	case "null":
		nullStyle := theme.DefaultTheme.Error
		valueDisplay = nullStyle.Render("null")
	default:
		valueDisplay = valueStyle.Render(fmt.Sprintf("%v", n.value))
	}

	// Combine parts
	line := fmt.Sprintf("%s%s%s: %s", indent, prefix, keyDisplay, valueDisplay)

	// Apply selection styling
	if selected {
		line = theme.DefaultTheme.Selected.Render(line)
	}

	return line
}

// View renders the JSON tree.
func (m Model) View() string {
	if !m.ready {
		return "Initializing JSON viewer..."
	}

	if m.root == nil {
		return theme.DefaultTheme.Muted.Render("No JSON data to display")
	}

	// Just return viewport content - the logs_tui handles outer styling
	return m.viewport.View()
}
