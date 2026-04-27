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
	case "nvim":
		return normalizeNvim(key)
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

// normalizeNvim converts Neovim key notation to standard form.
// Nvim uses: <C-x> for Ctrl, <M-x> for Meta, <CR> for Enter, etc.
func normalizeNvim(key string) string {
	// Handle special keys (nvim format uses angle brackets)
	switch strings.ToLower(key) {
	case "<cr>", "<enter>", "<return>":
		return "Enter"
	case "<tab>":
		return "Tab"
	case "<esc>", "<escape>":
		return "Escape"
	case "<space>":
		return "Space"
	case "<bs>", "<backspace>":
		return "Backspace"
	case "<del>", "<delete>":
		return "Delete"
	case "<up>":
		return "Up"
	case "<down>":
		return "Down"
	case "<left>":
		return "Left"
	case "<right>":
		return "Right"
	case "<home>":
		return "Home"
	case "<end>":
		return "End"
	case "<pageup>":
		return "PageUp"
	case "<pagedown>":
		return "PageDown"
	case "<insert>":
		return "Insert"
	case "<leader>":
		return "Leader"
	}

	// Handle function keys <F1>-<F12>
	if strings.HasPrefix(strings.ToLower(key), "<f") && strings.HasSuffix(key, ">") {
		inner := key[1 : len(key)-1] // Remove < and >
		return strings.ToUpper(inner[:1]) + inner[1:]
	}

	// Handle modifier keys <C-x>, <M-x>, <C-M-x>, etc.
	if strings.HasPrefix(key, "<") && strings.HasSuffix(key, ">") {
		inner := key[1 : len(key)-1] // Remove < and >

		var ctrl, meta bool
		remaining := inner

		// Parse modifiers (case insensitive)
		for {
			upper := strings.ToUpper(remaining)
			if strings.HasPrefix(upper, "C-") {
				ctrl = true
				remaining = remaining[2:]
			} else if strings.HasPrefix(upper, "M-") || strings.HasPrefix(upper, "A-") {
				meta = true
				remaining = remaining[2:]
			} else if strings.HasPrefix(upper, "S-") {
				// Shift modifier - skip for now
				remaining = remaining[2:]
			} else {
				break
			}
		}

		// Build standard notation
		if ctrl && meta {
			return "C-M-" + strings.ToUpper(remaining)
		}
		if ctrl {
			return "C-" + strings.ToUpper(remaining)
		}
		if meta {
			return "M-" + strings.ToUpper(remaining)
		}
		return strings.ToUpper(remaining)
	}

	return normalizeCommon(key)
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
// Standard form uses C-, M-, S- modifiers with uppercase base.
// Supported targets: "fish", "bash", "zsh", "tmux", "nvim".
//
// Examples:
//   - fish: C-G -> \cg, M-F -> \ef, C-M-X -> \e\cx
//   - bash: C-G -> \C-g, M-F -> \M-f, C-M-X -> \C-\M-x
//   - zsh: C-G -> ^g, M-F -> ^[f, C-M-X -> ^[^x
//   - tmux: returns key unchanged (tmux uses standard notation)
//   - nvim: C-G -> <C-g>, M-F -> <M-f>, Enter -> <CR>
func Denormalize(key, target string) string {
	if key == "" {
		return ""
	}

	switch target {
	case "fish":
		return denormalizeFish(key)
	case "bash":
		return denormalizeBash(key)
	case "zsh":
		return denormalizeZsh(key)
	case "tmux":
		return key // Tmux uses standard notation
	case "nvim":
		return denormalizeNvim(key)
	default:
		return key
	}
}

// denormalizeFish converts standard notation to fish shell format.
// C-G -> \cg, M-F -> \ef, C-M-X -> \e\cx
func denormalizeFish(key string) string {
	// Handle special keys first
	switch key {
	case "Enter":
		return "\\r"
	case "Tab":
		return "\\t"
	case "Escape":
		return "\\e"
	case "Backspace":
		return "\\b"
	case "Delete":
		return "\\x7f"
	case "Space":
		return " "
	case "Up":
		return "\\e[A"
	case "Down":
		return "\\e[B"
	case "Right":
		return "\\e[C"
	case "Left":
		return "\\e[D"
	case "Home":
		return "\\e[H"
	case "End":
		return "\\e[F"
	case "PageUp":
		return "\\e[5~"
	case "PageDown":
		return "\\e[6~"
	}

	// Handle function keys F1-F12
	if len(key) >= 2 && key[0] == 'F' {
		return "\\e" + key // Fish uses \eF1, \eF2, etc.
	}

	// Parse modifiers
	ctrl, meta, base := extractModifiersForDenorm(key)

	// Build fish notation
	if ctrl && meta {
		// C-M-X -> \e\cx
		return "\\e\\c" + strings.ToLower(base)
	}
	if ctrl {
		// C-X -> \cx
		return "\\c" + strings.ToLower(base)
	}
	if meta {
		// M-X -> \ex
		return "\\e" + strings.ToLower(base)
	}

	// Single character: return lowercase
	return strings.ToLower(base)
}

// denormalizeBash converts standard notation to bash/readline format.
// C-G -> \C-g, M-F -> \M-f, C-M-X -> \C-\M-x
func denormalizeBash(key string) string {
	// Handle special keys first
	switch key {
	case "Enter":
		return "\\C-m"
	case "Tab":
		return "\\C-i"
	case "Escape":
		return "\\e"
	case "Backspace":
		return "\\C-h"
	case "Delete":
		return "\\C-?"
	case "Space":
		return " "
	case "Up":
		return "\\e[A"
	case "Down":
		return "\\e[B"
	case "Right":
		return "\\e[C"
	case "Left":
		return "\\e[D"
	case "Home":
		return "\\e[H"
	case "End":
		return "\\e[F"
	case "PageUp":
		return "\\e[5~"
	case "PageDown":
		return "\\e[6~"
	}

	// Handle function keys F1-F12
	if len(key) >= 2 && key[0] == 'F' {
		return "\\e" + key
	}

	// Parse modifiers
	ctrl, meta, base := extractModifiersForDenorm(key)

	// Build bash notation
	if ctrl && meta {
		// C-M-X -> \C-\M-x
		return "\\C-\\M-" + strings.ToLower(base)
	}
	if ctrl {
		// C-X -> \C-x
		return "\\C-" + strings.ToLower(base)
	}
	if meta {
		// M-X -> \M-x
		return "\\M-" + strings.ToLower(base)
	}

	// Single character: return lowercase
	return strings.ToLower(base)
}

// denormalizeZsh converts standard notation to zsh format.
// C-G -> ^g, M-F -> ^[f, C-M-X -> ^[^x
func denormalizeZsh(key string) string {
	// Handle special keys first
	switch key {
	case "Enter":
		return "^m"
	case "Tab":
		return "^i"
	case "Escape":
		return "^["
	case "Backspace":
		return "^h"
	case "Delete":
		return "^?"
	case "Space":
		return " "
	case "Up":
		return "^[[A"
	case "Down":
		return "^[[B"
	case "Right":
		return "^[[C"
	case "Left":
		return "^[[D"
	case "Home":
		return "^[[H"
	case "End":
		return "^[[F"
	case "PageUp":
		return "^[[5~"
	case "PageDown":
		return "^[[6~"
	}

	// Handle function keys F1-F12
	if len(key) >= 2 && key[0] == 'F' {
		return "^[" + key
	}

	// Parse modifiers
	ctrl, meta, base := extractModifiersForDenorm(key)

	// Build zsh notation
	if ctrl && meta {
		// C-M-X -> ^[^x (meta first, then ctrl)
		return "^[^" + strings.ToLower(base)
	}
	if ctrl {
		// C-X -> ^x
		return "^" + strings.ToLower(base)
	}
	if meta {
		// M-X -> ^[x
		return "^[" + strings.ToLower(base)
	}

	// Single character: return lowercase
	return strings.ToLower(base)
}

// denormalizeNvim converts standard notation to Neovim format.
// C-G -> <C-g>, M-F -> <M-f>, C-M-X -> <C-M-x>
// Neovim uses <> notation for special keys and modifiers.
func denormalizeNvim(key string) string {
	// Handle special keys first
	switch key {
	case "Enter":
		return "<CR>"
	case "Tab":
		return "<Tab>"
	case "Escape":
		return "<Esc>"
	case "Space":
		return "<Space>"
	case "Backspace":
		return "<BS>"
	case "Delete":
		return "<Del>"
	case "Up":
		return "<Up>"
	case "Down":
		return "<Down>"
	case "Left":
		return "<Left>"
	case "Right":
		return "<Right>"
	case "Home":
		return "<Home>"
	case "End":
		return "<End>"
	case "PageUp":
		return "<PageUp>"
	case "PageDown":
		return "<PageDown>"
	case "Insert":
		return "<Insert>"
	}

	// Handle function keys F1-F12
	if len(key) >= 2 && key[0] == 'F' && unicode.IsDigit(rune(key[1])) {
		return "<" + key + ">"
	}

	// Handle leader key
	if strings.ToLower(key) == "leader" {
		return "<leader>"
	}

	// Parse modifiers
	ctrl, meta, base := extractModifiersForDenorm(key)

	// Build nvim notation with <> brackets
	if ctrl || meta {
		var sb strings.Builder
		sb.WriteString("<")
		if ctrl {
			sb.WriteString("C-")
		}
		if meta {
			sb.WriteString("M-")
		}
		sb.WriteString(strings.ToLower(base))
		sb.WriteString(">")
		return sb.String()
	}

	// Single character: return as-is
	return base
}

// extractModifiersForDenorm extracts ctrl, meta, and base key from standard notation.
// Used specifically by denormalization functions.
// "C-M-X" -> (true, true, "X")
// "C-P" -> (true, false, "P")
// "M-F" -> (false, true, "F")
// "X" -> (false, false, "X")
func extractModifiersForDenorm(key string) (ctrl, meta bool, base string) {
	remaining := key

	// Check for C- prefix
	if strings.HasPrefix(remaining, "C-") {
		ctrl = true
		remaining = remaining[2:]
	}

	// Check for M- prefix
	if strings.HasPrefix(remaining, "M-") {
		meta = true
		remaining = remaining[2:]
	}

	// Also check for M-C- order (normalize to C-M-)
	if strings.HasPrefix(remaining, "C-") {
		ctrl = true
		remaining = remaining[2:]
	}

	// Strip S- prefix (shift) — shells typically don't use shift modifier notation
	remaining = strings.TrimPrefix(remaining, "S-")

	base = remaining
	return
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
