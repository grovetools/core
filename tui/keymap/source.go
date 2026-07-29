package keymap

// KeybindingSource is the shape this package needs from a configuration, and
// the only thing standing between the config-aware entry points (Load,
// ApplyTUIOverrides, WhichKeyDelay, NewWhichKeyHost) and the config-free rest
// of the package.
//
// In-tree the implementation is *config.Config from core/config, which grew
// these four methods so every existing `keymap.Load(cfg, "flow.status")` call
// keeps compiling verbatim. Out-of-tree — a sidecar panel, a plugin, a test —
// it is whatever the host can produce; the zero case is a nil source, which
// yields the vim preset and no overrides.
//
// Every method must be safe to call on a nil concrete receiver, because a
// caller holding a nil *config.Config hands this package a non-nil interface
// wrapping a nil pointer.
type KeybindingSource interface {
	// KeymapPreset returns the base preset name: "vim", "emacs" or "arrows".
	// An empty string means "unset" and selects vim.
	KeymapPreset() string

	// KeybindingSection returns the global overrides for one of the reserved
	// section names — "navigation", "selection", "actions", "search", "view",
	// "fold", "system" — mapping a snake_case action to its key list. A nil
	// return means no overrides for that section.
	KeybindingSection(section string) map[string][]string

	// TUIKeybindings returns the per-TUI overrides for one package and TUI,
	// e.g. ("flow", "status"). A nil return means none are configured.
	TUIKeybindings(pkg, tui string) map[string][]string

	// WhichKeyDelayMillis returns the configured which-key popup show delay in
	// milliseconds. The bool is false when the setting is absent, which is
	// distinct from an explicit 0 (show the popup immediately).
	WhichKeyDelayMillis() (int, bool)
}

// OverrideSections lists the reserved global section names a KeybindingSource
// is asked for, in the order Load applies them. These are the grove.toml
// spellings ("navigation"), not the help-display section titles the Section*
// constants carry ("Navigation").
var OverrideSections = []string{
	"navigation",
	"selection",
	"actions",
	"search",
	"view",
	"fold",
	"system",
}

// section reads one global section from src, tolerating both a nil interface
// and an interface wrapping a nil pointer.
func section(src KeybindingSource, name string) map[string][]string {
	if src == nil {
		return nil
	}
	return src.KeybindingSection(name)
}
