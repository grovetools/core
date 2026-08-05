package theme

//go:generate go run ../../tools/icongen

import "strings"

// Icon returns the icon registered under a name, and whether there is one.
//
// This is what lets an icon be named as data. A Go caller writes
// theme.IconGitBranch and gets a compile-time check; a plugin manifest, a
// protocol frame, a grove.toml entry or a generated table has only a string,
// and until this existed the answer was to hardcode a glyph — which is how a
// panel ends up drawing a Nerd Font character into a terminal that has none.
//
// The value follows the current icon mode. Calling this after SetIcons("ascii")
// returns the ASCII form, because the table holds pointers to the same
// variables the exported constants do.
//
// Lookup is forgiving about spelling: "git-branch", "git_branch", "gitBranch",
// "GitBranch" and "IconGitBranch" all find the same icon. A wire format should
// still use the canonical hyphenated form that IconNames reports.
func Icon(name string) (string, bool) {
	p, ok := iconTable[normalizeIconName(name)]
	if !ok {
		return "", false
	}
	return *p, true
}

// IconOr returns the icon registered under a name, or fallback when there is
// none. It is the form a renderer wants: an unknown name from a manifest
// should degrade to a bullet, not to an empty cell that silently shifts every
// column after it.
func IconOr(name, fallback string) string {
	if icon, ok := Icon(name); ok {
		return icon
	}
	return fallback
}

// IconNames returns every registered icon name in canonical hyphenated form,
// in declaration order. Use it to validate a manifest, or to offer completion.
func IconNames() []string {
	return append([]string(nil), iconNames...)
}

// ResolveIconOr resolves an icon REFERENCE — a registered name, or a literal
// glyph — to the string a renderer should draw, falling back when neither
// reading applies.
//
// This exists for the surfaces whose icon arrives as data from OUTSIDE this
// module: a plugin manifest, a [tui.plugins] fragment, a digest frame. A
// registry name ("rss") gets the mode-following table entry, same as IconOr —
// but a third-party plugin is not limited to what this package happened to
// compile in: any string carrying a non-ASCII rune is taken as a literal
// glyph (a Nerd Font codepoint, an emoji) and returned verbatim, and a short
// all-ASCII string ("H", ">>") is a literal too. What does NOT pass through
// is a longer all-ASCII string that resolves to nothing — that is a name this
// registry has never heard of ("rss" against an old build, a typo), and
// rendering the word itself as if it were an icon is the failure this
// function replaces.
//
// In ASCII icon mode a literal non-ASCII glyph returns fallback: the author's
// glyph cannot degrade the way a registered name can (the table carries an
// ASCII form per name; a raw codepoint carries nothing), and tofu is worse
// than a generic mark.
func ResolveIconOr(ref, fallback string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return fallback
	}
	if icon, ok := Icon(ref); ok {
		return icon
	}
	for _, r := range ref {
		if r > 0x7F {
			if ASCIIIcons {
				return fallback
			}
			return ref
		}
	}
	if len(ref) <= 2 {
		return ref
	}
	return fallback
}

// normalizeIconName reduces a name to lowercase alphanumerics, which is the
// form the generated table is keyed on with hyphens re-inserted. Dropping the
// separators entirely on both sides is what makes every spelling of a name
// converge without the table carrying an entry per variant.
func normalizeIconName(name string) string {
	name = strings.TrimPrefix(strings.TrimSpace(name), "Icon")
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r - 'A' + 'a')
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		}
	}
	return b.String()
}
