package config

import (
	"path/filepath"
	"testing"
)

// The recorded share bit has to reach the COMPILED view, because that is the
// view the daemon's containment rule reads. Until it did, `share = true` was
// parsed, validated and then dropped on the floor one layer short of the only
// consumer that exists for it.
func TestCompileCodeRootsProjectsRecordedShare(t *testing.T) {
	shared := filepath.Join(t.TempDir(), "shared")
	unshared := filepath.Join(t.TempDir(), "unshared")
	never := filepath.Join(t.TempDir(), "never")
	rp, np := writeRecordedPair(t, "", `
default = "shared"

[notebooks.shared]
root = "`+shared+`"
[notebooks.shared.sync]
share = true

[notebooks.unshared]
root = "`+unshared+`"
[notebooks.unshared.sync]
share = false

[notebooks.never]
root = "`+never+`"
`)
	got, err := compileCodeRootsFromPaths(&Config{}, rp, np)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]bool{"shared": true, "unshared": false, "never": false} {
		definition := got.Notebooks.Definitions[name]
		if definition == nil {
			t.Fatalf("notebook %q is missing from the compiled view", name)
		}
		if definition.Shared != want {
			t.Fatalf("notebook %q compiled Shared = %t, want %t", name, definition.Shared, want)
		}
	}
}

// A legacy same-name definition carries no share state, so compilation must
// assign the recorded answer rather than merge into whatever the legacy layer
// happened to hold. Nothing in the config layers can spell `Shared`, and this
// is the test that keeps it that way.
func TestCompiledShareIsNotInheritedFromALegacyDefinition(t *testing.T) {
	root := filepath.Join(t.TempDir(), "nb")
	rp, np := writeRecordedPair(t, "", `
default = "nb"
[notebooks.nb]
root = "`+root+`"
`)
	legacy := &Config{Notebooks: &NotebooksConfig{Definitions: map[string]*Notebook{
		"nb": {RootDir: "/old", Shared: true, NotesPathTemplate: "notes"},
	}}}
	got, err := compileCodeRootsFromPaths(legacy, rp, np)
	if err != nil {
		t.Fatal(err)
	}
	definition := got.Notebooks.Definitions["nb"]
	if definition.Shared {
		t.Fatal("an unrecorded notebook compiled as shared; the recorded pair is the only authority on share")
	}
	if definition.NotesPathTemplate != "notes" {
		t.Fatalf("compilation dropped the legacy behavior fields it is supposed to preserve: %+v", definition)
	}
}
