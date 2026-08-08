package plugin

import (
	"strings"
	"testing"

	"github.com/grovetools/core/pkg/exectrust"
)

// `[[panel.setting_options]]` is the AUTHOR's half of a settings vocabulary: the
// panel says which values one of its settings takes, and a config UI offers them
// instead of a text box. It grants nothing and forbids nothing — a value outside
// the list is still writable, and the host delivers the settings table
// unchanged either way.

// optionsManifest is the shape the HN panel's browser selector has: a closed
// list, one entry of which hands the decision to a free-text setting beside it.
const optionsManifest = `
schema_version = 1

[plugin]
name        = "hn"
description = "Hacker News"

[build]
binary = "bin/grove-panel-hn"

[panel]
protocol = "embed/v1"

[panel.settings]
open_url_command        = "system"
open_url_custom_command = ""
palette                 = "hn"

[[panel.setting_options]]
setting        = "open_url_command"
description    = "which browser opens a story"
options        = ["system", "firefox", "chrome", "custom"]
custom_option  = "custom"
custom_setting = "open_url_custom_command"

[[panel.setting_options]]
setting = "palette"
options = ["hn", "host", "auto"]
`

func TestManifestSettingOptionsParseInDeclarationOrder(t *testing.T) {
	m, err := ParseManifest([]byte(optionsManifest))
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if len(m.Panel.SettingOptions) != 2 {
		t.Fatalf("setting options = %+v, want two", m.Panel.SettingOptions)
	}
	if m.Panel.SettingOptions[0].Setting != "open_url_command" {
		t.Errorf("first declaration = %q, want the author's order", m.Panel.SettingOptions[0].Setting)
	}
	o := m.Panel.OptionsFor("open_url_command")
	if o == nil {
		t.Fatal("OptionsFor found nothing for a declared setting")
	}
	if strings.Join(o.Options, ",") != "system,firefox,chrome,custom" {
		t.Errorf("options = %v — the order is the order a UI offers them in", o.Options)
	}
	if o.CustomOption != "custom" || o.CustomSetting != "open_url_custom_command" {
		t.Errorf("custom pair = %+v", o)
	}
	if o.AllowCustom {
		t.Error("allow_custom read as set — this list hands over to a sibling setting instead")
	}
	if m.Panel.OptionsFor("page_size") != nil {
		t.Error("OptionsFor invented a declaration for an undeclared setting")
	}
	if len(m.Unknown) != 0 {
		t.Errorf("unexpected unknown keys: %v", m.Unknown)
	}
}

// A nested setting is the case the array form exists for: `timer.work_minutes`
// is a legal setting path and an illegal bare TOML key, so it is named in a
// field rather than in a table header.
func TestSettingOptionsNameNestedSettingsWithoutQuoting(t *testing.T) {
	const manifest = `
schema_version = 1

[plugin]
name        = "timer"
description = "A timer"

[build]
binary = "bin/timer"

[panel.settings]
timer.length = "25m"

[[panel.setting_options]]
setting = "timer.length"
options = ["15m", "25m", "50m"]
`
	m, err := ParseManifest([]byte(manifest))
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if o := m.Panel.OptionsFor("timer.length"); o == nil || len(o.Options) != 3 {
		t.Fatalf("options = %+v, want the three durations", o)
	}
}

// Declaring none is normal and stays working: it is every manifest written
// before the section existed, and most manifests written after it.
func TestManifestSettingOptionsMayBeAbsent(t *testing.T) {
	m, err := ParseManifest([]byte(validManifest))
	if err != nil {
		t.Fatalf("a manifest with no setting options must parse: %v", err)
	}
	if m.Panel.SettingOptions != nil {
		t.Errorf("setting options = %+v, want nil", m.Panel.SettingOptions)
	}
}

