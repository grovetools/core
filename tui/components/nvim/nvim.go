package nvim

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sirupsen/logrus"
)

// redrawMsg carries Neovim's redraw event data.
type redrawMsg [][]interface{}

// waitForRedraw is a tea.Cmd that waits for the next redraw event from Neovim.
func (m Model) waitForRedraw() tea.Cmd {
	return func() tea.Msg {
		return redrawMsg(<-m.redraws)
	}
}

// keyToNvim translates a tea.KeyMsg to a string that Neovim's API can understand.
func keyToNvim(msg tea.KeyMsg) string {
	keyStr := msg.String()
	switch msg.Type {
	case tea.KeySpace:
		return "<Space>"
	case tea.KeyEnter:
		return "<CR>"
	case tea.KeyBackspace:
		return "<BS>"
	case tea.KeyTab:
		return "<Tab>"
	case tea.KeyEsc:
		return "<Esc>"
	case tea.KeyUp:
		return "<Up>"
	case tea.KeyDown:
		return "<Down>"
	case tea.KeyLeft:
		return "<Left>"
	case tea.KeyRight:
		return "<Right>"
	case tea.KeyRunes:
		// Special characters like '<', '>', '\' need to be wrapped.
		if len(keyStr) == 1 {
			switch keyStr {
			case "<":
				return "<LT>"
			case "\\":
				return "<Bslash>"
			}
		}
	}

	// Handle Alt key combinations
	if msg.Alt {
		if len(keyStr) > 4 && keyStr[:4] == "alt+" {
			return fmt.Sprintf("<M-%s>", keyStr[4:])
		}
	}

	// Handle Ctrl key combinations
	// tea.KeyMsg.String() for ctrl+char is "ctrl+char". Nvim expects "<C-char>".
	if len(keyStr) > 5 && keyStr[:5] == "ctrl+" {
		return fmt.Sprintf("<C-%s>", keyStr[5:])
	}

	return keyStr
}

// scrollGrid scrolls a rectangular region of the grid.
// If rows > 0, scroll down (content moves up). If rows < 0, scroll up (content moves down).
// If cols > 0, scroll right (content moves left). If cols < 0, scroll left (content moves right).
func (m *Model) scrollGrid(top, bot, left, right, rows, cols int) {
	if len(m.grid) == 0 {
		return
	}

	debugLogf("scrollGrid: top=%d bot=%d left=%d right=%d rows=%d cols=%d gridH=%d gridW=%d",
		top, bot, left, right, rows, cols, len(m.grid), m.nvimWidth)

	// Clamp boundaries to grid dimensions to prevent panics
	bot = min(bot, len(m.grid))

	// Handle vertical scrolling
	if rows != 0 {
		if rows > 0 { // Scroll down (content moves UP)
			// Copy rows from `top+rows` to `top`.
			for r := top; r < bot-rows; r++ {
				if r >= 0 && r < len(m.grid) && r+rows >= 0 && r+rows < len(m.grid) {
					// Ensure both rows have sufficient length
					srcRow := m.grid[r+rows]
					dstRow := m.grid[r]
					if len(srcRow) >= right && len(dstRow) >= right {
						copy(dstRow[left:right], srcRow[left:right])
					}
				}
			}
			// Clear the newly exposed area at the bottom.
			for r := bot - rows; r < bot; r++ {
				if r >= 0 && r < len(m.grid) {
					for c := left; c < right && c < len(m.grid[r]); c++ {
						m.grid[r][c] = nvimCell{text: " ", hlID: 0}
					}
				}
			}
		} else { // Scroll up (content moves DOWN, rows is negative)
			absRows := -rows
			// Copy rows from `top` to `top+absRows`.
			// We loop backwards to handle the overlapping copy correctly.
			for r := bot - 1; r >= top+absRows; r-- {
				if r >= 0 && r < len(m.grid) && r-absRows >= 0 && r-absRows < len(m.grid) {
					// Ensure both rows have sufficient length
					srcRow := m.grid[r-absRows]
					dstRow := m.grid[r]
					if len(srcRow) >= right && len(dstRow) >= right {
						copy(dstRow[left:right], srcRow[left:right])
					}
				}
			}
			// Clear the newly exposed area at the top.
			for r := top; r < top+absRows; r++ {
				if r >= 0 && r < len(m.grid) {
					for c := left; c < right && c < len(m.grid[r]); c++ {
						m.grid[r][c] = nvimCell{text: " ", hlID: 0}
					}
				}
			}
		}
	}

	// Handle horizontal scrolling (less common, but implemented for completeness)
	if cols != 0 {
		if cols > 0 { // Scroll right (content moves LEFT)
			for r := top; r < bot; r++ {
				if r >= 0 && r < len(m.grid) && len(m.grid[r]) >= right {
					copy(m.grid[r][left:right-cols], m.grid[r][left+cols:right])
					// Clear newly exposed area on the right
					for c := right - cols; c < right && c < len(m.grid[r]); c++ {
						m.grid[r][c] = nvimCell{text: " ", hlID: 0}
					}
				}
			}
		} else { // Scroll left (content moves RIGHT, cols is negative)
			absCols := -cols
			for r := top; r < bot; r++ {
				if r >= 0 && r < len(m.grid) {
					// Loop backwards for correct overlapping copy
					for c := right - 1; c >= left+absCols; c-- {
						if c-absCols >= 0 && c < len(m.grid[r]) && c-absCols < len(m.grid[r]) {
							m.grid[r][c] = m.grid[r][c-absCols]
						}
					}
					// Clear newly exposed area on the left
					for c := left; c < left+absCols && c < len(m.grid[r]); c++ {
						m.grid[r][c] = nvimCell{text: " ", hlID: 0}
					}
				}
			}
		}
	}
}

