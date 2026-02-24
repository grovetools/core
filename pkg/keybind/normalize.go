package keybind

import (
	"regexp"
	"strings"
	"unicode"
)

// Normalize converts key notation from various sources to a standard form.
// Standard form uses:
//   - C- for Control
//   - M- for Meta/Alt
//   - S- for Shift (when explicit)
//   - Uppercase letters (e.g., C-P not C-p)
//
// Examples:
//   - Fish: \ct -> C-T, \ef -> M-F
//   - Bash: \C-t -> C-T, \M-f -> M-F
//   - Zsh: ^T -> C-T, ^[f -> M-F
//   - Tmux: C-t -> C-T (already standard, just uppercase)
func Normalize(key, source string) string {
	if key == "" {
		return ""
	}

	// Handle based on source
	switch source {
	case "fish":
		return normalizeFish(key)
	case "bash":
		return normalizeBash(key)
	case "zsh":
		return normalizeZsh(key)
	case "tmux", "tmux-root", "tmux-prefix", "tmux-table", "grove":
		return normalizeTmux(key)
	default:
		// Default: try to detect and normalize
		return normalizeAuto(key)
	}
}

// normalizeFish converts fish shell key notation to standard form.
// Fish uses: \c for Ctrl, \e for Escape/Meta, \t for Tab, etc.
func normalizeFish(key string) string {
	// Handle escaped sequences
	result := key

	// Check for multi-character sequences like \e\cx (Meta+Ctrl+x)
	if strings.HasPrefix(result, "\\e\\c") {
		// Meta + Ctrl
		rest := result[4:]
		if len(rest) > 0 {
			return "C-M-" + strings.ToUpper(rest[:1]) + rest[1:]
		}
		return result
	}

	// \cx -> C-X
	if strings.HasPrefix(result, "\\c") {
		rest := result[2:]
		if len(rest) > 0 {
			return "C-" + strings.ToUpper(rest[:1]) + rest[1:]
		}
		return result
	}

	// \e followed by something -> Meta
	if strings.HasPrefix(result, "\\e") {
		rest := result[2:]
		if len(rest) > 0 {
			// Check if followed by \c for Ctrl (standardize to C-M-)
			if strings.HasPrefix(rest, "\\c") {
				inner := rest[2:]
				if len(inner) > 0 {
					return "C-M-" + strings.ToUpper(inner[:1]) + inner[1:]
				}
			}
			return "M-" + strings.ToUpper(rest[:1]) + rest[1:]
		}
		return "Escape"
	}

	// Handle special keys
	switch result {
	case "\\t", "\\cI", "\\ci":
		return "Tab"
	case "\\r", "\\cM", "\\cm":
		return "Enter"
	case "\\n", "\\cJ", "\\cj":
		return "C-J"
	case "\\b", "\\cH", "\\ch":
		return "Backspace"
	case "\\x7f":
		return "Delete"
	}

	return normalizeCommon(result)
}

// normalizeBash converts bash/readline key notation to standard form.
// Bash uses: \C- for Ctrl, \M- for Meta, \e for Escape
func normalizeBash(key string) string {
	result := key

	// Handle quoted strings from bind -p output
	result = strings.Trim(result, "\"")

	// \C-\M-x -> C-M-X
	if strings.HasPrefix(result, "\\C-\\M-") {
		rest := result[6:]
		if len(rest) > 0 {
			return "C-M-" + strings.ToUpper(rest[:1]) + rest[1:]
		}
		return result
	}

	// \M-\C-x -> M-C-X (same as C-M-X)
	if strings.HasPrefix(result, "\\M-\\C-") {
		rest := result[6:]
		if len(rest) > 0 {
			return "C-M-" + strings.ToUpper(rest[:1]) + rest[1:]
		}
		return result
	}

	// \C-x -> C-X
	if strings.HasPrefix(result, "\\C-") {
		rest := result[3:]
		if len(rest) > 0 {
			return "C-" + strings.ToUpper(rest[:1]) + rest[1:]
		}
		return result
	}

	// \M-x -> M-X
	if strings.HasPrefix(result, "\\M-") {
		rest := result[3:]
		if len(rest) > 0 {
			return "M-" + strings.ToUpper(rest[:1]) + rest[1:]
		}
		return result
	}

	// \e -> Escape (or Meta prefix)
	if result == "\\e" {
		return "Escape"
	}
	if strings.HasPrefix(result, "\\e") {
		rest := result[2:]
		if len(rest) > 0 {
			return "M-" + strings.ToUpper(rest[:1]) + rest[1:]
		}
		return "Escape"
	}

	return normalizeCommon(result)
}

