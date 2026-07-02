package theme

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"sort"

	"github.com/charmbracelet/lipgloss"
)

//go:embed themes/*.toml
var themesFS embed.FS

// familyEntry tracks the default palette per appearance for a theme family.
type familyEntry struct {
	dark  *Palette
	light *Palette
}

// paletteRegistry holds every successfully loaded palette, keyed by name,
// plus the per-family default variants used to build adaptive legacy colors.
type paletteRegistry struct {
	palettes map[string]*Palette
	families map[string]*familyEntry
}

// registry is the process-wide palette registry, loaded from the embedded
// theme TOML files at package init.
var registry = newPaletteRegistry(loadPalettesFS(themesFS, "themes"))

// themeRegistry maps selectable theme names (families and individual
// variants) to legacy Colors builders. Family names resolve to adaptive
// light/dark pairs when both appearances exist.
var themeRegistry = registry.legacyBuilders()

// loadPalettesFS loads and validates every *.toml palette in dir of fsys. A
// file that fails to parse or validate is skipped with a warning so a single
// bad theme cannot sink the registry.
func loadPalettesFS(fsys fs.FS, dir string) []*Palette {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "grove: theme registry: reading %s: %v\n", dir, err)
		return nil
	}
	var palettes []*Palette
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := dir + "/" + entry.Name()
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "grove: theme registry: skipping %s: %v\n", path, err)
			continue
		}
		palette, err := parsePalette(data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "grove: theme registry: skipping %s: %v\n", path, err)
			continue
		}
		palettes = append(palettes, palette)
	}
	return palettes
}

// newPaletteRegistry indexes palettes by name and computes each family's
// default variant per appearance. Duplicate names are skipped with a warning.
func newPaletteRegistry(palettes []*Palette) *paletteRegistry {
	r := &paletteRegistry{
		palettes: make(map[string]*Palette),
		families: make(map[string]*familyEntry),
	}
	sort.SliceStable(palettes, func(i, j int) bool {
		return palettes[i].Meta.Name < palettes[j].Meta.Name
	})
	for _, p := range palettes {
		if _, exists := r.palettes[p.Meta.Name]; exists {
			fmt.Fprintf(os.Stderr, "grove: theme registry: skipping duplicate theme %q\n", p.Meta.Name)
			continue
		}
		r.palettes[p.Meta.Name] = p

		entry := r.families[p.Meta.Family]
		if entry == nil {
			entry = &familyEntry{}
			r.families[p.Meta.Family] = entry
		}
		slot := &entry.dark
		if p.Meta.Appearance == "light" {
			slot = &entry.light
		}
		// First palette wins unless a later one is flagged as the default.
		if *slot == nil || (p.Meta.Default && !(*slot).Meta.Default) {
			*slot = p
		}
	}
	return r
}

// legacyBuilders produces the name → Colors builder map consumed by
// resolveThemeColors. Every variant is selectable by its palette name;
// family names are registered last so they win on collision.
func (r *paletteRegistry) legacyBuilders() map[string]func() Colors {
	builders := make(map[string]func() Colors, len(r.palettes)+len(r.families))
	for name, p := range r.palettes {
		builders[name] = singleColorsBuilder(p)
	}
	for name, entry := range r.families {
		builders[name] = familyColorsBuilder(entry)
	}
	return builders
}

// singleColorsBuilder derives static legacy Colors from one palette.
func singleColorsBuilder(p *Palette) func() Colors {
	v := p.legacyValues()
	return func() Colors {
		return Colors{
			Green:                lipgloss.Color(v.Green),
			Yellow:               lipgloss.Color(v.Yellow),
			Red:                  lipgloss.Color(v.Red),
			Orange:               lipgloss.Color(v.Orange),
			Cyan:                 lipgloss.Color(v.Cyan),
			Blue:                 lipgloss.Color(v.Blue),
			Violet:               lipgloss.Color(v.Violet),
			Pink:                 lipgloss.Color(v.Pink),
			LightText:            lipgloss.Color(v.LightText),
			MutedText:            lipgloss.Color(v.MutedText),
			DarkText:             lipgloss.Color(v.DarkText),
			Border:               lipgloss.Color(v.Border),
			SelectedBackground:   lipgloss.Color(v.SelectedBackground),
			SubtleBackground:     lipgloss.Color(v.SubtleBackground),
			VerySubtleBackground: lipgloss.Color(v.VerySubtleBackground),
		}
	}
}

