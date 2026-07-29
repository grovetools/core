package config

// This file implements core/tui/keymap.KeybindingSource for *Config.
//
// keymap used to take *Config directly, which is what put core/config — and
// through it the workspace, plan and git graph — behind every keymap. It now
// takes a four-method interface instead, and these methods are what keep
// `keymap.Load(cfg, "flow.status")` compiling unchanged at ~45 call sites
// across the ecosystem: the argument is still the same *Config, it just
// arrives through an interface the keymap package can describe on its own.
//
// The interface is deliberately not imported here — implementing it
// structurally is what keeps the dependency pointing one way.
//
// Every method is nil-safe on the receiver: a caller holding a nil *Config
// hands keymap a non-nil interface wrapping a nil pointer, and that has to
// behave like "nothing configured" rather than panic.

// KeymapPreset returns [tui].preset — "vim", "emacs" or "arrows" — or "" when
// unset.
func (c *Config) KeymapPreset() string {
	if c == nil || c.TUI == nil {
		return ""
	}
	return c.TUI.Preset
}

// KeybindingSection returns the global keybinding overrides for one of the
// reserved section names, or nil when the section is absent.
func (c *Config) KeybindingSection(section string) map[string][]string {
	if c == nil || c.TUI == nil || c.TUI.Keybindings == nil {
		return nil
	}
	kb := c.TUI.Keybindings
	switch section {
	case "navigation":
		return kb.Navigation
	case "selection":
		return kb.Selection
	case "actions":
		return kb.Actions
	case "search":
		return kb.Search
	case "view":
		return kb.View
	case "fold":
		return kb.Fold
	case "system":
		return kb.System
	default:
		return nil
	}
}

// TUIKeybindings returns the per-TUI overrides for pkg/tui — e.g.
// ("flow", "status") for [tui.keybindings.flow.status] — or nil when none are
// configured. It reads through GetTUIOverrides, so the legacy `overrides`
// spelling keeps working.
func (c *Config) TUIKeybindings(pkg, tui string) map[string][]string {
	if c == nil || c.TUI == nil || c.TUI.Keybindings == nil {
		return nil
	}
	pkgOverrides, ok := c.TUI.Keybindings.GetTUIOverrides()[pkg]
	if !ok {
		return nil
	}
	overrides, ok := pkgOverrides[tui]
	if !ok {
		return nil
	}
	return overrides
}

// WhichKeyDelayMillis returns [tui].whichkey_delay_ms. The bool distinguishes
// "unset" (use the built-in delay) from an explicit 0 (show the popup
// immediately).
func (c *Config) WhichKeyDelayMillis() (int, bool) {
	if c == nil || c.TUI == nil || c.TUI.WhichKeyDelayMs == nil {
		return 0, false
	}
	return *c.TUI.WhichKeyDelayMs, true
}