// normalizeZsh converts zsh key notation to standard form.
// Zsh uses: ^ for Ctrl, ^[ for Escape/Meta
func normalizeZsh(key string) string {
	result := key

	// Handle bindkey output format: may have quotes
	result = strings.Trim(result, "\"")

	// ^[^X -> M-C-X
	if strings.HasPrefix(result, "^[^") {
		rest := result[3:]
		if len(rest) > 0 {
			return "M-C-" + strings.ToUpper(rest[:1]) + rest[1:]
		}
		return result
	}

	// ^[x -> M-X
	if strings.HasPrefix(result, "^[") {
		rest := result[2:]
		if len(rest) > 0 {
			return "M-" + strings.ToUpper(rest[:1]) + rest[1:]
		}
		return "Escape"
	}

	// ^X -> C-X
	if strings.HasPrefix(result, "^") && len(result) > 1 {
		rest := result[1:]
		if len(rest) > 0 {
			return "C-" + strings.ToUpper(rest[:1]) + rest[1:]
		}
		return result
	}

	return normalizeCommon(result)
}

// normalizeTmux normalizes tmux key notation.
// Tmux already uses C-, M-, S- but may have lowercase.
func normalizeTmux(key string) string {
	result := key

	// Handle combined modifiers
	// C-M-x, M-C-x -> C-M-X
	cmPattern := regexp.MustCompile(`^[CM]-[CM]-(.+)$`)
	if matches := cmPattern.FindStringSubmatch(result); len(matches) > 1 {
		return "C-M-" + strings.ToUpper(matches[1][:1]) + matches[1][1:]
	}

	// C-x -> C-X
	if strings.HasPrefix(result, "C-") && len(result) > 2 {
		rest := result[2:]
		return "C-" + strings.ToUpper(rest[:1]) + rest[1:]
	}

	// M-x -> M-X
	if strings.HasPrefix(result, "M-") && len(result) > 2 {
		rest := result[2:]
		return "M-" + strings.ToUpper(rest[:1]) + rest[1:]
	}

	// S-x -> S-X
	if strings.HasPrefix(result, "S-") && len(result) > 2 {
		rest := result[2:]
		return "S-" + strings.ToUpper(rest[:1]) + rest[1:]
	}

	return normalizeCommon(result)
}

// normalizeAuto tries to detect the format and normalize accordingly.
func normalizeAuto(key string) string {
	// Fish-style \cx
	if strings.HasPrefix(key, "\\c") || strings.HasPrefix(key, "\\e") {
		return normalizeFish(key)
	}

	// Bash-style \C- or \M-
	if strings.HasPrefix(key, "\\C-") || strings.HasPrefix(key, "\\M-") {
		return normalizeBash(key)
	}

	// Zsh-style ^
	if strings.HasPrefix(key, "^") {
		return normalizeZsh(key)
	}

	// Tmux-style C- or M-
	if strings.HasPrefix(key, "C-") || strings.HasPrefix(key, "M-") || strings.HasPrefix(key, "S-") {
		return normalizeTmux(key)
	}

	return normalizeCommon(key)
}

// normalizeCommon handles common key names across all formats.
func normalizeCommon(key string) string {
	lower := strings.ToLower(key)

	// Standard special key names
	switch lower {
	case "enter", "return", "cr":
		return "Enter"
	case "escape", "esc":
		return "Escape"
	case "tab":
		return "Tab"
	case "space", "spc":
		return "Space"
	case "backspace", "bs":
		return "Backspace"
	case "delete", "del":
		return "Delete"
	case "up":
		return "Up"
	case "down":
		return "Down"
	case "left":
		return "Left"
	case "right":
		return "Right"
	case "home":
		return "Home"
	case "end":
		return "End"
	case "pageup", "pgup":
		return "PageUp"
	case "pagedown", "pgdn":
		return "PageDown"
	case "insert", "ins":
		return "Insert"
	}

	// Function keys F1-F12
	if len(lower) >= 2 && lower[0] == 'f' && unicode.IsDigit(rune(lower[1])) {
		return "F" + key[1:]
	}

	// Single character: return uppercase
	if len(key) == 1 {
		return strings.ToUpper(key)
	}

	return key
}

// Denormalize converts standard key notation to a target format.
// Currently returns the key unchanged; full implementation in Phase 2.
func Denormalize(key, target string) string {
	// TODO: Implement for shell config generation (Phase 2)
	return key
}

// ParseKeySequence splits a key sequence string into individual keys.
// For example: "C-g p" -> ["C-G", "P"]
func ParseKeySequence(seq string) []string {
	parts := strings.Fields(seq)
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		normalized := normalizeAuto(p)
		if normalized != "" {
			result = append(result, normalized)
		}
	}
	return result
}

// KeysEqual checks if two key notations represent the same key.
func KeysEqual(a, b string) bool {
	return Normalize(a, "") == Normalize(b, "")
}