func TestManifestSettingOptionsRejectWhatCannotBeOffered(t *testing.T) {
	cases := []struct {
		name     string
		manifest string
		wantIn   string
	}{
		{
			"options for a setting the panel does not declare",
			strings.Replace(optionsManifest, `setting = "palette"`, `setting = "pallete"`, 1),
			"names no setting in [panel.settings]",
		},
		{
			"an entry with no options",
			strings.Replace(optionsManifest, `options = ["hn", "host", "auto"]`, "", 1),
			"options is required",
		},
		{
			"a default outside its own vocabulary",
			strings.Replace(optionsManifest, `palette                 = "hn"`, `palette                 = "solarized"`, 1),
			"is not one of the options",
		},
		{
			"the same setting declared twice",
			optionsManifest + "\n[[panel.setting_options]]\nsetting = \"palette\"\noptions = [\"hn\"]\n",
			"declared twice",
		},
		{
			"the same option listed twice",
			strings.Replace(optionsManifest, `options = ["hn", "host", "auto"]`, `options = ["hn", "hn"]`, 1),
			`lists "hn" twice`,
		},
		{
			"a custom option that is not one of the options",
			strings.Replace(optionsManifest, `custom_option  = "custom"`, `custom_option  = "other"`, 1),
			"is not one of the options",
		},
		{
			"a custom option with nothing to hand over to",
			strings.Replace(optionsManifest, `custom_setting = "open_url_custom_command"`, "", 1),
			"does not say which setting that choice hands over to",
		},
		{
			"a custom setting with no option that selects it",
			strings.Replace(optionsManifest, `custom_option  = "custom"`, "", 1),
			"does not say which choice selects it",
		},
		{
			"a custom setting the panel does not declare",
			strings.Replace(optionsManifest, `custom_setting = "open_url_custom_command"`, `custom_setting = "open_url_custom"`, 1),
			"names no setting in [panel.settings]",
		},
		{
			"a setting handing over to itself",
			strings.Replace(optionsManifest, `custom_setting = "open_url_custom_command"`, `custom_setting = "open_url_command"`, 1),
			"cannot hand over to itself",
		},
		{
			"a control character in an option",
			strings.Replace(optionsManifest, `"firefox"`, `"fire\u001b[2Jfox"`, 1),
			"control character",
		},
		{
			"an empty option",
			strings.Replace(optionsManifest, `"firefox"`, `""`, 1),
			"is empty",
		},
		{
			"an entry naming no setting",
			strings.Replace(optionsManifest, `setting = "palette"`, `setting = ""`, 1),
			"setting is required",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseManifest([]byte(tc.manifest))
			if err == nil {
				t.Fatalf("expected an error mentioning %q", tc.wantIn)
			}
			if !strings.Contains(err.Error(), tc.wantIn) {
				t.Errorf("error = %q, want it to mention %q", err, tc.wantIn)
			}
		})
	}
}

// allow_custom is the OTHER escape hatch, and it is what makes a default outside
// the list legal: the author is saying their seven browsers are the ones they
// tested, not the only ones that work.
func TestAllowCustomAdmitsADefaultOutsideTheList(t *testing.T) {
	manifest := strings.Replace(optionsManifest,
		`options = ["hn", "host", "auto"]`,
		`options      = ["hn", "host", "auto"]
allow_custom = true`, 1)
	manifest = strings.Replace(manifest, `palette                 = "hn"`, `palette                 = "solarized"`, 1)
	m, err := ParseManifest([]byte(manifest))
	if err != nil {
		t.Fatalf("allow_custom must admit a value outside the list: %v", err)
	}
	if o := m.Panel.OptionsFor("palette"); o == nil || !o.AllowCustom {
		t.Errorf("palette = %+v, want allow_custom", o)
	}
}

