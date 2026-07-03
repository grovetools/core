package theme

import "testing"

// TestTerminalColorsForHexTheme asserts a concrete hex variant reports its
// palette's raw fg/bg and the 16 terminal slots in ANSI order.
func TestTerminalColorsForHexTheme(t *testing.T) {
	p, found := Lookup("kanagawa-wave")
	if !found {
		t.Fatal("kanagawa-wave palette missing from registry")
	}

	fg, bg, slots, ok := terminalColorsFor("kanagawa-wave", true)
	if !ok {
		t.Fatal("terminalColorsFor(kanagawa-wave) not ok")
	}
	if fg != p.Colors.Fg || bg != p.Colors.Bg {
		t.Errorf("fg/bg = %q/%q, want %q/%q", fg, bg, p.Colors.Fg, p.Colors.Bg)
	}
	want := [16]string{
		p.Terminal.Black, p.Terminal.Red, p.Terminal.Green, p.Terminal.Yellow,
		p.Terminal.Blue, p.Terminal.Magenta, p.Terminal.Cyan, p.Terminal.White,
		p.Terminal.BlackBright, p.Terminal.RedBright, p.Terminal.GreenBright, p.Terminal.YellowBright,
		p.Terminal.BlueBright, p.Terminal.MagentaBright, p.Terminal.CyanBright, p.Terminal.WhiteBright,
	}
	if slots != want {
		t.Errorf("terminal slots = %v, want %v", slots, want)
	}
	for i, v := range slots {
		if !isHexColor(v) {
			t.Errorf("slot %d = %q, want #rrggbb hex", i, v)
		}
	}
}

// TestTerminalColorsForANSITheme asserts the ANSI passthrough theme reports
// ok=false so callers reset instead of emitting index strings as hex.
func TestTerminalColorsForANSITheme(t *testing.T) {
	fg, bg, _, ok := terminalColorsFor("terminal", true)
	if ok {
		t.Fatal("terminalColorsFor(terminal) ok = true, want false for ANSI palette")
	}
	if fg != "" || bg != "" {
		t.Errorf("ANSI palette returned fg/bg %q/%q, want empty", fg, bg)
	}
}

// TestTerminalColorsForFamilyAppearance asserts a family name resolves to
// the variant matching the requested appearance.
func TestTerminalColorsForFamilyAppearance(t *testing.T) {
	dark, foundDark := Lookup("gruvbox-dark")
	light, foundLight := Lookup("gruvbox-light")
	if !foundDark || !foundLight {
		t.Fatal("gruvbox variants missing from registry")
	}

	if _, bg, _, ok := terminalColorsFor("gruvbox", true); !ok || bg != dark.Colors.Bg {
		t.Errorf("gruvbox dark bg = %q (ok=%v), want %q", bg, ok, dark.Colors.Bg)
	}
	if _, bg, _, ok := terminalColorsFor("gruvbox", false); !ok || bg != light.Colors.Bg {
		t.Errorf("gruvbox light bg = %q (ok=%v), want %q", bg, ok, light.Colors.Bg)
	}
	// A concrete variant ignores the appearance flag.
	if _, bg, _, ok := terminalColorsFor("gruvbox-dark", false); !ok || bg != dark.Colors.Bg {
		t.Errorf("gruvbox-dark light-mode bg = %q (ok=%v), want %q", bg, ok, dark.Colors.Bg)
	}
}

// TestTerminalColorsForUnknownTheme asserts an unknown selection reports the
// default theme's colors, mirroring resolveThemeColors' fallback (the screen
// is rendered with those colors, so terminals should match them).
func TestTerminalColorsForUnknownTheme(t *testing.T) {
	fallback, found := lookupForAppearance(DefaultThemeName, true)
	if !found {
		t.Fatalf("default theme %q missing from registry", DefaultThemeName)
	}
	_, bg, _, ok := terminalColorsFor("no-such-theme", true)
	if !ok || bg != fallback.Colors.Bg {
		t.Errorf("unknown theme bg = %q (ok=%v), want default theme bg %q", bg, ok, fallback.Colors.Bg)
	}
}

// TestActiveTerminalColorsTracksSetTheme asserts the exported helper follows
// the live theme.SetTheme resolution path.
func TestActiveTerminalColorsTracksSetTheme(t *testing.T) {
	t.Setenv("GROVE_THEME", "")
	orig := DefaultTheme.Name
	t.Cleanup(func() { _ = SetTheme(orig) })

	if err := SetTheme("kanagawa-wave"); err != nil {
		t.Fatalf("SetTheme: %v", err)
	}
	p, _ := Lookup("kanagawa-wave")
	if fg, bg, _, ok := ActiveTerminalColors(); !ok || fg != p.Colors.Fg || bg != p.Colors.Bg {
		t.Errorf("ActiveTerminalColors = %q/%q (ok=%v), want %q/%q", fg, bg, ok, p.Colors.Fg, p.Colors.Bg)
	}

	if err := SetTheme("terminal"); err != nil {
		t.Fatalf("SetTheme: %v", err)
	}
	if _, _, _, ok := ActiveTerminalColors(); ok {
		t.Error("ActiveTerminalColors ok = true under the terminal (ANSI) theme")
	}
}
