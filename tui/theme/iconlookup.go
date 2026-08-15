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

// UnknownIconRef reports whether a reference is neither reading ResolveIconOr
// accepts: not a registered name, and not short enough or non-ASCII enough to
// be read as a literal glyph. It is the ONE case where falling back to the
// generic mark means the author wrote something that means nothing — "papers"
// against a registry that has no such icon, or a typo — as opposed to a literal
// this build simply cannot draw.
//
// That distinction is the whole reason it is a separate function. ResolveIconOr
// answers "what do I draw", and it answers "generic mark" for an unknown name
// AND for an author's own Nerd Font glyph under ASCII icon mode; only the first
// is worth telling anyone about, and a caller that tried to detect it by
// comparing against the fallback would report the second every time a user
// switched icon sets. Callers here are diagnostics — the plugin Warnings page,
// a manifest linter — never the render path.
//
// It is mode-independent for the same reason: whether "rss" is a name this
// build carries has nothing to do with whether the terminal can draw glyphs.
func UnknownIconRef(ref string) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		// Naming no icon is not naming a wrong one. A surface that wants an
		// icon declared is asking a different question than this one.
		return false
	}
	if _, ok := Icon(ref); ok {
		return false
	}
	for _, r := range ref {
		if r > 0x7F {
			return false
		}
	}
	// Same two-cell cutoff ResolveIconOr spends on a short ASCII mark ("H",
	// ">>"); iconlookup_test pins the two against each other so the pair cannot
	// drift into a warning about a reference that renders fine.
	return len(ref) > 2
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
