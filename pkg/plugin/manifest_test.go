package plugin

import (
	"strings"
	"testing"
)

// validManifest is the smallest manifest that passes, and the base every
// negative case below mutates one field of.
const validManifest = `
schema_version = 1

[plugin]
name        = "hello"
description = "A hello-world sidecar panel"

[build]
command = ["go", "build", "-o", "bin/grove-panel-hello", "."]
binary  = "bin/grove-panel-hello"

[panel]
icon     = "H"
protocol = "embed/v1"

[[panel.keys]]
key         = "ctrl+f"
description = "jump to the notebook"
`

func TestParseManifestAcceptsTheDocumentedShape(t *testing.T) {
	m, err := ParseManifest([]byte(validManifest))
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if m.Plugin.Name != "hello" {
		t.Errorf("name = %q", m.Plugin.Name)
	}
	if got, want := strings.Join(m.Build.Command, " "), "go build -o bin/grove-panel-hello ."; got != want {
		t.Errorf("build command = %q, want %q", got, want)
	}
	if m.BinaryName() != "grove-panel-hello" {
		t.Errorf("BinaryName = %q", m.BinaryName())
	}
	if len(m.Panel.Keys) != 1 || m.Panel.Keys[0].Key != "ctrl+f" {
		t.Errorf("keys = %+v", m.Panel.Keys)
	}
	if len(m.Unknown) != 0 {
		t.Errorf("unexpected unknown keys: %v", m.Unknown)
	}
}

