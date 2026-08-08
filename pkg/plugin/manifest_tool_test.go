package plugin

import (
	"strings"
	"testing"

	"github.com/grovetools/core/pkg/exectrust"
)

// `[tool]` is the manifest's other kind: a binary grove dispatches to rather
// than a pane treemux runs. The kind is inferred from section presence — no
// kind field, no schema bump — so what these tests pin is the inference: that a
// tool manifest parses as one, that declaring both kinds is refused, that a
// manifest declaring neither stays the plain PTY panel it has always been, and
// above all that a panel's approval digest hashes exactly as it always did.

// toolManifest is the documented shape: the extracted forge recipe.
const toolManifest = `
schema_version = 1

[plugin]
name        = "forge"
description = "Provision cloud machines from a recipe"

[build]
command = ["go", "build", "-o", "bin/grove-tool-forge", "."]
binary  = "bin/grove-tool-forge"

[tool]
binary   = "forge"
provides = ["forge up", "forge status"]
`

func TestToolManifestParses(t *testing.T) {
	m, err := ParseManifest([]byte(toolManifest))
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if m.Kind() != "tool" {
		t.Errorf("Kind = %q, want tool", m.Kind())
	}
	if m.Tool == nil {
		t.Fatal("[tool] did not parse")
	}
	if got := strings.Join(m.Tool.Provides, ","); got != "forge up,forge status" {
		t.Errorf("provides = %q", got)
	}
	// The section is a known key, not a forward-compat warning.
	if len(m.Unknown) != 0 {
		t.Errorf("unexpected unknown keys: %v", m.Unknown)
	}
	// tool.binary overrides the installed name; without it the name falls
	// back to the basename of what the build produces, as it does for panels.
	if m.BinaryName() != "forge" {
		t.Errorf("BinaryName = %q, want the tool.binary override", m.BinaryName())
	}
	bare, err := ParseManifest([]byte(strings.Replace(toolManifest, `binary   = "forge"`, "", 1)))
	if err != nil {
		t.Fatalf("ParseManifest without tool.binary: %v", err)
	}
	if bare.BinaryName() != "grove-tool-forge" {
		t.Errorf("BinaryName = %q, want the build.binary basename", bare.BinaryName())
	}
}

