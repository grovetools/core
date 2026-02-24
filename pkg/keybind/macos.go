package keybind

import (
	"bufio"
	"context"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

// macOSHotkeyNames maps symbolic hotkey IDs to human-readable names.
// These IDs come from com.apple.symbolichotkeys AppleSymbolicHotKeys.
var macOSHotkeyNames = map[int]string{
	7:   "Move focus to menu bar",
	8:   "Move focus to Dock",
	9:   "Move focus to active window",
	10:  "Move focus to toolbar",
	11:  "Move focus to floating window",
	12:  "Move focus next window",
	13:  "Move focus to window drawer",
	14:  "Move focus to status menus",
	15:  "Turn Dock Hiding On/Off",
	17:  "Turn VoiceOver On/Off",
	18:  "Show Accessibility controls",
	19:  "Show Help menu",
	21:  "Toggle Terminal",
	22:  "Terminal Visor",
	23:  "Show Spotlight search",
	25:  "Move focus to the next window",
	26:  "Move focus to the previous window",
	27:  "Move focus to the window toolbar",
	28:  "Move focus to the window drawer",
	29:  "Zoom In",
	30:  "Zoom Out",
	31:  "Reverse/Invert Colors",
	32:  "Mission Control",
	33:  "Application windows",
	34:  "Move left a space",
	35:  "Move right a space",
	36:  "Move focus to Dock",
	37:  "Move focus to active or next window",
	51:  "Move focus to the status menus",
	52:  "Turn Dock hiding on or off",
	57:  "Turn VoiceOver on or off",
	59:  "Turn image smoothing on or off",
	60:  "Increase contrast",
	61:  "Decrease contrast",
	62:  "Move focus to the window toolbar",
	64:  "Show Spotlight search",
	65:  "Show Finder search window",
	70:  "Show Desktop",
	73:  "Move to previous desktop",
	75:  "Switch to desktop 1",
	76:  "Switch to desktop 2",
	77:  "Switch to desktop 3",
	78:  "Switch to desktop 4",
	79:  "Switch to desktop 5",
	80:  "Switch to desktop 6",
	81:  "Move focus to next window",
	82:  "Move focus to Dock",
	83:  "Move focus to active window toolbar",
	84:  "Show Launchpad",
	85:  "Move focus to next window",
	118: "Move window to left side of screen",
	119: "Move window to right side of screen",
	160: "Copy a picture of selected area to clipboard",
	162: "Save picture of screen as file",
	163: "Save picture of selected area as file",
	164: "Screenshot and recording options",
	175: "Screenshot",
	184: "Show/Hide Notification Center",
}

// MacOSCollector collects system-level hotkeys from macOS.
type MacOSCollector struct{}

// NewMacOSCollector creates a new macOS hotkey collector.
// Returns nil if not running on macOS.
func NewMacOSCollector() *MacOSCollector {
	if runtime.GOOS != "darwin" {
		return nil
	}
	return &MacOSCollector{}
}

func (c *MacOSCollector) Name() string {
	return "macos"
}

func (c *MacOSCollector) Layer() Layer {
	return LayerOS
}

func (c *MacOSCollector) Collect(ctx context.Context) ([]Binding, error) {
	if runtime.GOOS != "darwin" {
		return nil, nil
	}

	var bindings []Binding

	// Read symbolic hotkeys from defaults
	cmd := exec.CommandContext(ctx, "defaults", "read", "com.apple.symbolichotkeys", "AppleSymbolicHotKeys")
	output, err := cmd.Output()
	if err != nil {
		// Unable to read hotkeys, return empty
		return nil, nil
	}

	// Parse the plist output
	parsed := parseMacOSPlist(string(output))
	for id, info := range parsed {
		if !info.Enabled {
			continue
		}

		// Get human-readable name
		name := macOSHotkeyNames[id]
		if name == "" {
			name = "Unknown hotkey"
		}

		// Convert keycode and modifiers to standard notation
		key := macOSKeyToStandard(info.KeyCode, info.Modifiers)
		if key == "" {
			continue
		}

		bindings = append(bindings, Binding{
			Key:         key,
			RawKey:      key,
			Layer:       LayerOS,
			Source:      "macos",
			Action:      name,
			Description: name,
			Provenance:  ProvenanceDefault,
		})
	}

	// Add well-known macOS shortcuts that might not appear in symbolichotkeys
	wellKnown := []struct {
		key    string
		action string
	}{
		{"Cmd-Tab", "Application Switcher"},
		{"Cmd-Space", "Spotlight Search"},
		{"Cmd-Shift-3", "Screenshot (full screen)"},
		{"Cmd-Shift-4", "Screenshot (selection)"},
		{"Cmd-Q", "Quit Application"},
		{"Cmd-W", "Close Window"},
		{"Cmd-H", "Hide Application"},
		{"Cmd-M", "Minimize Window"},
		{"Cmd-,", "Preferences"},
		{"Cmd-`", "Next Window (same app)"},
		{"Ctrl-Up", "Mission Control"},
		{"Ctrl-Down", "Application Windows"},
		{"Ctrl-Left", "Move Left Space"},
		{"Ctrl-Right", "Move Right Space"},
	}

	for _, wk := range wellKnown {
		// Check if already added
		found := false
		for _, b := range bindings {
			if b.Key == wk.key {
				found = true
				break
			}
		}
		if !found {
			bindings = append(bindings, Binding{
				Key:         wk.key,
				RawKey:      wk.key,
				Layer:       LayerOS,
				Source:      "macos",
				Action:      wk.action,
				Description: wk.action,
				Provenance:  ProvenanceDefault,
			})
		}
	}

	return bindings, nil
}

// macOSHotkeyInfo holds parsed hotkey information.
type macOSHotkeyInfo struct {
	Enabled   bool
	KeyCode   int
	Modifiers int
}

// parseMacOSPlist parses the output of defaults read for symbolic hotkeys.
// This is a simplified parser for the specific plist format.
func parseMacOSPlist(output string) map[int]macOSHotkeyInfo {
	result := make(map[int]macOSHotkeyInfo)

	scanner := bufio.NewScanner(strings.NewReader(output))
	var currentID int
	var inEntry bool
	var entryContent strings.Builder

	for scanner.Scan() {
		line := scanner.Text()

		// Look for entry start: "64 = {"
		if matches := regexp.MustCompile(`^\s*(\d+)\s*=\s*\{`).FindStringSubmatch(line); matches != nil {
			currentID = parseInt(matches[1])
			inEntry = true
			entryContent.Reset()
			entryContent.WriteString(line)
			continue
		}

		if inEntry {
			entryContent.WriteString("\n")
			entryContent.WriteString(line)

			// Check for entry end
			if strings.Contains(line, "};") {
				content := entryContent.String()

				// Parse enabled
				enabled := false
				if enabledMatch := regexp.MustCompile(`enabled\s*=\s*(\d)`).FindStringSubmatch(content); enabledMatch != nil {
					enabled = enabledMatch[1] == "1"
				}

				// Parse parameters
				if paramsMatch := regexp.MustCompile(`parameters\s*=\s*\(\s*(\d+),\s*(\d+),\s*(\d+)\s*\)`).FindStringSubmatch(content); paramsMatch != nil {
					// parameters[0] = character (or 65535 for special keys)
					// parameters[1] = keycode
					// parameters[2] = modifiers
					keyCode := parseInt(paramsMatch[2])
					modifiers := parseInt(paramsMatch[3])

					result[currentID] = macOSHotkeyInfo{
						Enabled:   enabled,
						KeyCode:   keyCode,
						Modifiers: modifiers,
					}
				}

				inEntry = false
			}
		}
	}

	return result
}

// parseInt parses an integer from a string, returning 0 on error.
func parseInt(s string) int {
	var result int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			result = result*10 + int(c-'0')
		}
	}
	return result
}

