package theme

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/pelletier/go-toml/v2"
)

// PaletteMeta describes a theme palette's identity and provenance. It is the
// metadata half of a theme TOML file ([meta] table).
type PaletteMeta struct {
	// Name is the unique, kebab-case registry key for this palette
	// (e.g. "kanagawa-dark").
	Name string `toml:"name"`
	// Family groups related variants (e.g. "kanagawa", "gruvbox").
	Family string `toml:"family"`
	// Variant names this palette within its family (e.g. "dark", "mocha").
	Variant string `toml:"variant"`
	// Appearance is "dark" or "light".
	Appearance string `toml:"appearance"`
	// Default marks this palette as the family's default variant for its
	// appearance when a family has several variants per appearance.
	Default bool `toml:"default"`
	// ANSI marks a palette whose values are terminal ANSI color indices
	// ("0".."255") instead of hex colors. Blend-based derivations are
	// skipped for ANSI palettes.
	ANSI bool `toml:"ansi"`
	// Upstream is the URL of the source the palette was derived from.
	Upstream string `toml:"upstream"`
	// Author credits the upstream palette author(s).
	Author string `toml:"author"`
	// License is the SPDX identifier of the upstream license (e.g. "MIT").
	License string `toml:"license"`
}

// GitColors holds git-status accent colors ([palette.git] table). All fields
// are optional and derived from the base accents when absent.
type GitColors struct {
	Add    string `toml:"add"`    // defaults to green
	Change string `toml:"change"` // defaults to blue
	Delete string `toml:"delete"` // defaults to red
}

// DiagnosticColors holds diagnostic severity colors ([palette.diagnostics]
// table). All fields are optional and derived from the base accents when
// absent.
type DiagnosticColors struct {
	Error   string `toml:"error"`   // defaults to red
	Warning string `toml:"warning"` // defaults to yellow
	Info    string `toml:"info"`    // defaults to blue
	Hint    string `toml:"hint"`    // defaults to cyan
}

// TerminalColors holds the 16 terminal slot colors ([terminal] table). All
// fields are optional and derived from the base roles when absent.
type TerminalColors struct {
	Black         string `toml:"black"`
	Red           string `toml:"red"`
	Green         string `toml:"green"`
	Yellow        string `toml:"yellow"`
	Blue          string `toml:"blue"`
	Magenta       string `toml:"magenta"`
	Cyan          string `toml:"cyan"`
	White         string `toml:"white"`
	BlackBright   string `toml:"black_bright"`
	RedBright     string `toml:"red_bright"`
	GreenBright   string `toml:"green_bright"`
	YellowBright  string `toml:"yellow_bright"`
	BlueBright    string `toml:"blue_bright"`
	MagentaBright string `toml:"magenta_bright"`
	CyanBright    string `toml:"cyan_bright"`
	WhiteBright   string `toml:"white_bright"`
}

// PaletteColors is the role-based color set of a theme ([palette] table).
type PaletteColors struct {
	// Backgrounds.
	Bg          string `toml:"bg"`           // main background (required)
	BgDark      string `toml:"bg_dark"`      // darker background: statusline, sidebars (required)
	BgHighlight string `toml:"bg_highlight"` // highlighted background: cursor line (derivable)
	BgVisual    string `toml:"bg_visual"`    // selection background (derivable)

	// Foregrounds.
	Fg        string `toml:"fg"`         // main foreground (required)
	FgDark    string `toml:"fg_dark"`    // secondary foreground (derivable)
	FgGutter  string `toml:"fg_gutter"`  // gutter / line-number foreground (derivable)
	FgInverse string `toml:"fg_inverse"` // text on accent backgrounds (derivable: bg_dark)
	Comment   string `toml:"comment"`    // muted / comment text (required)
	Border    string `toml:"border"`     // panel and box borders (required)

	// Accents (all required).
	Red     string `toml:"red"`
	Green   string `toml:"green"`
	Yellow  string `toml:"yellow"`
	Blue    string `toml:"blue"`
	Magenta string `toml:"magenta"`
	Cyan    string `toml:"cyan"`
	Orange  string `toml:"orange"`
	Purple  string `toml:"purple"`

	Git         GitColors        `toml:"git"`
	Diagnostics DiagnosticColors `toml:"diagnostics"`
}

// Palette is the canonical rich theme definition loaded from a theme TOML
// file. The legacy Colors struct is derived from it.
type Palette struct {
	Meta     PaletteMeta    `toml:"meta"`
	Colors   PaletteColors  `toml:"palette"`
	Terminal TerminalColors `toml:"terminal"`
}

