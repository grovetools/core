package theme

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/charmbracelet/lipgloss"
)

// TestLoadPalettesFSSkipsBadFiles asserts a broken theme file does not sink
// the rest of the registry.
func TestLoadPalettesFSSkipsBadFiles(t *testing.T) {
	fsys := fstest.MapFS{
		"themes/good.toml":       {Data: []byte(validThemeTOML)},
		"themes/unparsable.toml": {Data: []byte("this is not toml = [")},
		// Parses but fails validation (required role missing).
		"themes/invalid.toml": {Data: []byte(strings.Replace(validThemeTOML, "red = \"#FF5D62\"\n", "", 1))},
	}
	palettes := loadPalettesFS(fsys, "themes")
	if len(palettes) != 1 {
		t.Fatalf("loadPalettesFS() returned %d palettes, want 1", len(palettes))
	}
	if palettes[0].Meta.Name != "testtheme-dark" {
		t.Errorf("loaded palette = %q, want testtheme-dark", palettes[0].Meta.Name)
	}
}

// TestEmbeddedThemesAllLoad asserts every embedded TOML file parses and
// validates, so none get silently skipped at init.
func TestEmbeddedThemesAllLoad(t *testing.T) {
	entries, err := themesFS.ReadDir("themes")
	if err != nil {
		t.Fatalf("reading embedded themes: %v", err)
	}
	palettes := loadPalettesFS(themesFS, "themes")
	if len(palettes) != len(entries) {
		t.Errorf("loaded %d of %d embedded theme files; a file was skipped", len(palettes), len(entries))
	}
	if len(registry.palettes) != len(entries) {
		t.Errorf("registry holds %d palettes, want %d", len(registry.palettes), len(entries))
	}
}

func TestNewPaletteRegistrySkipsDuplicates(t *testing.T) {
	a, err := parsePalette([]byte(validThemeTOML))
	if err != nil {
		t.Fatal(err)
	}
	b, err := parsePalette([]byte(validThemeTOML))
	if err != nil {
		t.Fatal(err)
	}
	r := newPaletteRegistry([]*Palette{a, b})
	if len(r.palettes) != 1 {
		t.Errorf("registry holds %d palettes, want 1 (duplicate skipped)", len(r.palettes))
	}
}

func TestDefaultVariantSelection(t *testing.T) {
	first, err := parsePalette([]byte(strings.Replace(validThemeTOML, "name = \"testtheme-dark\"", "name = \"testtheme-a\"", 1)))
	if err != nil {
		t.Fatal(err)
	}
	flagged, err := parsePalette([]byte(strings.Replace(validThemeTOML,
		"name = \"testtheme-dark\"", "name = \"testtheme-z\"\ndefault = true", 1)))
	if err != nil {
		t.Fatal(err)
	}
	r := newPaletteRegistry([]*Palette{first, flagged})
	if got := r.families["testtheme"].dark.Meta.Name; got != "testtheme-z" {
		t.Errorf("family default = %q, want default-flagged testtheme-z", got)
	}

	// Without a default flag the alphabetically first variant wins.
	first2, _ := parsePalette([]byte(strings.Replace(validThemeTOML, "name = \"testtheme-dark\"", "name = \"testtheme-a\"", 1)))
	second2, _ := parsePalette([]byte(strings.Replace(validThemeTOML, "name = \"testtheme-dark\"", "name = \"testtheme-z\"", 1)))
	r = newPaletteRegistry([]*Palette{second2, first2})
	if got := r.families["testtheme"].dark.Meta.Name; got != "testtheme-a" {
		t.Errorf("family default = %q, want alphabetically first testtheme-a", got)
	}
}

func TestList(t *testing.T) {
	metas := List()
	if len(metas) < 5 {
		t.Fatalf("List() returned %d palettes, want at least the 5 original built-ins", len(metas))
	}

	// The original built-in palettes must always be present.
	byName := make(map[string]PaletteMeta, len(metas))
	for _, m := range metas {
		byName[m.Name] = m
	}
	for _, want := range []string{"gruvbox-dark", "gruvbox-light", "kanagawa-dark", "kanagawa-light", "terminal"} {
		if _, ok := byName[want]; !ok {
			t.Errorf("List() missing built-in palette %q", want)
		}
	}

	// Sorted by family, then appearance (dark first), then name.
	appearanceRank := func(a string) int {
		if a == "dark" {
			return 0
		}
		return 1
	}
	for i := 1; i < len(metas); i++ {
		prev, cur := metas[i-1], metas[i]
		switch {
		case prev.Family < cur.Family:
			continue
		case prev.Family > cur.Family:
			t.Fatalf("List() not sorted by family: %q before %q", prev.Name, cur.Name)
		case appearanceRank(prev.Appearance) < appearanceRank(cur.Appearance):
			continue
		case appearanceRank(prev.Appearance) > appearanceRank(cur.Appearance):
			t.Fatalf("List() dark variants must precede light within family: %q before %q", prev.Name, cur.Name)
		case prev.Name >= cur.Name:
			t.Fatalf("List() not sorted by name within family/appearance: %q before %q", prev.Name, cur.Name)
		}
	}

	for _, m := range metas {
		if m.Family == "" || m.Variant == "" || m.Appearance == "" {
			t.Errorf("palette %q has incomplete metadata: %+v", m.Name, m)
		}
	}
}

