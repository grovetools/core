package theme

import (
	"strings"
	"testing"
)

func TestIconLookup(t *testing.T) {
	want := IconGitBranch
	if want == "" {
		t.Fatal("IconGitBranch is empty; the icon set did not load")
	}

	// Every spelling a manifest, a config file or a wire frame might carry.
	for _, name := range []string{
		"git-branch", "git_branch", "gitBranch", "GitBranch",
		"gitbranch", "IconGitBranch", "  git-branch  ",
	} {
		got, ok := Icon(name)
		if !ok {
			t.Errorf("Icon(%q) not found", name)
			continue
		}
		if got != want {
			t.Errorf("Icon(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestIconUnknownName(t *testing.T) {
	if _, ok := Icon("no-such-icon"); ok {
		t.Error("Icon reported a hit for a name that does not exist")
	}
	if got := IconOr("no-such-icon", "•"); got != "•" {
		t.Errorf("IconOr fallback = %q, want the bullet", got)
	}
	if got := IconOr("git-branch", "•"); got != IconGitBranch {
		t.Errorf("IconOr with a real name = %q, want the icon", got)
	}
}

// The table holds pointers, so a lookup follows a runtime icon-mode switch.
// A table of values would keep serving Nerd Font glyphs to a terminal that
// had already dropped to ASCII.
func TestIconFollowsTheIconMode(t *testing.T) {
	restoreSelection(t)

	SetIcons("ascii")
	ascii, ok := Icon("git-branch")
	if !ok {
		t.Fatal("Icon(git-branch) not found in ascii mode")
	}
	if ascii != IconGitBranch {
		t.Errorf("Icon(git-branch) = %q but IconGitBranch = %q", ascii, IconGitBranch)
	}

	SetIcons("nerd")
	nerd, _ := Icon("git-branch")
	if nerd != IconGitBranch {
		t.Errorf("Icon(git-branch) = %q but IconGitBranch = %q", nerd, IconGitBranch)
	}
	if ascii == nerd {
		t.Errorf("the ascii and nerd forms of git-branch are both %q; "+
			"the lookup is not tracking the mode", ascii)
	}
}

func TestIconNamesAreCanonicalAndResolvable(t *testing.T) {
	names := IconNames()
	if len(names) < 100 {
		t.Fatalf("IconNames() returned %d names; the table looks truncated", len(names))
	}
	for _, name := range names {
		if name != strings.ToLower(name) {
			t.Errorf("name %q is not lowercase; canonical names are the wire form", name)
		}
		if strings.ContainsAny(name, " _") {
			t.Errorf("name %q uses a separator other than a hyphen", name)
		}
		if _, ok := Icon(name); !ok {
			t.Errorf("IconNames() reported %q but Icon() cannot resolve it", name)
		}
	}
}

func TestIconNamesIsACopy(t *testing.T) {
	names := IconNames()
	if len(names) == 0 {
		t.Fatal("no icon names")
	}
	names[0] = "clobbered"
	if IconNames()[0] == "clobbered" {
		t.Error("IconNames() handed out the backing array")
	}
}

// ResolveIconOr is the name-OR-literal form: a registered name follows the
// table, a literal glyph passes through, and an unregistered NAME degrades to
// the fallback rather than rendering the word itself as if it were an icon.
func TestResolveIconOr(t *testing.T) {
	restoreSelection(t)
	SetIcons("nerd")

	if got := ResolveIconOr("git-branch", "•"); got != IconGitBranch {
		t.Errorf("registered name = %q, want the table's glyph %q", got, IconGitBranch)
	}
	if got := ResolveIconOr("", "•"); got != "•" {
		t.Errorf("empty ref = %q, want fallback", got)
	}
	// A third-party plugin's own codepoint — fa-yc here — is not in the
	// table and must pass through verbatim.
	if got := ResolveIconOr("", "•"); got != "" {
		t.Errorf("literal glyph = %q, want it verbatim", got)
	}
	if got := ResolveIconOr("H", "•"); got != "H" {
		t.Errorf("short ascii literal = %q, want it verbatim", got)
	}
	if got := ResolveIconOr("no-such-icon", "•"); got != "•" {
		t.Errorf("unknown name = %q, want fallback — never the name as text", got)
	}
}

// In ASCII mode a literal glyph has no ASCII form to degrade to, so the
// fallback wins; a registered name still follows the table's ASCII column.
func TestResolveIconOrASCIIMode(t *testing.T) {
	restoreSelection(t)
	SetIcons("ascii")

	if got := ResolveIconOr("", "*"); got != "*" {
		t.Errorf("ascii mode literal glyph = %q, want fallback", got)
	}
	if got := ResolveIconOr("git-branch", "*"); got != IconGitBranch {
		t.Errorf("ascii mode registered name = %q, want the table's ascii form", got)
	}
	if got := ResolveIconOr("H", "*"); got != "H" {
		t.Errorf("ascii mode ascii literal = %q, want it verbatim", got)
	}
}
