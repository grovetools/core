package keybind

// KnownDefaults contains shell keybindings that exist by default but may not
// appear in `bind` output unless explicitly overridden.
// Organized by shell and mode (emacs/vi).
var KnownDefaults = map[string]map[string]string{
	// Fish shell emacs mode (default)
	"fish-emacs": {
		// Movement
		"C-A": "beginning-of-line",
		"C-E": "end-of-line",
		"C-F": "forward-char",
		"C-B": "backward-char",
		"M-F": "forward-word",
		"M-B": "backward-word",

		// History
		"C-P": "up-or-search",
		"C-N": "down-or-search",
		"C-R": "history-search-backward",
		"M-P": "history-prefix-search-backward",
		"M-N": "history-prefix-search-forward",

		// Editing
		"C-D": "delete-char",
		"C-H": "backward-delete-char",
		"C-W": "backward-kill-word",
		"C-K": "kill-line",
		"C-U": "backward-kill-line",
		"C-Y": "yank",
		"M-D": "kill-word",

		// Control
		"C-C": "cancel-commandline",
		"C-L": "clear-screen",
		"C-Z": "suspend",
		"C-J": "execute",
		"C-M": "execute",

		// Tab completion
		"Tab": "complete",

		// Misc
		"M-.":     "history-token-search-backward",
		"M-<":     "beginning-of-buffer",
		"M->":     "end-of-buffer",
		"C-T":     "transpose-chars",
		"M-T":     "transpose-words",
		"C-_":     "undo",
		"M-U":     "upcase-word",
		"M-L":     "downcase-word",
		"M-C":     "capitalize-word",
		"Escape":  "cancel",
		"C-X":     "",         // Prefix for extended commands
		"C-Space": "set-mark", // Start selection
	},

	// Fish shell vi mode
	"fish-vi": {
		// Normal mode (subset)
		"H":      "backward-char",
		"L":      "forward-char",
		"K":      "up-or-search",
		"J":      "down-or-search",
		"W":      "forward-word",
		"B":      "backward-word",
		"0":      "beginning-of-line",
		"$":      "end-of-line",
		"Escape": "repaint-mode",
	},

	// Bash emacs mode (readline)
	"bash-emacs": {
		// Movement
		"C-A": "beginning-of-line",
		"C-E": "end-of-line",
		"C-F": "forward-char",
		"C-B": "backward-char",
		"M-F": "forward-word",
		"M-B": "backward-word",

		// History
		"C-P": "previous-history",
		"C-N": "next-history",
		"C-R": "reverse-search-history",
		"C-S": "forward-search-history",
		"M-<": "beginning-of-history",
		"M->": "end-of-history",

		// Editing
		"C-D": "delete-char",
		"C-H": "backward-delete-char",
		"C-W": "unix-word-rubout",
		"C-K": "kill-line",
		"C-U": "unix-line-discard",
		"C-Y": "yank",
		"M-D": "kill-word",

		// Control
		"C-C": "abort",
		"C-L": "clear-screen",
		"C-Z": "suspend",
		"C-J": "accept-line",
		"C-M": "accept-line",

		// Tab completion
		"Tab": "complete",

		// Misc
		"C-T":     "transpose-chars",
		"M-T":     "transpose-words",
		"C-_":     "undo",
		"M-U":     "upcase-word",
		"M-L":     "downcase-word",
		"M-C":     "capitalize-word",
		"C-X C-E": "edit-and-execute-command",
	},

	// Zsh emacs mode (ZLE)
	"zsh-emacs": {
		// Movement
		"C-A": "beginning-of-line",
		"C-E": "end-of-line",
		"C-F": "forward-char",
		"C-B": "backward-char",
		"M-F": "forward-word",
		"M-B": "backward-word",

		// History
		"C-P": "up-line-or-history",
		"C-N": "down-line-or-history",
		"C-R": "history-incremental-search-backward",
		"C-S": "history-incremental-search-forward",

		// Editing
		"C-D": "delete-char-or-list",
		"C-H": "backward-delete-char",
		"C-W": "backward-kill-word",
		"C-K": "kill-line",
		"C-U": "kill-whole-line",
		"C-Y": "yank",
		"M-D": "kill-word",

		// Control
		"C-C": "send-break",
		"C-L": "clear-screen",
		"C-Z": "suspend",
		"C-J": "accept-line",
		"C-M": "accept-line",

		// Tab completion
		"Tab": "expand-or-complete",

		// Misc
		"C-T":    "transpose-chars",
		"M-T":    "transpose-words",
		"C-_":    "undo",
		"M-U":    "up-case-word",
		"M-L":    "down-case-word",
		"M-C":    "capitalize-word",
		"M-.":    "insert-last-word",
		"M-'":    "quote-line",
		"M-\"":   "quote-region",
		"Escape": "vi-cmd-mode",
	},

	// Tmux default bindings (prefix table)
	"tmux-prefix": {
		"D":         "detach-client",
		"C":         "new-window",
		"N":         "next-window",
		"P":         "previous-window",
		"L":         "last-window",
		"W":         "choose-tree -w",
		"S":         "choose-tree -s",
		"\"":        "split-window",
		"%":         "split-window -h",
		"O":         "select-pane -t :.+",
		";":         "last-pane",
		"X":         "confirm-before -p 'kill-pane #P? (y/n)' kill-pane",
		"Z":         "resize-pane -Z",
		"!":         "break-pane",
		"&":         "confirm-before -p 'kill-window #W? (y/n)' kill-window",
		"?":         "list-keys",
		":":         "command-prompt",
		"[":         "copy-mode",
		"]":         "paste-buffer",
		"$":         "command-prompt -I #S 'rename-session %1'",
		",":         "command-prompt -I #W 'rename-window %1'",
		"0":         "select-window -t :0",
		"1":         "select-window -t :1",
		"2":         "select-window -t :2",
		"3":         "select-window -t :3",
		"4":         "select-window -t :4",
		"5":         "select-window -t :5",
		"6":         "select-window -t :6",
		"7":         "select-window -t :7",
		"8":         "select-window -t :8",
		"9":         "select-window -t :9",
		"Space":     "next-layout",
		"Up":        "select-pane -U",
		"Down":      "select-pane -D",
		"Left":      "select-pane -L",
		"Right":     "select-pane -R",
		"M-1":       "select-layout even-horizontal",
		"M-2":       "select-layout even-vertical",
		"M-3":       "select-layout main-horizontal",
		"M-4":       "select-layout main-vertical",
		"M-5":       "select-layout tiled",
		"M-Up":      "resize-pane -U 5",
		"M-Down":    "resize-pane -D 5",
		"M-Left":    "resize-pane -L 5",
		"M-Right":   "resize-pane -R 5",
		"C-Up":      "resize-pane -U",
		"C-Down":    "resize-pane -D",
		"C-Left":    "resize-pane -L",
		"C-Right":   "resize-pane -R",
		"{":         "swap-pane -U",
		"}":         "swap-pane -D",
		"T":         "clock-mode",
		"Q":         "display-panes",
		"I":         "display-message",
		"R":         "refresh-client",
		"~":         "show-messages",
		"F":         "command-prompt 'find-window -Z %1'",
		"M-N":       "next-window -a",
		"M-P":       "previous-window -a",
		"PageUp":    "copy-mode -u",
		"(":         "switch-client -p",
		")":         "switch-client -n",
		"Enter":     "copy-mode",
		"C-O":       "rotate-window",
		"M-O":       "rotate-window -D",
		"C-Z":       "suspend-client",
	},

	// Tmux default root table bindings (typically empty unless user adds)
	"tmux-root": {
		// By default, only C-b (prefix) is bound in root
		// Users typically don't bind directly to root without modifiers
	},
}

// GetKnownDefaults returns known default bindings for a specific shell/mode.
func GetKnownDefaults(shellMode string) map[string]string {
	if defaults, ok := KnownDefaults[shellMode]; ok {
		return defaults
	}
	return nil
}

// GetDefaultBinding returns the default action for a key in a specific shell/mode.
func GetDefaultBinding(shellMode, key string) (string, bool) {
	defaults := GetKnownDefaults(shellMode)
	if defaults == nil {
		return "", false
	}
	normalizedKey := Normalize(key, "")
	if action, ok := defaults[normalizedKey]; ok {
		return action, true
	}
	return "", false
}

// IsKnownDefault checks if a binding matches a known default.
func IsKnownDefault(shellMode, key, action string) bool {
	expectedAction, ok := GetDefaultBinding(shellMode, key)
	if !ok {
		return false
	}
	return expectedAction == action
}