// macOSKeyToStandard converts macOS keycode and modifiers to standard notation.
func macOSKeyToStandard(keyCode, modifiers int) string {
	// Modifier flags
	const (
		cmdFlag   = 1 << 20 // 1048576
		shiftFlag = 1 << 17 // 131072
		optFlag   = 1 << 19 // 524288
		ctrlFlag  = 1 << 18 // 262144
	)

	// Map common keycodes to key names
	keyNames := map[int]string{
		0:   "A",
		1:   "S",
		2:   "D",
		3:   "F",
		4:   "H",
		5:   "G",
		6:   "Z",
		7:   "X",
		8:   "C",
		9:   "V",
		11:  "B",
		12:  "Q",
		13:  "W",
		14:  "E",
		15:  "R",
		16:  "Y",
		17:  "T",
		18:  "1",
		19:  "2",
		20:  "3",
		21:  "4",
		22:  "6",
		23:  "5",
		24:  "=",
		25:  "9",
		26:  "7",
		27:  "-",
		28:  "8",
		29:  "0",
		30:  "]",
		31:  "O",
		32:  "U",
		33:  "[",
		34:  "I",
		35:  "P",
		36:  "Enter",
		37:  "L",
		38:  "J",
		39:  "'",
		40:  "K",
		41:  ";",
		42:  "\\",
		43:  ",",
		44:  "/",
		45:  "N",
		46:  "M",
		47:  ".",
		48:  "Tab",
		49:  "Space",
		50:  "`",
		51:  "Backspace",
		53:  "Escape",
		96:  "F5",
		97:  "F6",
		98:  "F7",
		99:  "F3",
		100: "F8",
		101: "F9",
		103: "F11",
		105: "F13",
		106: "F16",
		107: "F14",
		109: "F10",
		111: "F12",
		113: "F15",
		118: "F4",
		119: "F2",
		120: "F1",
		122: "F1",
		123: "Left",
		124: "Right",
		125: "Down",
		126: "Up",
	}

	keyName, ok := keyNames[keyCode]
	if !ok {
		return ""
	}

	// Build modifier prefix
	var parts []string

	if modifiers&ctrlFlag != 0 {
		parts = append(parts, "Ctrl")
	}
	if modifiers&optFlag != 0 {
		parts = append(parts, "Alt")
	}
	if modifiers&shiftFlag != 0 {
		parts = append(parts, "Shift")
	}
	if modifiers&cmdFlag != 0 {
		parts = append(parts, "Cmd")
	}

	if len(parts) > 0 {
		return strings.Join(parts, "-") + "-" + keyName
	}
	return keyName
}