func TestLookup(t *testing.T) {
	p, ok := Lookup("kanagawa-dark")
	if !ok {
		t.Fatal("Lookup(kanagawa-dark) not found; alias should resolve")
	}
	// kanagawa-dark is aliased to the kanagawa family, whose default dark
	// variant is kanagawa-dark itself.
	if p.Meta.Name != "kanagawa-dark" {
		t.Errorf("Lookup(kanagawa-dark) = %q", p.Meta.Name)
	}

	p, ok = Lookup("kanagawa")
	if !ok || p.Meta.Appearance != "dark" {
		t.Errorf("Lookup(kanagawa) = %+v, %v; want family default dark variant", p.Meta, ok)
	}

	p, ok = Lookup("Kanagawa Light")
	if !ok || p.Meta.Name != "kanagawa-light" {
		t.Errorf("Lookup normalization failed: %+v, %v", p.Meta, ok)
	}

	if _, ok := Lookup("does-not-exist"); ok {
		t.Error("Lookup(does-not-exist) = ok, want not found")
	}
}

func TestNames(t *testing.T) {
	names := Names()
	got := make(map[string]bool, len(names))
	for _, n := range names {
		got[n] = true
	}
	for _, want := range []string{"kanagawa", "gruvbox", "terminal", "kanagawa-dark", "kanagawa-light", "gruvbox-dark", "gruvbox-light"} {
		if !got[want] {
			t.Errorf("Names() missing %q (got %v)", want, names)
		}
	}
}

func TestSetTheme(t *testing.T) {
	t.Setenv("GROVE_THEME", "")
	restoreDefaultTheme(t)

	if err := SetTheme("gruvbox"); err != nil {
		t.Fatalf("SetTheme(gruvbox) error: %v", err)
	}
	want := goldenGruvboxColors()
	compareColors(t, "gruvbox", want, DefaultColors)
	compareColors(t, "gruvbox", want, DefaultTheme.Colors)
	if DefaultTheme.Name != "gruvbox" {
		t.Errorf("DefaultTheme.Name = %q, want gruvbox", DefaultTheme.Name)
	}
	if Green != want.Green || VerySubtleBackground != want.VerySubtleBackground {
		t.Error("exported color shortcuts were not refreshed by SetTheme")
	}
	style := DefaultTheme.Success
	if style.GetForeground() != want.Green {
		t.Errorf("Success style foreground = %#v, want %#v", style.GetForeground(), want.Green)
	}

	// Aliases work through SetTheme too.
	if err := SetTheme("kanagawa-dragon"); err != nil {
		t.Fatalf("SetTheme(kanagawa-dragon) error: %v", err)
	}
	if DefaultTheme.Name != "kanagawa" {
		t.Errorf("DefaultTheme.Name = %q, want alias target kanagawa", DefaultTheme.Name)
	}
}

func TestSetThemeUnknown(t *testing.T) {
	t.Setenv("GROVE_THEME", "")
	restoreDefaultTheme(t)

	before := DefaultColors
	if err := SetTheme("definitely-not-a-theme"); err == nil {
		t.Fatal("SetTheme(unknown) = nil, want error")
	}
	compareColors(t, "unchanged", before, DefaultColors)
}

func TestSetThemePinnedByEnv(t *testing.T) {
	t.Setenv("GROVE_THEME", "kanagawa")
	restoreDefaultTheme(t)

	if !IsPinned() {
		t.Fatal("IsPinned() = false with GROVE_THEME set")
	}
	before := DefaultColors
	if err := SetTheme("gruvbox"); err != nil {
		t.Fatalf("SetTheme while pinned returned error: %v", err)
	}
	compareColors(t, "pinned", before, DefaultColors)

	t.Setenv("GROVE_THEME", "")
	if IsPinned() {
		t.Error("IsPinned() = true with GROVE_THEME empty")
	}
}

// restoreDefaultTheme restores the package-level theme state after tests
// that mutate it via SetTheme.
func restoreDefaultTheme(t *testing.T) {
	t.Helper()
	theme, colors := DefaultTheme, DefaultColors
	t.Cleanup(func() {
		DefaultTheme = theme
		applyColors(colors)
	})
}

func TestResolveColor(t *testing.T) {
	colors := resolveThemeColors("kanagawa")
	fallback := lipgloss.Color("#123456")

	cases := []struct {
		name string
		want lipgloss.TerminalColor
	}{
		{"green", colors.Green},
		{"muted_text", colors.MutedText},
		{"muted", colors.MutedText},
		{"BORDER", colors.Border},
		{"selected_background", colors.SelectedBackground},
		{"subtle_background", colors.SubtleBackground},
		{"very_subtle_background", colors.VerySubtleBackground},
		{"verysubtlebackground", colors.VerySubtleBackground},
		{"#AABBCC", lipgloss.Color("#AABBCC")},
		{"none", lipgloss.NoColor{}},
		{"", fallback},
		{"unknown-color", fallback},
	}
	for _, c := range cases {
		if got := colors.ResolveColor(c.name, fallback); got != c.want {
			t.Errorf("ResolveColor(%q) = %#v, want %#v", c.name, got, c.want)
		}
	}
}
