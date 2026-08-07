package plugin

import (
	"strings"
	"testing"

	"github.com/grovetools/core/pkg/exectrust"
)

// `[panel.notebook]` is the panel's disclosure of the notebook subtree it
// writes. It grants nothing and forbids nothing — the host never resolves,
// creates or fences the path — so what these tests pin is the disclosure
// itself: that it parses, that a half-declared one is refused, that the
// approval digest binds it, and above all that a manifest WITHOUT it hashes
// exactly as it always did.

// notebookManifest is the documented shape: a panel that clips stories into
// the user's notebook and says where.
const notebookManifest = `
schema_version = 1

[plugin]
name        = "hn"
description = "A Hacker News reader"

[build]
command = ["go", "build", "-o", "bin/grove-panel-hn", "."]
binary  = "bin/grove-panel-hn"

[panel]
protocol = "embed/v1"

[panel.notebook]
subtree     = "hn/clippings"
description = "stories you clip from the feed"
`

func TestManifestNotebookParses(t *testing.T) {
	m, err := ParseManifest([]byte(notebookManifest))
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	nb := m.Panel.Notebook
	if nb == nil {
		t.Fatal("[panel.notebook] did not parse")
	}
	if nb.Subtree != "hn/clippings" || nb.Description != "stories you clip from the feed" {
		t.Errorf("notebook = %+v", nb)
	}
	// The section is a known key now, not a forward-compat warning.
	if len(m.Unknown) != 0 {
		t.Errorf("unexpected unknown keys: %v", m.Unknown)
	}
}

// A manifest that declares no notebook stays valid and carries none: most
// panels save nothing, and absence must remain the ordinary case.
func TestManifestWithoutNotebookParsesToNil(t *testing.T) {
	m, err := ParseManifest([]byte(validManifest))
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if m.Panel.Notebook != nil {
		t.Errorf("notebook = %+v, want nil for a manifest that declares none", m.Panel.Notebook)
	}
}

func TestManifestNotebookRejects(t *testing.T) {
	cases := []struct {
		name     string
		manifest string
		wantIn   string
	}{
		{"missing subtree", strings.Replace(notebookManifest, `subtree     = "hn/clippings"`, "", 1), "panel.notebook.subtree is required"},
		{"missing description", strings.Replace(notebookManifest, `description = "stories you clip from the feed"`, "", 1), "panel.notebook.description is required"},
		{"escaping subtree", strings.Replace(notebookManifest, `"hn/clippings"`, `"../outside"`, 1), "must stay inside"},
		{"absolute subtree", strings.Replace(notebookManifest, `"hn/clippings"`, `"/etc/hn"`, 1), "must be relative"},
		{"trailing slash", strings.Replace(notebookManifest, `"hn/clippings"`, `"hn/clippings/"`, 1), "must not begin or end with a slash"},
		{"overlong subtree", strings.Replace(notebookManifest, `"hn/clippings"`, `"hn/`+strings.Repeat("x", 130)+`"`, 1), "longer than a path"},
		{"control character in the description", strings.Replace(notebookManifest, `"stories you clip from the feed"`, `"stories\u001b[2J"`, 1), "control character"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseManifest([]byte(tc.manifest))
			if err == nil {
				t.Fatalf("expected a validation error mentioning %q", tc.wantIn)
			}
			if !strings.Contains(err.Error(), tc.wantIn) {
				t.Errorf("error = %q, want it to mention %q", err, tc.wantIn)
			}
		})
	}
}

// THE critical property: a manifest that declares no notebook hashes exactly
// as it did before the section existed, so every plugin already installed
// stays approved. The parts list is spelled out rather than compared against
// a sibling ConsentFacts, because the claim is about a specific historical
// string and not about two values differing. Views are present deliberately:
// the notebook line must add nothing even when other conditional lines fire.
func TestDigestIsUnchangedForAManifestWithoutNotebook(t *testing.T) {
	facts := ConsentFacts{
		Name: "timer", Source: "src", Commit: "abc",
		ManifestDigest: "sha256:dead", Build: []string{"go", "build"},
		Run: []string{"/opt/grove/bin/timer"}, Protocol: "embed/v1",
		Keys: []string{"ctrl+f — start a break"}, Label: "Timer",
		Settings: []string{"work_minutes = 25"},
		Views:    []string{"compact — one line (what a drawer pane gets by default)"},
	}
	want := exectrust.Digest([]string{
		"source=src",
		"commit=abc",
		"manifest=sha256:dead",
		"build=go\x1fbuild",
		"run=/opt/grove/bin/timer",
		"env=",
		"protocol=embed/v1",
		"keys=ctrl+f — start a break",
		"label=Timer",
		"settings=work_minutes = 25",
		"views=compact — one line (what a drawer pane gets by default)",
	})
	if got := facts.Digest(); got != want {
		t.Errorf("digest = %q, want %q — an approval recorded before [panel.notebook] existed would read as edited", got, want)
	}

	// And the converse: the same facts WITH a declaration hash differently,
	// which is what re-opens the prompt for a panel that starts saving.
	declared := facts
	declared.NotebookSubtree = "hn/clippings"
	declared.NotebookDescription = "stories you clip from the feed"
	if declared.Digest() == want {
		t.Error("declaring a notebook subtree did not change the approval digest")
	}
}
