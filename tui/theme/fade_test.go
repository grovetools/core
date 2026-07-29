package theme

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// A fade must actually interpolate: each step closer to the background, from the
// role's own color rather than from whatever the renderer's color profile happens
// to flatten it to.
func TestFadeToBackgroundInterpolatesTowardTheBackground(t *testing.T) {
	palette, ok := ActivePaletteColors()
	if !ok {
		t.Skip("active theme is an ANSI passthrough palette; nothing to blend")
	}
	if !isHexColor(palette.Magenta) || !isHexColor(palette.Bg) {
		t.Fatalf("palette roles are not hex: magenta=%q bg=%q", palette.Magenta, palette.Bg)
	}

	full, ok := FadeToBackground(palette.Magenta, 1)
	if !ok {
		t.Fatal("alpha 1 could not be resolved")
	}
	if got := hexString(t, full); got != palette.Magenta {
		t.Errorf("alpha 1 = %s, want the role unchanged (%s)", got, palette.Magenta)
	}

	// Every step must differ from the last and end adjacent to the background.
	prev := palette.Magenta
	for _, alpha := range []float64{0.75, 0.5, 0.25, 0.1} {
		c, ok := FadeToBackground(palette.Magenta, alpha)
		if !ok {
			t.Fatalf("alpha %v could not be resolved", alpha)
		}
		got := hexString(t, c)
		if got == prev {
			t.Errorf("alpha %v produced %s again — the fade is not advancing", alpha, got)
		}
		prev = got
	}

	zero, ok := FadeToBackground(palette.Magenta, 0)
	if !ok {
		t.Fatal("alpha 0 could not be resolved")
	}
	_, bg, _, bgOK := ActiveTerminalColors()
	if bgOK && hexString(t, zero) != bg {
		t.Errorf("alpha 0 = %s, want the background %s", hexString(t, zero), bg)
	}
}

// Callers must be able to tell "no fade available" from a silently wrong color,
// because the honest fallback (render it unfaded) is theirs to choose.
func TestFadeToBackgroundRejectsNonHexRoles(t *testing.T) {
	for _, role := range []string{"", "13", "magenta", "#xyzxyz"} {
		if _, ok := FadeToBackground(role, 0.5); ok {
			t.Errorf("role %q reported a usable fade", role)
		}
	}
}

// hexString recovers the literal a resolved fade color was built from.
// FadeToBackground returns lipgloss.Color (a string type) holding "#rrggbb", so
// this comparison never goes through the renderer's color profile — which is the
// whole reason the API is hex in and hex out.
func hexString(t *testing.T, c lipgloss.TerminalColor) string {
	t.Helper()
	hex, ok := c.(lipgloss.Color)
	if !ok {
		t.Fatalf("color %T is not a lipgloss.Color carrying its literal", c)
	}
	return string(hex)
}