// The options are compared against the setting's value as TEXT, which is how
// every other comparison in this package reads a setting — so a numeric setting
// declares its options as strings and still validates.
func TestSettingOptionsMatchANumericDefaultAsText(t *testing.T) {
	const manifest = `
schema_version = 1

[plugin]
name        = "hn"
description = "Hacker News"

[build]
binary = "bin/grove-panel-hn"

[panel.settings]
page_size = 24

[[panel.setting_options]]
setting = "page_size"
options = ["12", "24", "48"]
`
	if _, err := ParseManifest([]byte(manifest)); err != nil {
		t.Fatalf("a numeric default must match its option written as text: %v", err)
	}
}

// An unrecognised key inside an entry is a warning like any other: a manifest
// written for a later schema must still install, and still say what it could
// not read.
func TestUnknownKeyInsideASettingOptionIsReportedNotFatal(t *testing.T) {
	m, err := ParseManifest([]byte(optionsManifest + "\nmultiple = true\n"))
	if err != nil {
		t.Fatalf("an unknown key inside an entry must not fail the parse: %v", err)
	}
	if !strings.Contains(strings.Join(m.Unknown, ","), "multiple") {
		t.Errorf("unknown = %v, want it to name the key", m.Unknown)
	}
	if m.Panel.OptionsFor("palette") == nil {
		t.Error("the entry it appeared in was dropped")
	}
}

// --- what the approval is bound to ---

func TestSettingOptionFactsAreTheLinesAnApprovalHashes(t *testing.T) {
	m, err := ParseManifest([]byte(optionsManifest))
	if err != nil {
		t.Fatal(err)
	}
	got := SettingOptionFacts(&m.Panel)
	want := []string{
		"open_url_command = system, firefox, chrome, custom (custom hands over to open_url_custom_command) — which browser opens a story",
		"palette = hn, host, auto",
	}
	if len(got) != len(want) {
		t.Fatalf("facts = %q", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("fact %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// A manifest that declares no options hashes exactly as it did before they
// existed, so every plugin already installed stays approved. Spelled out rather
// than compared against a sibling ConsentFacts, because the claim is about a
// specific historical string.
func TestDigestIsUnchangedForAManifestWithoutSettingOptions(t *testing.T) {
	facts := ConsentFacts{
		Name: "timer", Source: "src", Commit: "abc",
		ManifestDigest: "sha256:dead", Build: []string{"go", "build"},
		Run: []string{"/opt/grove/bin/timer"}, Protocol: "embed/v1",
		Label: "Timer", Settings: []string{"work_minutes = 25"},
	}
	want := exectrust.Digest([]string{
		"source=src",
		"commit=abc",
		"manifest=sha256:dead",
		"build=go\x1fbuild",
		"run=/opt/grove/bin/timer",
		"env=",
		"protocol=embed/v1",
		"keys=",
		"label=Timer",
		"settings=work_minutes = 25",
	})
	if got := facts.Digest(); got != want {
		t.Errorf("digest = %q, want %q — an approval recorded before setting options existed would read as edited", got, want)
	}
	// And declaring one moves the digest, which is the whole point of recording
	// it: an update that starts offering `custom` starts offering to point the
	// panel at an executable.
	declared := facts
	declared.SettingOptions = []string{"palette = hn, host, auto"}
	if declared.Digest() == want {
		t.Error("a new vocabulary hashed the same as none at all")
	}
}

// An update that retunes a vocabulary names the setting whose choices moved,
// beside the row that would name a changed default.
func TestDiffNamesTheSettingWhoseOptionsMoved(t *testing.T) {
	old := ConsentFacts{SettingOptions: []string{"palette = hn, host", "page_size = 12, 24"}}
	next := ConsentFacts{SettingOptions: []string{"palette = hn, host, auto", "page_size = 12, 24"}}
	changes := Diff(old, next)
	if len(changes) != 1 {
		t.Fatalf("changes = %+v, want only the setting that moved", changes)
	}
	if changes[0].Field != "options.palette" {
		t.Errorf("field = %q, want it keyed on the setting", changes[0].Field)
	}
	if changes[0].Old != "hn, host" || changes[0].New != "hn, host, auto" {
		t.Errorf("change = %+v, want both vocabularies", changes[0])
	}
}