func TestToolManifestRejects(t *testing.T) {
	cases := []struct {
		name     string
		manifest string
		wantIn   string
	}{
		{
			"no provides",
			strings.Replace(toolManifest, `provides = ["forge up", "forge status"]`, "", 1),
			"tool.provides is required",
		},
		{
			"empty provides",
			strings.Replace(toolManifest, `["forge up", "forge status"]`, `[]`, 1),
			"tool.provides is required",
		},
		{
			"binary with a path separator",
			strings.Replace(toolManifest, `binary   = "forge"`, `binary   = "bin/forge"`, 1),
			"bare command name",
		},
		{
			"uppercase binary",
			strings.Replace(toolManifest, `binary   = "forge"`, `binary   = "Forge"`, 1),
			"bare command name",
		},
		{
			"flag-shaped phrase",
			strings.Replace(toolManifest, `"forge up"`, `"-f up"`, 1),
			"flag-shaped token",
		},
		{
			"flag-shaped later token",
			strings.Replace(toolManifest, `"forge up"`, `"forge --verbose"`, 1),
			"flag-shaped token",
		},
		{
			"blank phrase",
			strings.Replace(toolManifest, `"forge up"`, `"   "`, 1),
			"is empty",
		},
		{
			"whitespace-padded phrase",
			strings.Replace(toolManifest, `"forge up"`, `"forge up "`, 1),
			"must not begin or end with whitespace",
		},
		{
			"verb that is not a bare name",
			strings.Replace(toolManifest, `"forge up"`, `"Forge up"`, 1),
			"bare verb",
		},
		{
			"control character in a phrase",
			strings.Replace(toolManifest, `"forge up"`, `"forge\u001b[2Jup"`, 1),
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

// Declaring both kinds is refused in every spelling TOML allows for the panel
// section — a mutual-exclusion check that only saw `[panel]` headers would
// accept the same conflict written another way.
func TestToolAlongsidePanelIsRefused(t *testing.T) {
	for name, manifest := range map[string]string{
		"table header":  toolManifest + "\n[panel]\nprotocol = \"embed/v1\"\n",
		"subtable only": toolManifest + "\n[panel.settings]\nregion = \"us-east1\"\n",
		"array header":  toolManifest + "\n[[panel.keys]]\nkey = \"ctrl+f\"\ndescription = \"x\"\n",
		// A dotted key is only top-level before the first table header, which
		// is the one place TOML lets it spell the [panel] section.
		"dotted key": "panel.protocol = \"embed/v1\"\n" + toolManifest,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseManifest([]byte(manifest))
			if err == nil {
				t.Fatal("a manifest declaring both [tool] and [panel] was accepted")
			}
			if !strings.Contains(err.Error(), "[tool]") || !strings.Contains(err.Error(), "[panel]") {
				t.Errorf("error = %q, want it to name both sections", err)
			}
		})
	}
}

// A manifest without [tool] is a panel — including one with no [panel] section
// at all, which has always meant a plain PTY panel and must go on meaning it.
func TestManifestWithoutToolIsAPanel(t *testing.T) {
	m, err := ParseManifest([]byte(validManifest))
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if m.Kind() != "panel" || m.Tool != nil {
		t.Errorf("Kind = %q, Tool = %+v, want a panel with no tool", m.Kind(), m.Tool)
	}

	plain, err := ParseManifest([]byte(`
schema_version = 1

[plugin]
name        = "plain"
description = "A plain PTY panel with no [panel] section"

[build]
binary = "bin/plain"
`))
	if err != nil {
		t.Fatalf("a manifest with neither section must stay valid: %v", err)
	}
	if plain.Kind() != "panel" {
		t.Errorf("Kind = %q, want panel for a manifest declaring neither section", plain.Kind())
	}
}

// THE critical property: a panel's facts hash exactly as they did before tools
// existed, so every plugin already installed stays approved. The parts list is
// spelled out rather than compared against a sibling ConsentFacts, because the
// claim is about a specific historical string — no kind part, no provides part.
func TestDigestIsUnchangedForAPanelWithoutAKind(t *testing.T) {
	facts := ConsentFacts{
		Name: "timer", Source: "src", Commit: "abc",
		ManifestDigest: "sha256:dead", Build: []string{"go", "build"},
		Run: []string{"/opt/grove/bin/timer"}, Protocol: "embed/v1",
		Keys: []string{"ctrl+f — start a break"}, Label: "Timer",
		Settings: []string{"work_minutes = 25"},
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
	})
	if got := facts.Digest(); got != want {
		t.Errorf("digest = %q, want %q — an approval recorded before [tool] existed would read as edited", got, want)
	}

	// And the converse: the same facts as a tool hash differently, which is
	// what re-opens the prompt when a plugin changes what sort of thing it is
	// or what commands it answers.
	tool := facts
	tool.Kind = "tool"
	tool.Provides = []string{"grove forge up"}
	if tool.Digest() == want {
		t.Error("becoming a tool did not change the approval digest")
	}
	verbs := tool
	verbs.Provides = []string{"grove forge up", "grove forge status"}
	if verbs.Digest() == tool.Digest() {
		t.Error("a grown provides list did not change the approval digest")
	}
}

// An update that changes what commands the tool answers says so, in one row —
// the list is the tool's whole claim on the command line.
func TestDiffReportsKindAndProvides(t *testing.T) {
	old := ConsentFacts{
		Source: "src", Commit: "abc", Kind: "tool",
		Provides: []string{"grove forge up"},
	}
	next := old
	next.Provides = []string{"grove forge up", "grove forge status"}

	changes := Diff(old, next)
	if len(changes) != 1 || changes[0].Field != "provides" {
		t.Fatalf("changes = %+v, want exactly one provides row", changes)
	}
	if changes[0].Old != "grove forge up" || changes[0].New != "grove forge up, grove forge status" {
		t.Errorf("provides row = %+v", changes[0])
	}

	rekinded := old
	rekinded.Kind = ""
	found := false
	for _, c := range Diff(old, rekinded) {
		if c.Field == "kind" && c.Old == "tool" && c.New == "" {
			found = true
		}
	}
	if !found {
		t.Error("a changed kind produced no kind row")
	}
}

// ToolFacts renders the lines an approval is hashed over, so the exact wording
// is a recorded fact rather than presentation: each phrase is spelled as the
// command it enables.
func TestToolFactsRenderTheApprovedLines(t *testing.T) {
	m, err := ParseManifest([]byte(toolManifest))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	facts := ToolFacts(m.Tool)
	want := []string{"grove forge up", "grove forge status"}
	if len(facts) != len(want) {
		t.Fatalf("tool facts = %v, want %v", facts, want)
	}
	for i := range want {
		if facts[i] != want[i] {
			t.Errorf("tool fact[%d] = %q, want %q", i, facts[i], want[i])
		}
	}
}
