package config

import "github.com/grovetools/core/tui/theme/themecfg"

// init publishes grove.toml's tui.theme and tui.icons to core/tui/theme.
//
// theme used to call LoadDefault itself, which is what dragged core/config —
// and through it the workspace, plan and git graph — into every program that
// wanted a colour. It now reads a resolver seam instead (core/tui/theme/
// themecfg, stdlib-only), and this is the grove-config implementation of that
// seam. A sidecar panel installs its own resolver over the host's theme
// tokens and links none of this.
//
// The registration lives here rather than in an adapter package under theme
// because an adapter would have to be blank-imported by every binary to keep
// in-tree behaviour unchanged, and core/config cannot blank-import it without
// an import cycle. Every program that reads grove.toml already imports this
// package, so wiring it here is what makes the decoupling invisible in-tree.
//
// The resolver itself is lazy: LoadDefault runs when theme asks, and only in
// a program that actually links theme.
func init() {
	themecfg.SetResolver(func() themecfg.Selection {
		cfg, err := LoadDefault()
		if err != nil || cfg == nil || cfg.TUI == nil {
			return themecfg.Selection{}
		}
		return themecfg.Selection{Theme: cfg.TUI.Theme, Icons: cfg.TUI.Icons}
	})
}
