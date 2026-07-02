package daemon

import "encoding/json"

// UpdateTypeThemeChanged is the SSE update_type broadcast by the daemon when
// the resolved global tui.theme value changes. The event payload is a
// ThemeChangedPayload carrying fully resolved role colors so consumers
// (grove TUIs, grove.nvim) never re-parse layered config.
const UpdateTypeThemeChanged = "theme_changed"

// ThemeGitColors carries the git-status accent colors of a theme palette.
type ThemeGitColors struct {
	Add    string `json:"add"`
	Change string `json:"change"`
	Delete string `json:"delete"`
}

// ThemeDiagnosticColors carries the diagnostic severity colors of a theme
// palette.
type ThemeDiagnosticColors struct {
	Error   string `json:"error"`
	Warning string `json:"warning"`
	Info    string `json:"info"`
	Hint    string `json:"hint"`
}

// ThemeTerminalColors carries the 16 terminal slot colors of a theme palette.
type ThemeTerminalColors struct {
	Black         string `json:"black"`
	Red           string `json:"red"`
	Green         string `json:"green"`
	Yellow        string `json:"yellow"`
	Blue          string `json:"blue"`
	Magenta       string `json:"magenta"`
	Cyan          string `json:"cyan"`
	White         string `json:"white"`
	BlackBright   string `json:"black_bright"`
	RedBright     string `json:"red_bright"`
	GreenBright   string `json:"green_bright"`
	YellowBright  string `json:"yellow_bright"`
	BlueBright    string `json:"blue_bright"`
	MagentaBright string `json:"magenta_bright"`
	CyanBright    string `json:"cyan_bright"`
	WhiteBright   string `json:"white_bright"`
}

// ThemePalette carries the fully derived role colors of one concrete theme
// variant. Values are "#rrggbb" hex colors, except for ANSI palettes
// (ThemeChangedPayload.Mode == "ansi") where every value is a terminal ANSI
// color index string ("0".."255") that consumers should pass through (or
// cterm-map) instead of treating as hex.
type ThemePalette struct {
	// Name is the concrete registry key of this variant (e.g.
	// "kanagawa-dark") — always a specific variant, never a family name.
	Name       string `json:"name"`
	Variant    string `json:"variant"`
	Appearance string `json:"appearance"` // "dark" | "light"

	// Backgrounds.
	Bg          string `json:"bg"`
	BgDark      string `json:"bg_dark"`
	BgHighlight string `json:"bg_highlight"`
	BgVisual    string `json:"bg_visual"`

	// Foregrounds.
	Fg        string `json:"fg"`
	FgDark    string `json:"fg_dark"`
	FgGutter  string `json:"fg_gutter"`
	FgInverse string `json:"fg_inverse"`
	Comment   string `json:"comment"`
	Border    string `json:"border"`

	// Accents.
	Red     string `json:"red"`
	Green   string `json:"green"`
	Yellow  string `json:"yellow"`
	Blue    string `json:"blue"`
	Magenta string `json:"magenta"`
	Cyan    string `json:"cyan"`
	Orange  string `json:"orange"`
	Purple  string `json:"purple"`

	Git         ThemeGitColors        `json:"git"`
	Diagnostics ThemeDiagnosticColors `json:"diagnostics"`
	Terminal    ThemeTerminalColors   `json:"terminal"`
}

// ThemeChangedPayload is the wire payload of a theme_changed SSE event (and
// of the Theme field on the initial snapshot). It carries the selected theme
// name plus the resolved palettes for both appearances of the theme's family
// so consumers can restyle for either terminal background without re-parsing
// layered config.
type ThemeChangedPayload struct {
	// Name is the normalized theme selection from config (family name,
	// variant name, or legacy alias — e.g. "kanagawa", "catppuccin-frappe",
	// "gruvbox-dark"). It is what consumers should pass to theme.SetTheme,
	// which resolves aliases and family names internally. Consumers
	// comparing names should prefer Family plus the concrete Dark/Light
	// palette names, since distinct Name values can resolve to the same
	// effective theme.
	Name string `json:"name"`
	// Family is the resolved theme family (e.g. "kanagawa").
	Family string `json:"family"`
	// Mode is "hex" for regular palettes or "ansi" for the terminal
	// passthrough theme, whose palette values are ANSI index strings.
	Mode string `json:"mode"`
	// Dark and Light are the resolved palettes per appearance. When the
	// selection is a specific variant, that variant occupies its own
	// appearance slot and the family's default fills the other; a slot is
	// nil when the family has no palette for that appearance.
	Dark  *ThemePalette `json:"dark,omitempty"`
	Light *ThemePalette `json:"light,omitempty"`
}

// ParseThemeChanged extracts a theme payload from a StateUpdate. It handles
// both the dedicated theme_changed event (generic Payload decode) and the
// initial snapshot's typed Theme field, so SSE consumers — including
// bespoke streams like treemux's — can share one decode path.
func ParseThemeChanged(update StateUpdate) (*ThemeChangedPayload, bool) {
	switch update.UpdateType {
	case UpdateTypeThemeChanged:
		if p, ok := update.Payload.(*ThemeChangedPayload); ok {
			if p == nil || p.Name == "" {
				return nil, false
			}
			return p, true
		}
		data, err := json.Marshal(update.Payload)
		if err != nil {
			return nil, false
		}
		var p ThemeChangedPayload
		if err := json.Unmarshal(data, &p); err != nil || p.Name == "" {
			return nil, false
		}
		return &p, true
	case "initial":
		if update.Theme != nil && update.Theme.Name != "" {
			return update.Theme, true
		}
	}
	return nil, false
}
