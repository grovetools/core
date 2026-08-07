package plugin

import (
	"strings"
	"testing"

	"github.com/grovetools/core/pkg/exectrust"
)

// `[panel.digest]` is the panel's disclosure that it publishes a digest — the
// projection a host draws in a slot the panel cannot run in. It grants nothing
// and obliges nothing: a host draws the LIVE frame and reads this nowhere. So
// what these tests pin is the disclosure — that it parses, that an empty one is
// refused, that the approval digest binds it, and above all that a manifest
// WITHOUT it hashes exactly as it always did.

// digestManifest is the documented shape: a panel with something worth showing
// from somewhere else, saying what that something is.
const digestManifest = `
schema_version = 1

[plugin]
name        = "breaktimer"
description = "A work/break interval timer"

[build]
command = ["go", "build", "-o", "bin/grove-panel-breaktimer", "."]
binary  = "bin/grove-panel-breaktimer"

[panel]
protocol = "embed/v1"

[panel.digest]
description = "the timer's state and how long is left of it"
`

func TestManifestDigestParses(t *testing.T) {
	m, err := ParseManifest([]byte(digestManifest))
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	d := m.Panel.Digest
	if d == nil {
		t.Fatal("[panel.digest] did not parse")
	}
	if d.Description != "the timer's state and how long is left of it" {
		t.Errorf("digest = %+v", d)
	}
	// The section is a known key now, not a forward-compat warning.
	if len(m.Unknown) != 0 {
		t.Errorf("unexpected unknown keys: %v", m.Unknown)
	}
}

// A manifest that declares no digest stays valid and carries none. Absence is
// the ordinary case — every manifest written before this table existed is one —
// which is why the declaration is only ever read in the affirmative.
func TestManifestWithoutDigestParsesToNil(t *testing.T) {
	m, err := ParseManifest([]byte(validManifest))
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if m.Panel.Digest != nil {
		t.Errorf("digest = %+v, want nil for a manifest that declares none", m.Panel.Digest)
	}
}

func TestManifestDigestRejects(t *testing.T) {
	cases := []struct {
		name     string
		manifest string
		wantIn   string
	}{
		{
			// The table with nothing in it is the case worth refusing: it claims
			// a surface without saying what appears there.
			"empty table",
			strings.Replace(digestManifest, `description = "the timer's state and how long is left of it"`, "", 1),
			"panel.digest.description is required",
		},
		{
			"blank description",
			strings.Replace(digestManifest, `"the timer's state and how long is left of it"`, `"   "`, 1),
			"panel.digest.description is required",
		},
		{
			"control character in the description",
			strings.Replace(digestManifest, `"the timer's state and how long is left of it"`, `"state\u001b[2J"`, 1),
			"control character",
		},
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

// THE critical property: a manifest that declares no digest hashes exactly as it
// did before the section existed, so every plugin already installed stays
// approved. The parts list is spelled out rather than compared against a sibling
// ConsentFacts, because the claim is about a specific historical string and not
// about two values differing. Views and a notebook are present deliberately: the
// digest line must add nothing even when every other conditional line fires.
func TestDigestIsUnchangedForAManifestWithoutADigestDeclaration(t *testing.T) {
	facts := ConsentFacts{
		Name: "timer", Source: "src", Commit: "abc",
		ManifestDigest: "sha256:dead", Build: []string{"go", "build"},
		Run: []string{"/opt/grove/bin/timer"}, Protocol: "embed/v1",
		Keys: []string{"ctrl+f — start a break"}, Label: "Timer",
		Settings:            []string{"work_minutes = 25"},
		Views:               []string{"compact — one line (what a drawer pane gets by default)"},
		NotebookSubtree:     "timer/sessions",
		NotebookDescription: "one line per completed session",
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
		"notebook=timer/sessions — one line per completed session",
	})
	if got := facts.Digest(); got != want {
		t.Errorf("digest = %q, want %q — an approval recorded before [panel.digest] existed would read as edited", got, want)
	}

	// And the converse: the same facts WITH a declaration hash differently,
	// which is what re-opens the prompt for a panel that starts projecting
	// itself into surfaces the user was not shown it in.
	declared := facts
	declared.DigestDescription = "the timer's state and how long is left of it"
	if declared.Digest() == want {
		t.Error("declaring a digest did not change the approval digest")
	}
}

// An update that starts, stops or reworks the projection says so by name. The
// row is one line rather than a bare "digest changed" for the reason every other
// declaration's is: the user is deciding about the sentence, so the sentence has
// to be on the screen.
func TestDiffReportsTheDigestDeclaration(t *testing.T) {
	old := ConsentFacts{Source: "src", Commit: "abc"}
	next := old
	next.DigestDescription = "the timer's state and how long is left of it"

	changes := Diff(old, next)
	var found *FactChange
	for i := range changes {
		if changes[i].Field == "digest" {
			found = &changes[i]
		}
	}
	if found == nil {
		t.Fatalf("a panel that started publishing a digest produced no digest row: %+v", changes)
	}
	if found.Old != "" || found.New != next.DigestDescription {
		t.Errorf("digest row = %+v", *found)
	}
	// Unchanged declarations produce no row, which is what keeps an update diff
	// to what actually moved.
	if len(Diff(next, next)) != 0 {
		t.Errorf("identical facts produced a diff: %+v", Diff(next, next))
	}
}