// min is a helper function to find the minimum of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// handleRedraw parses Neovim's UI events and updates the model's grid.
func (m *Model) handleRedraw(updates [][]interface{}) {
	for _, update := range updates {
		if len(update) == 0 {
			continue
		}
		eventName, ok := update[0].(string)
		if !ok {
			continue
		}
		eventData := update[1:]

		switch eventName {
		case "grid_resize":
			for _, data := range eventData {
				args, ok := data.([]interface{})
				if !ok || len(args) < 3 {
					continue
				}

				// Try int64 first, then uint64
				var width, height int64
				ok1 := false
				ok2 := false

				if w, ok := args[1].(int64); ok {
					width = w
					ok1 = true
				} else if w, ok := args[1].(uint64); ok {
					width = int64(w)
					ok1 = true
				}

				if h, ok := args[2].(int64); ok {
					height = h
					ok2 = true
				} else if h, ok := args[2].(uint64); ok {
					height = int64(h)
					ok2 = true
				}

				if !ok1 || !ok2 {
					continue
				}
				m.nvimWidth = int(width)
				m.nvimHeight = int(height)

				// Resize the grid slice
				m.grid = make([][]nvimCell, m.nvimHeight)
				for i := range m.grid {
					m.grid[i] = make([]nvimCell, m.nvimWidth)
					for j := range m.grid[i] {
						m.grid[i][j] = nvimCell{text: " ", hlID: 0}
					}
				}
			}

		case "default_colors_set":
			for _, data := range eventData {
				args, ok := data.([]interface{})
				if !ok || len(args) < 3 {
					log.Warn("default_colors_set: invalid args format")
					debugLog("default_colors_set: invalid args format")
					continue
				}
				// rgb_fg, rgb_bg, rgb_sp - Neovim sends these as int64 for -1, but uint64 otherwise.
				var fg, bg int64

				if fgU, ok := args[0].(uint64); ok {
					fg = int64(fgU)
				} else if fgI, ok := args[0].(int64); ok {
					fg = fgI
				}

				if bgU, ok := args[1].(uint64); ok {
					bg = int64(bgU)
				} else if bgI, ok := args[1].(int64); ok {
					bg = bgI
				}

				debugLogf("default_colors_set: fg=%d (#%06x) bg=%d (#%06x)", fg, fg, bg, bg)
				log.WithFields(logrus.Fields{
					"fg": fg, "bg": bg,
				}).Info("Received default_colors_set event")

				// Only use default_colors_set if the queried Normal was empty at startup
				if m.needDefaultColors && !m.defaultColorsProcessed {
					style := lipgloss.NewStyle()
					// Use foreground, even if it's white - some highlights need it
					if fg != -1 {
						style = style.Foreground(lipgloss.Color(fmt.Sprintf("#%06x", fg)))
						debugLogf("default_colors_set: Set fg color #%06x", fg)
					}
					// Skip pure black background (#000000) - let terminal background show through
					if bg != -1 && bg != 0 {
						style = style.Background(lipgloss.Color(fmt.Sprintf("#%06x", bg)))
						debugLogf("default_colors_set: Set bg color #%06x", bg)
					} else {
						debugLog("default_colors_set: Skipping black bg (#000000) - using transparent background")
					}
					m.hlMutex.Lock()
					m.hlDefs[0] = style
					m.defaultColorsProcessed = true
					m.hlMutex.Unlock()
					debugLog("Applied default_colors_set to hlDefs[0] (Normal was empty at startup)")
				} else {
					debugLog("Ignoring default_colors_set - using queried Normal highlight instead")
				}
			}

		case "hl_attr_define":
			for _, data := range eventData {
				args, ok := data.([]interface{})
				if !ok || len(args) < 2 {
					continue
				}
				id, ok := args[0].(int64)
				if !ok {
					continue
				}
				attrs, ok := args[1].(map[string]interface{})
				if !ok {
					continue
				}

				// Log what we received
				if len(attrs) > 0 {
					log.WithFields(logrus.Fields{
						"hl_id": id,
						"attrs": attrs,
					}).Debug("hl_attr_define: received attrs")
				}

				// Check if this highlight already exists (Neovim sends multiple events for same ID)
				m.hlMutex.RLock()
				style, exists := m.hlDefs[int(id)]
				m.hlMutex.RUnlock()

				if !exists {
					// Inherit from base style (which has white fg, no bg from default_colors_set)
					m.hlMutex.RLock()
					baseStyle, hasBase := m.hlDefs[0]
					m.hlMutex.RUnlock()

					if hasBase {
						style = baseStyle
					} else {
						style = lipgloss.NewStyle()
					}
				}
				// Otherwise we'll modify the existing style

				hasFg := false
				hasBg := false

				// Apply explicit foreground if specified
				if fg, ok := attrs["foreground"].(uint64); ok {
					fgColor := fmt.Sprintf("#%06x", fg)
					style = style.Foreground(lipgloss.Color(fgColor))
					hasFg = true
					debugLogf("hl_attr_define: id=%d fg=%s", id, fgColor)
				}

				// Apply explicit background if specified
				if bg, ok := attrs["background"].(uint64); ok {
					bgColor := fmt.Sprintf("#%06x", bg)
					style = style.Background(lipgloss.Color(bgColor))
					hasBg = true
					debugLogf("hl_attr_define: id=%d bg=%s", id, bgColor)
				}
				if _, ok := attrs["bold"]; ok {
					style = style.Bold(true)
				}
				if _, ok := attrs["italic"]; ok {
					style = style.Italic(true)
				}
				if _, ok := attrs["reverse"]; ok {
					style = style.Reverse(true)
				}
				m.hlMutex.Lock()
				m.hlDefs[int(id)] = style
				m.hlMutex.Unlock()

				if hasFg || hasBg {
					debugLog(fmt.Sprintf("RGB highlight stored: id=%d fg=%v bg=%v total=%d", id, hasFg, hasBg, len(m.hlDefs)))
					log.WithFields(logrus.Fields{
						"hl_id": id,
						"has_fg": hasFg,
						"has_bg": hasBg,
						"total_defs": len(m.hlDefs),
					}).Info("hl_attr_define: Stored RGB highlight")
				}
			}

		case "grid_scroll":
			debugLogf("grid_scroll event received with %d data items", len(eventData))
			for _, data := range eventData {
				args, ok := data.([]interface{})
				if !ok || len(args) < 7 {
					debugLogf("grid_scroll: invalid args format, len=%d ok=%v", len(args), ok)
					continue
				}
				// Log the actual types we're receiving
				debugLogf("grid_scroll args types: [0]=%T [1]=%T [2]=%T [3]=%T [4]=%T [5]=%T [6]=%T",
					args[0], args[1], args[2], args[3], args[4], args[5], args[6])

				// grid_scroll: [grid, top, bot, left, right, rows, cols]
				// Try to handle both int64 and uint64
				var top, bot, left, right, rows, cols int

				if v, ok := args[1].(int64); ok {
					top = int(v)
				} else if v, ok := args[1].(uint64); ok {
					top = int(v)
				} else {
					debugLogf("grid_scroll: failed to convert top, got type %T value %v", args[1], args[1])
					continue
				}

				if v, ok := args[2].(int64); ok {
					bot = int(v)
				} else if v, ok := args[2].(uint64); ok {
					bot = int(v)
				} else {
					debugLogf("grid_scroll: failed to convert bot, got type %T value %v", args[2], args[2])
					continue
				}

				if v, ok := args[3].(int64); ok {
					left = int(v)
				} else if v, ok := args[3].(uint64); ok {
					left = int(v)
				} else {
					debugLogf("grid_scroll: failed to convert left, got type %T value %v", args[3], args[3])
					continue
				}

				if v, ok := args[4].(int64); ok {
					right = int(v)
				} else if v, ok := args[4].(uint64); ok {
					right = int(v)
				} else {
					debugLogf("grid_scroll: failed to convert right, got type %T value %v", args[4], args[4])
					continue
				}

				if v, ok := args[5].(int64); ok {
					rows = int(v)
				} else if v, ok := args[5].(uint64); ok {
					rows = int(v)
				} else {
					debugLogf("grid_scroll: failed to convert rows, got type %T value %v", args[5], args[5])
					continue
				}

				if v, ok := args[6].(int64); ok {
					cols = int(v)
				} else if v, ok := args[6].(uint64); ok {
					cols = int(v)
				} else {
					debugLogf("grid_scroll: failed to convert cols, got type %T value %v", args[6], args[6])
					continue
				}

				debugLogf("grid_scroll event: calling scrollGrid with top=%d bot=%d left=%d right=%d rows=%d cols=%d",
					top, bot, left, right, rows, cols)
				m.scrollGrid(top, bot, left, right, rows, cols)
			}

		case "grid_line":
			for _, data := range eventData {
				args, ok := data.([]interface{})
				if !ok || len(args) < 4 {
					debugLogf("grid_line: invalid args format")
					continue
				}
				row, ok1 := args[1].(int64)
				colStart, ok2 := args[2].(int64)
				cells, ok3 := args[3].([]interface{})
				if !ok1 || !ok2 || !ok3 {
					continue
				}

				if int(row) >= len(m.grid) {
					continue
				}

				col := int(colStart)
				// Track the current highlight ID - it carries over to subsequent cells if not specified
				currentHlID := 0
				for _, cellData := range cells {
					cell, ok := cellData.([]interface{})
					if !ok || len(cell) == 0 {
						continue
					}
					text, ok := cell[0].(string)
					if !ok {
						continue
					}
					// Empty strings cause rendering issues - ensure at least a space
					if text == "" {
						text = " "
					}
					// Only update hlID if specified; otherwise inherit from previous cell
					if len(cell) > 1 {
						// Handle both int64 and uint64 for highlight ID
						if id, ok := cell[1].(int64); ok {
							currentHlID = int(id)
						} else if id, ok := cell[1].(uint64); ok {
							currentHlID = int(id)
						}
					}
					repeat := 1
					if len(cell) > 2 {
						// Handle both int64 and uint64 for repeat count
						if r, ok := cell[2].(int64); ok {
							repeat = int(r)
						} else if r, ok := cell[2].(uint64); ok {
							repeat = int(r)
						}
					}

					for i := 0; i < repeat; i++ {
						if col < len(m.grid[int(row)]) {
							m.grid[int(row)][col] = nvimCell{text: text, hlID: currentHlID}
							col++
						}
					}
				}
			}

		case "grid_cursor_goto":
			for _, data := range eventData {
				args, ok := data.([]interface{})
				if !ok || len(args) < 3 {
					continue
				}
				row, ok1 := args[1].(int64)
				col, ok2 := args[2].(int64)
				if ok1 && ok2 {
					m.cursorRow = int(row)
					m.cursorCol = int(col)
				}
			}

		case "mode_change":
			for _, data := range eventData {
				args, ok := data.([]interface{})
				if !ok || len(args) == 0 {
					continue
				}
				if mode, ok := args[0].(string); ok {
					m.mode = mode
				}
			}

		case "flush":
			// This signals a redraw is complete. The view will be called by bubbletea automatically.
		}
	}
}

// FormatMode returns a human-readable representation of the current mode.
func (m Model) FormatMode() string {
	return strings.ToUpper(m.mode)
}