func TestParseManifestRejects(t *testing.T) {
	cases := []struct {
		name     string
		manifest string
		wantIn   string
	}{
		{"no schema version", strings.Replace(validManifest, "schema_version = 1", "", 1), "schema_version is required"},
		{"future schema version", strings.Replace(validManifest, "schema_version = 1", "schema_version = 2", 1), "newer than this grove"},
		{"no name", strings.Replace(validManifest, `name        = "hello"`, "", 1), "plugin.name is required"},
		{"name with a slash", strings.Replace(validManifest, `"hello"`, `"../../etc/hello"`, 1), "must be lowercase letters"},
		{"uppercase name", strings.Replace(validManifest, `"hello"`, `"Hello"`, 1), "must be lowercase letters"},
		{"no description", strings.Replace(validManifest, `description = "A hello-world sidecar panel"`, "", 1), "plugin.description is required"},
		{"no binary", strings.Replace(validManifest, `binary  = "bin/grove-panel-hello"`, "", 1), "build.binary is required"},
		{"absolute binary", strings.Replace(validManifest, `"bin/grove-panel-hello"
`, `"/usr/bin/env"
`, 1), "must be relative"},
		{"escaping binary", strings.Replace(validManifest, `binary  = "bin/grove-panel-hello"`, `binary  = "../../../bin/sh"`, 1), "must stay inside"},
		{"unknown protocol", strings.Replace(validManifest, `"embed/v1"`, `"embed/v9"`, 1), "not a protocol this host speaks"},
		{"bad timeout", strings.Replace(validManifest, `protocol = "embed/v1"`, `protocol_timeout = "soon"`, 1), "not a Go duration"},
		{"bad env", strings.Replace(validManifest, `icon     = "H"`, `env = ["not-an-assignment"]`, 1), "must be KEY=VALUE"},
		{"empty argv element", strings.Replace(validManifest, `"go", "build"`, `"go", ""`, 1), "build.command[1] is empty"},
		{"key without a description", strings.Replace(validManifest, `description = "jump to the notebook"`, "", 1), "description is required"},
		{"control character in the icon", strings.Replace(validManifest, `icon     = "H"`, `icon = "\u001b[2J"`, 1), "control character"},
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

// A manifest written for a later grove must still install, with its
// unrecognized keys reported rather than swallowed or fatal. That is what
// keeps one plugin repo installable by two grove versions.
func TestParseManifestReportsUnknownKeysWithoutFailing(t *testing.T) {
	m, err := ParseManifest([]byte(validManifest + "\n[panel.sandbox]\nnetwork = false\n"))
	if err != nil {
		t.Fatalf("an unknown key must not fail the parse: %v", err)
	}
	if len(m.Unknown) == 0 {
		t.Fatal("expected the unknown key to be reported")
	}
	if !strings.Contains(strings.Join(m.Unknown, ","), "sandbox") {
		t.Errorf("unknown = %v, want it to name panel.sandbox", m.Unknown)
	}
}

// An interpreted panel ships its program in the repo and needs no toolchain.
func TestParseManifestAllowsNoBuildStep(t *testing.T) {
	manifest := strings.Replace(validManifest, `command = ["go", "build", "-o", "bin/grove-panel-hello", "."]`, "", 1)
	m, err := ParseManifest([]byte(manifest))
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if len(m.Build.Command) != 0 {
		t.Errorf("build.command = %v, want empty", m.Build.Command)
	}
}

// A panel's settings are the only thing on the consent screen the user is
// expected to go on to edit, so they have to survive the manifest, the digest
// and the fragment intact.
func TestSettingsAndLabelRoundTripThroughTheManifest(t *testing.T) {
	m, err := ParseManifest([]byte(`
schema_version = 1

[plugin]
name        = "timer"
description = "A break timer"

[build]
command = ["go", "build", "-o", "bin/timer", "."]
binary  = "bin/timer"

[panel]
label    = "Break timer"
protocol = "embed/v1"

[panel.settings]
work_minutes  = 25
break_minutes = 5
chime         = "bell"

[panel.settings.notify]
desktop = true
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if m.Panel.Label != "Break timer" {
		t.Errorf("label = %q", m.Panel.Label)
	}

	flat := FlattenSettings(m.Panel.Settings)
	want := []string{
		"break_minutes = 5",
		"chime = bell",
		"notify.desktop = true",
		"work_minutes = 25",
	}
	if len(flat) != len(want) {
		t.Fatalf("flattened settings = %v, want %v", flat, want)
	}
	for i := range want {
		if flat[i] != want[i] {
			t.Errorf("flattened[%d] = %q, want %q", i, flat[i], want[i])
		}
	}
}

// The flattening feeds a digest, so it must not depend on map iteration order:
// an unchanged manifest that re-hashed on every run would re-open the consent
// prompt forever.
func TestFlattenSettingsIsStable(t *testing.T) {
	settings := map[string]any{
		"z": 1, "a": 2, "m": 3, "nested": map[string]any{"q": 4, "b": 5},
	}
	first := strings.Join(FlattenSettings(settings), "|")
	for i := 0; i < 50; i++ {
		if got := strings.Join(FlattenSettings(settings), "|"); got != first {
			t.Fatalf("flattening is order-dependent:\n %s\n %s", first, got)
		}
	}
}

// Everything in a settings table is printed on a screen the user's approval
// depends on, so a value that could redraw that screen is refused rather than
// displayed — including one buried in a nested table or an array.
func TestSettingsRejectControlCharactersAtAnyDepth(t *testing.T) {
	for name, settings := range map[string]map[string]any{
		"top-level value": {"greeting": "hi\x1b[2Jbye"},
		"nested value":    {"a": map[string]any{"b": "x\rrewritten"}},
		"array element":   {"list": []any{"fine", "bad\x1b[A"}},
		"key":             {"we\x1bird": "fine"},
	} {
		if err := validateSettings("panel.settings", settings); err == nil {
			t.Errorf("%s: a control character was accepted", name)
		}
	}

	ok := map[string]any{
		"work_minutes": int64(25),
		"chime":        "bell",
		"tags":         []any{"a", "b"},
		"notify":       map[string]any{"desktop": true},
	}
	if err := validateSettings("panel.settings", ok); err != nil {
		t.Errorf("an ordinary settings table was refused: %v", err)
	}
}