// familyColorsBuilder derives legacy Colors for a family. Families with both
// appearances get AdaptiveColor light/dark pairs; single-appearance families
// fall back to static colors from the available palette.
func familyColorsBuilder(entry *familyEntry) func() Colors {
	if entry.dark == nil {
		return singleColorsBuilder(entry.light)
	}
	if entry.light == nil {
		return singleColorsBuilder(entry.dark)
	}
	light := entry.light.legacyValues()
	dark := entry.dark.legacyValues()
	adaptive := func(l, d string) lipgloss.TerminalColor {
		return lipgloss.AdaptiveColor{Light: l, Dark: d}
	}
	return func() Colors {
		return Colors{
			Green:                adaptive(light.Green, dark.Green),
			Yellow:               adaptive(light.Yellow, dark.Yellow),
			Red:                  adaptive(light.Red, dark.Red),
			Orange:               adaptive(light.Orange, dark.Orange),
			Cyan:                 adaptive(light.Cyan, dark.Cyan),
			Blue:                 adaptive(light.Blue, dark.Blue),
			Violet:               adaptive(light.Violet, dark.Violet),
			Pink:                 adaptive(light.Pink, dark.Pink),
			LightText:            adaptive(light.LightText, dark.LightText),
			MutedText:            adaptive(light.MutedText, dark.MutedText),
			DarkText:             adaptive(light.DarkText, dark.DarkText),
			Border:               adaptive(light.Border, dark.Border),
			SelectedBackground:   adaptive(light.SelectedBackground, dark.SelectedBackground),
			SubtleBackground:     adaptive(light.SubtleBackground, dark.SubtleBackground),
			VerySubtleBackground: adaptive(light.VerySubtleBackground, dark.VerySubtleBackground),
		}
	}
}

// fallbackColors is the last-resort palette used if the embedded registry is
// somehow empty. It uses only standard ANSI colors so it works everywhere.
func fallbackColors() Colors {
	return Colors{
		Green:                lipgloss.Color("2"),
		Yellow:               lipgloss.Color("3"),
		Red:                  lipgloss.Color("1"),
		Orange:               lipgloss.Color("11"),
		Cyan:                 lipgloss.Color("6"),
		Blue:                 lipgloss.Color("4"),
		Violet:               lipgloss.Color("5"),
		Pink:                 lipgloss.Color("13"),
		LightText:            lipgloss.Color("7"),
		MutedText:            lipgloss.Color("8"),
		DarkText:             lipgloss.Color("0"),
		Border:               lipgloss.Color("8"),
		SelectedBackground:   lipgloss.Color("8"),
		SubtleBackground:     lipgloss.Color("0"),
		VerySubtleBackground: lipgloss.Color("0"),
	}
}

// List returns metadata for every loaded palette, sorted by family,
// appearance (dark first), and name.
func List() []PaletteMeta {
	metas := make([]PaletteMeta, 0, len(registry.palettes))
	for _, p := range registry.palettes {
		metas = append(metas, p.Meta)
	}
	sort.Slice(metas, func(i, j int) bool {
		if metas[i].Family != metas[j].Family {
			return metas[i].Family < metas[j].Family
		}
		if metas[i].Appearance != metas[j].Appearance {
			return metas[i].Appearance == "dark"
		}
		return metas[i].Name < metas[j].Name
	})
	return metas
}

// Lookup returns the fully derived palette registered under name. The name
// is normalized and theme aliases are honored; alias targets that are family
// names resolve to the family's default dark (or only) variant.
func Lookup(name string) (Palette, bool) {
	key := normalizeThemeName(name)
	if alias, ok := themeAliases[key]; ok {
		key = alias
	}
	if p, ok := registry.palettes[key]; ok {
		return *p, true
	}
	if entry, ok := registry.families[key]; ok {
		if entry.dark != nil {
			return *entry.dark, true
		}
		if entry.light != nil {
			return *entry.light, true
		}
	}
	return Palette{}, false
}

// Names returns every selectable theme name (families and individual
// variants), sorted.
func Names() []string {
	seen := make(map[string]bool, len(themeRegistry))
	names := make([]string, 0, len(themeRegistry))
	for name := range themeRegistry {
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// IsPinned reports whether the GROVE_THEME environment variable pins the
// theme for this process. SetTheme is a no-op while pinned.
func IsPinned() bool {
	return normalizeThemeName(os.Getenv("GROVE_THEME")) != ""
}

// SetTheme re-themes the running process: it rebuilds DefaultTheme and
// DefaultColors and refreshes the exported color shortcuts. It is a no-op
// when GROVE_THEME pins the theme (the environment always wins) and returns
// an error for unknown theme names.
func SetTheme(name string) error {
	if IsPinned() {
		return nil
	}
	key := normalizeThemeName(name)
	if alias, ok := themeAliases[key]; ok {
		key = alias
	}
	builder, ok := themeRegistry[key]
	if !ok {
		return fmt.Errorf("unknown theme %q", name)
	}
	colors := builder()
	applyColors(colors)
	DefaultTheme = newThemeFromColors(colors, key)
	return nil
}
