package theme

import (
	"testing"

	"github.com/grovetools/core/tui/theme/themecfg"
)

// restoreSelection puts the process-global theme and icon set back the way the
// test found them. Both are package variables, so a test that moves them has
// to move them back.
func restoreSelection(t *testing.T) {
	t.Helper()
	name := DefaultTheme.Name
	ascii := ASCIIIcons
	t.Cleanup(func() {
		themecfg.SetResolver(nil)
		_ = SetTheme(name)
		applyIcons(ascii)
	})
}

func TestSelectionResolverAppliesLate(t *testing.T) {
	t.Setenv("GROVE_THEME", "")
	t.Setenv("GROVE_ICONS", "")
	restoreSelection(t)

	// The resolver stands in for a package that registered after this one's
	// variable initialisation had already picked the default.
	themecfg.SetResolver(func() themecfg.Selection {
		return themecfg.Selection{Theme: "tokyonight", Icons: "ascii"}
	})

	if got := DefaultTheme.Name; got != "tokyonight" {
		t.Errorf("theme after late resolver = %q, want %q", got, "tokyonight")
	}
	if !ASCIIIcons {
		t.Error("ASCIIIcons = false after a resolver reporting ascii, want true")
	}
}

func TestEnvironmentBeatsResolver(t *testing.T) {
	t.Setenv("GROVE_THEME", "gruvbox")
	t.Setenv("GROVE_ICONS", "ascii")
	restoreSelection(t)

	themecfg.SetResolver(func() themecfg.Selection {
		return themecfg.Selection{Theme: "tokyonight", Icons: "nerd"}
	})

	if got := getThemeName(); got != "gruvbox" {
		t.Errorf("getThemeName with GROVE_THEME set = %q, want %q", got, "gruvbox")
	}
	if !useASCIIIcons() {
		t.Error("useASCIIIcons with GROVE_ICONS=ascii = false, want true")
	}
}

func TestNoResolverFallsBackToDefaults(t *testing.T) {
	t.Setenv("GROVE_THEME", "")
	t.Setenv("GROVE_ICONS", "")
	restoreSelection(t)

	themecfg.SetResolver(nil)

	if got := getThemeName(); got != DefaultThemeName {
		t.Errorf("getThemeName without a resolver = %q, want %q", got, DefaultThemeName)
	}
	if useASCIIIcons() {
		t.Error("useASCIIIcons without a resolver = true, want false (Nerd Font default)")
	}
}