// parsePalette decodes a theme TOML document, validates required roles, and
// fills every derivable role so the returned palette is fully populated.
func parsePalette(data []byte) (*Palette, error) {
	var p Palette
	if err := toml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parsing TOML: %w", err)
	}
	if err := p.normalize(); err != nil {
		return nil, err
	}
	return &p, nil
}

// normalize validates metadata and required roles, then derives every
// optional role that was left empty.
func (p *Palette) normalize() error {
	var errs []error

	p.Meta.Name = normalizeThemeName(p.Meta.Name)
	p.Meta.Family = normalizeThemeName(p.Meta.Family)
	if p.Meta.Name == "" {
		errs = append(errs, errors.New("meta.name is required"))
	}
	if p.Meta.Family == "" {
		errs = append(errs, errors.New("meta.family is required"))
	}
	if p.Meta.Variant == "" {
		errs = append(errs, errors.New("meta.variant is required"))
	}
	if p.Meta.Appearance != "dark" && p.Meta.Appearance != "light" {
		errs = append(errs, fmt.Errorf("meta.appearance must be %q or %q, got %q", "dark", "light", p.Meta.Appearance))
	}

	c := &p.Colors
	required := []struct {
		role  string
		value string
	}{
		{"bg", c.Bg},
		{"bg_dark", c.BgDark},
		{"fg", c.Fg},
		{"comment", c.Comment},
		{"border", c.Border},
		{"red", c.Red},
		{"green", c.Green},
		{"yellow", c.Yellow},
		{"blue", c.Blue},
		{"magenta", c.Magenta},
		{"cyan", c.Cyan},
		{"orange", c.Orange},
		{"purple", c.Purple},
	}
	for _, r := range required {
		if r.value == "" {
			errs = append(errs, fmt.Errorf("palette.%s is required", r.role))
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	p.derive()

	for _, v := range p.colorValues() {
		if v.value == "" {
			continue
		}
		if err := p.checkValue(v.role, v.value); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// derive fills every optional role that is empty. Hex palettes use the
// floraverse blend helpers; ANSI palettes fall back to plain copies since
// index values cannot be blended.
func (p *Palette) derive() {
	c := &p.Colors
	if c.BgVisual == "" {
		if p.Meta.ANSI {
			c.BgVisual = c.Comment
		} else {
			c.BgVisual = Darken(c.Blue, 0.25, c.Bg)
		}
	}
	if c.BgHighlight == "" {
		if p.Meta.ANSI {
			c.BgHighlight = c.BgVisual
		} else {
			c.BgHighlight = Lighten(c.Bg, 0.93, c.Fg)
		}
	}
	if c.FgDark == "" {
		if p.Meta.ANSI {
			c.FgDark = c.Fg
		} else {
			c.FgDark = Darken(c.Fg, 0.85, c.Bg)
		}
	}
	if c.FgGutter == "" {
		if p.Meta.ANSI {
			c.FgGutter = c.Comment
		} else {
			c.FgGutter = Darken(c.Fg, 0.30, c.Bg)
		}
	}
	if c.FgInverse == "" {
		c.FgInverse = c.BgDark
	}

	if c.Git.Add == "" {
		c.Git.Add = c.Green
	}
	if c.Git.Change == "" {
		c.Git.Change = c.Blue
	}
	if c.Git.Delete == "" {
		c.Git.Delete = c.Red
	}

	if c.Diagnostics.Error == "" {
		c.Diagnostics.Error = c.Red
	}
	if c.Diagnostics.Warning == "" {
		c.Diagnostics.Warning = c.Yellow
	}
	if c.Diagnostics.Info == "" {
		c.Diagnostics.Info = c.Blue
	}
	if c.Diagnostics.Hint == "" {
		c.Diagnostics.Hint = c.Cyan
	}

	t := &p.Terminal
	normal := []struct {
		slot *string
		base string
	}{
		{&t.Black, c.BgDark},
		{&t.Red, c.Red},
		{&t.Green, c.Green},
		{&t.Yellow, c.Yellow},
		{&t.Blue, c.Blue},
		{&t.Magenta, c.Magenta},
		{&t.Cyan, c.Cyan},
		{&t.White, c.Fg},
	}
	for _, s := range normal {
		if *s.slot == "" {
			*s.slot = s.base
		}
	}
	if t.BlackBright == "" {
		t.BlackBright = c.Comment
	}
	bright := []struct {
		slot *string
		base string
	}{
		{&t.RedBright, t.Red},
		{&t.GreenBright, t.Green},
		{&t.YellowBright, t.Yellow},
		{&t.BlueBright, t.Blue},
		{&t.MagentaBright, t.Magenta},
		{&t.CyanBright, t.Cyan},
	}
	for _, s := range bright {
		if *s.slot == "" {
			if p.Meta.ANSI {
				*s.slot = s.base
			} else {
				*s.slot = Lighten(s.base, 0.8, c.Fg)
			}
		}
	}
	if t.WhiteBright == "" {
		t.WhiteBright = t.White
	}
}

// checkValue validates a single color value: "#rrggbb" hex for regular
// palettes, an ANSI index "0".."255" for ANSI palettes.
func (p *Palette) checkValue(role, value string) error {
	if p.Meta.ANSI {
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 || n > 255 {
			return fmt.Errorf("%s: %q is not a valid ANSI color index (0-255)", role, value)
		}
		return nil
	}
	if !isHexColor(value) {
		return fmt.Errorf("%s: %q is not a valid #rrggbb hex color", role, value)
	}
	return nil
}

// colorValues enumerates every color-bearing field with its role name for
// validation.
func (p *Palette) colorValues() []struct{ role, value string } {
	c := p.Colors
	t := p.Terminal
	return []struct{ role, value string }{
		{"bg", c.Bg},
		{"bg_dark", c.BgDark},
		{"bg_highlight", c.BgHighlight},
		{"bg_visual", c.BgVisual},
		{"fg", c.Fg},
		{"fg_dark", c.FgDark},
		{"fg_gutter", c.FgGutter},
		{"fg_inverse", c.FgInverse},
		{"comment", c.Comment},
		{"border", c.Border},
		{"red", c.Red},
		{"green", c.Green},
		{"yellow", c.Yellow},
		{"blue", c.Blue},
		{"magenta", c.Magenta},
		{"cyan", c.Cyan},
		{"orange", c.Orange},
		{"purple", c.Purple},
		{"git.add", c.Git.Add},
		{"git.change", c.Git.Change},
		{"git.delete", c.Git.Delete},
		{"diagnostics.error", c.Diagnostics.Error},
		{"diagnostics.warning", c.Diagnostics.Warning},
		{"diagnostics.info", c.Diagnostics.Info},
		{"diagnostics.hint", c.Diagnostics.Hint},
		{"terminal.black", t.Black},
		{"terminal.red", t.Red},
		{"terminal.green", t.Green},
		{"terminal.yellow", t.Yellow},
		{"terminal.blue", t.Blue},
		{"terminal.magenta", t.Magenta},
		{"terminal.cyan", t.Cyan},
		{"terminal.white", t.White},
		{"terminal.black_bright", t.BlackBright},
		{"terminal.red_bright", t.RedBright},
		{"terminal.green_bright", t.GreenBright},
		{"terminal.yellow_bright", t.YellowBright},
		{"terminal.blue_bright", t.BlueBright},
		{"terminal.magenta_bright", t.MagentaBright},
		{"terminal.cyan_bright", t.CyanBright},
		{"terminal.white_bright", t.WhiteBright},
	}
}

// legacyValues maps the rich palette onto the 15 legacy Colors slots as raw
// color strings.
type legacyValues struct {
	Green                string
	Yellow               string
	Red                  string
	Orange               string
	Cyan                 string
	Blue                 string
	Violet               string
	Pink                 string
	LightText            string
	MutedText            string
	DarkText             string
	Border               string
	SelectedBackground   string
	SubtleBackground     string
	VerySubtleBackground string
}

// legacyValues derives the legacy 15-slot color set from the rich palette.
func (p *Palette) legacyValues() legacyValues {
	c := p.Colors
	return legacyValues{
		Green:                c.Green,
		Yellow:               c.Yellow,
		Red:                  c.Red,
		Orange:               c.Orange,
		Cyan:                 c.Cyan,
		Blue:                 c.Blue,
		Violet:               c.Purple,
		Pink:                 c.Magenta,
		LightText:            c.Fg,
		MutedText:            c.Comment,
		DarkText:             c.FgInverse,
		Border:               c.Border,
		SelectedBackground:   c.BgVisual,
		SubtleBackground:     c.Bg,
		VerySubtleBackground: c.BgDark,
	}
}
