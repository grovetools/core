package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolateGlobalConfig points the XDG lookups at an empty temp dir so the
// developer's real ~/.config/grove cannot leak into a load-path assertion.
func isolateGlobalConfig(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

const fixtureCardTOML = `name = "grovetools"
workspaces = ["*"]

[ecosystem]
id = "01J8ZZZZZZZZZZZZZZZZZZZZZZ"
layout = "superrepo"

[[ecosystem.remotes]]
name = "origin"
url = "https://github.com/grovetools/grovetools.git"

[ecosystem.notebooks.personal]
default = true

[ecosystem.notebooks.org]
audience = "org"
`

const fixtureCardYAML = `name: grovetools
workspaces:
  - "*"
ecosystem:
  id: "01J8ZZZZZZZZZZZZZZZZZZZZZZ"
  layout: superrepo
  remotes:
    - name: origin
      url: "https://github.com/grovetools/grovetools.git"
  notebooks:
    personal:
      default: true
    org:
      audience: org
`

// assertFixtureCard checks the shape both fixtures encode.
func assertFixtureCard(t *testing.T, card *EcosystemCard) {
	t.Helper()
	if card == nil {
		t.Fatal("expected an ecosystem card, got nil")
	}
	if card.ID != "01J8ZZZZZZZZZZZZZZZZZZZZZZ" {
		t.Errorf("id = %q, want the fixture ULID", card.ID)
	}
	if card.Layout != LayoutSuperrepo {
		t.Errorf("layout = %q, want %q", card.Layout, LayoutSuperrepo)
	}
	if len(card.Remotes) != 1 {
		t.Fatalf("remotes = %d, want 1", len(card.Remotes))
	}
	if card.Remotes[0].Name != "origin" || card.Remotes[0].URL != "https://github.com/grovetools/grovetools.git" {
		t.Errorf("remote = %+v, want origin/https://github.com/grovetools/grovetools.git", card.Remotes[0])
	}
	if len(card.Notebooks) != 2 {
		t.Fatalf("notebooks = %d, want 2", len(card.Notebooks))
	}
	if !card.Notebooks["personal"].Default {
		t.Error("personal notebook should be the default")
	}
	if card.Notebooks["org"].Audience != "org" {
		t.Errorf("org audience = %q, want %q", card.Notebooks["org"].Audience, "org")
	}
	if got := card.DefaultNotebookName(); got != "personal" {
		t.Errorf("DefaultNotebookName() = %q, want %q", got, "personal")
	}
}

// TestEcosystemCardRoundTripsThroughLoadLayeredTOML is the acceptance case: a
// grove.toml carrying a card loads as a typed card, and nothing about it lands
// in Extensions (which is what "ecosystem is a core key" means in practice).
func TestEcosystemCardRoundTripsThroughLoadLayeredTOML(t *testing.T) {
	isolateGlobalConfig(t)
	dir := t.TempDir()
	writeFixture(t, filepath.Join(dir, "grove.toml"), fixtureCardTOML)

	layered, err := LoadLayered(dir)
	if err != nil {
		t.Fatalf("LoadLayered: %v", err)
	}
	assertFixtureCard(t, layered.Final.Ecosystem)
	assertNoEcosystemExtension(t, layered.Final)
	assertNoEcosystemExtension(t, layered.Project)
}

// TestEcosystemCardRoundTripsThroughLoadLayeredYAML is the same assertion on
// the YAML path, where the inline Extensions map in Config.UnmarshalYAML would
// swallow the card if rawConfig did not name it.
func TestEcosystemCardRoundTripsThroughLoadLayeredYAML(t *testing.T) {
	isolateGlobalConfig(t)
	dir := t.TempDir()
	writeFixture(t, filepath.Join(dir, "grove.yml"), fixtureCardYAML)

	layered, err := LoadLayered(dir)
	if err != nil {
		t.Fatalf("LoadLayered: %v", err)
	}
	assertFixtureCard(t, layered.Final.Ecosystem)
	assertNoEcosystemExtension(t, layered.Final)
	assertNoEcosystemExtension(t, layered.Project)
}

// TestEcosystemCardUnmarshalsFromBytes covers the two byte-level entry points
// directly, so a regression in either dialect is attributable without a
// filesystem in the way.
func TestEcosystemCardUnmarshalsFromBytes(t *testing.T) {
	tomlCfg, err := LoadFromTOMLBytes([]byte(fixtureCardTOML))
	if err != nil {
		t.Fatalf("LoadFromTOMLBytes: %v", err)
	}
	assertFixtureCard(t, tomlCfg.Ecosystem)
	assertNoEcosystemExtension(t, tomlCfg)

	yamlCfg, err := LoadFromBytes([]byte(fixtureCardYAML))
	if err != nil {
		t.Fatalf("LoadFromBytes: %v", err)
	}
	assertFixtureCard(t, yamlCfg.Ecosystem)
	assertNoEcosystemExtension(t, yamlCfg)
}

// TestWriteEcosystemCardPreservesEverythingElse: the writer is surgical. Every
// byte outside the [ecosystem] table — comments, key order, tables the struct
// does not model — comes back unchanged.
func TestWriteEcosystemCardPreservesEverythingElse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "grove.toml")
	original := `# hand-authored, do not mangle
name = "grovetools"
workspaces = ["*"]

[tui]
theme = "solarized"   # trailing comment

[custom.extension]
anything = "goes"
`
	writeFixture(t, path, original)

	changed, err := WriteEcosystemCard(path, EcosystemCard{
		ID:      "01JCARD0000000000000000000",
		Layout:  LayoutFlat,
		Remotes: []EcosystemRemote{{Name: "origin", URL: "git@github.com:me/eco.git"}},
	})
	if err != nil {
		t.Fatalf("WriteEcosystemCard: %v", err)
	}
	if !changed {
		t.Fatal("expected the first write to change the file")
	}

	got := readFixture(t, path)
	for _, line := range strings.Split(strings.TrimRight(original, "\n"), "\n") {
		if !strings.Contains(got, line) {
			t.Errorf("original line %q was not preserved; file is:\n%s", line, got)
		}
	}
	if !strings.HasPrefix(got, "# hand-authored, do not mangle\nname = \"grovetools\"\n") {
		t.Errorf("the head of the file was rewritten; file is:\n%s", got)
	}

	card, err := LoadEcosystemCard(path)
	if err != nil {
		t.Fatalf("LoadEcosystemCard: %v", err)
	}
	if card == nil || card.ID != "01JCARD0000000000000000000" || card.Layout != LayoutFlat {
		t.Fatalf("card did not round-trip: %+v", card)
	}
}

// TestWriteEcosystemCardIsIdempotent is the "re-running adopt is a no-op"
// acceptance criterion, enforced at the writer rather than at the caller.
func TestWriteEcosystemCardIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "grove.toml")
	writeFixture(t, path, "name = \"eco\"\nworkspaces = [\"*\"]\n")

	card := EcosystemCard{
		ID:        "01JCARD0000000000000000000",
		Layout:    LayoutSuperrepo,
		Remotes:   []EcosystemRemote{{Name: "origin", URL: "https://example.test/eco.git"}},
		Notebooks: map[string]EcosystemNotebook{"nb": {Default: true}},
	}
	if changed, err := WriteEcosystemCard(path, card); err != nil || !changed {
		t.Fatalf("first write: changed=%v err=%v", changed, err)
	}
	first := readFixture(t, path)

	changed, err := WriteEcosystemCard(path, card)
	if err != nil {
		t.Fatalf("second write: %v", err)
	}
	if changed {
		t.Error("re-writing an identical card should report no change")
	}
	if got := readFixture(t, path); got != first {
		t.Errorf("re-write mutated the file:\n--- first ---\n%s\n--- second ---\n%s", first, got)
	}
}

// TestWriteEcosystemCardReplacesTheTableInPlace: an update must rewrite the
// card region, never append a second one.
func TestWriteEcosystemCardReplacesTheTableInPlace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "grove.toml")
	writeFixture(t, path, fixtureCardTOML+"\n[tui]\ntheme = \"dark\"\n")

	if _, err := WriteEcosystemCard(path, EcosystemCard{
		ID:      "01J8ZZZZZZZZZZZZZZZZZZZZZZ",
		Layout:  LayoutFlat,
		Remotes: []EcosystemRemote{{Name: "upstream", URL: "https://example.test/up.git"}},
	}); err != nil {
		t.Fatalf("WriteEcosystemCard: %v", err)
	}

	got := readFixture(t, path)
	if n := strings.Count(got, "[ecosystem]"); n != 1 {
		t.Errorf("expected exactly one [ecosystem] table, got %d:\n%s", n, got)
	}
	if strings.Contains(got, "notebooks.personal") {
		t.Errorf("the stale notebook subtable survived the replacement:\n%s", got)
	}
	if !strings.Contains(got, "[tui]\ntheme = \"dark\"") {
		t.Errorf("the table after the card was damaged:\n%s", got)
	}

	card, err := LoadEcosystemCard(path)
	if err != nil {
		t.Fatalf("LoadEcosystemCard: %v", err)
	}
	if card.Layout != LayoutFlat || len(card.Remotes) != 1 || card.Remotes[0].Name != "upstream" {
		t.Fatalf("card was not replaced: %+v", card)
	}
}

// TestWriteEcosystemCardNeverRewritesTheID is the plan's hard guard: the id is
// minted once. The writer keeps an existing id when the caller supplies none,
// and refuses outright when the caller supplies a different one.
func TestWriteEcosystemCardNeverRewritesTheID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "grove.toml")
	writeFixture(t, path, fixtureCardTOML)

	if _, err := WriteEcosystemCard(path, EcosystemCard{Layout: LayoutFlat}); err != nil {
		t.Fatalf("WriteEcosystemCard with no id: %v", err)
	}
	card, err := LoadEcosystemCard(path)
	if err != nil {
		t.Fatalf("LoadEcosystemCard: %v", err)
	}
	if card.ID != "01J8ZZZZZZZZZZZZZZZZZZZZZZ" {
		t.Errorf("id = %q, want the original ULID to survive", card.ID)
	}

	_, err = WriteEcosystemCard(path, EcosystemCard{ID: "01JOTHER000000000000000000"})
	if err == nil {
		t.Fatal("expected an error when a different id is written over an existing one")
	}
	if !strings.Contains(err.Error(), "minted once") {
		t.Errorf("error = %v, want it to name the mint-once rule", err)
	}
	if card, _ := LoadEcosystemCard(path); card.ID != "01J8ZZZZZZZZZZZZZZZZZZZZZZ" {
		t.Errorf("the rejected write still mutated the id: %q", card.ID)
	}
}

// TestWriteEcosystemCardYAML: ecosystems scaffolded before `grove ecosystem
// init` switched to TOML carry a grove.yml, so adopt has to be able to
// backfill a card into a YAML manifest with the same surgical guarantee.
func TestWriteEcosystemCardYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "grove.yml")
	original := "# yaml ecosystem\nname: eco\nworkspaces:\n  - \"*\"\n"
	writeFixture(t, path, original)

	card := EcosystemCard{
		ID:        "01JCARD0000000000000000000",
		Layout:    LayoutSuperrepo,
		Remotes:   []EcosystemRemote{{Name: "origin", URL: "https://example.test/eco.git"}},
		Notebooks: map[string]EcosystemNotebook{"nb": {Default: true}, "plain": {}},
	}
	if changed, err := WriteEcosystemCard(path, card); err != nil || !changed {
		t.Fatalf("first write: changed=%v err=%v", changed, err)
	}
	got := readFixture(t, path)
	if !strings.HasPrefix(got, original) {
		t.Errorf("the original document was not preserved verbatim:\n%s", got)
	}

	loaded, err := LoadEcosystemCard(path)
	if err != nil {
		t.Fatalf("LoadEcosystemCard: %v", err)
	}
	if loaded == nil || loaded.ID != card.ID || loaded.Layout != LayoutSuperrepo {
		t.Fatalf("card did not round-trip: %+v", loaded)
	}
	if len(loaded.Remotes) != 1 || loaded.Remotes[0].Name != "origin" {
		t.Fatalf("remotes did not round-trip: %+v", loaded.Remotes)
	}
	if !loaded.Notebooks["nb"].Default {
		t.Errorf("default notebook did not round-trip: %+v", loaded.Notebooks)
	}
	if _, ok := loaded.Notebooks["plain"]; !ok {
		t.Errorf("empty notebook binding was dropped: %+v", loaded.Notebooks)
	}

	// And the YAML path is idempotent too.
	if changed, err := WriteEcosystemCard(path, card); err != nil || changed {
		t.Fatalf("second write: changed=%v err=%v", changed, err)
	}
}

// TestWriteEcosystemCardCreatesMissingManifest covers the init case where the
// manifest does not exist yet.
func TestWriteEcosystemCardCreatesMissingManifest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "grove.toml")

	if changed, err := WriteEcosystemCard(path, EcosystemCard{ID: "01JNEW0000000000000000000X", Layout: LayoutFlat}); err != nil || !changed {
		t.Fatalf("write: changed=%v err=%v", changed, err)
	}
	card, err := LoadEcosystemCard(path)
	if err != nil {
		t.Fatalf("LoadEcosystemCard: %v", err)
	}
	if card == nil || card.Layout != LayoutFlat {
		t.Fatalf("card = %+v", card)
	}
}

func TestEcosystemCardValidate(t *testing.T) {
	cases := []struct {
		name    string
		card    EcosystemCard
		wantErr string
	}{
		{name: "empty is valid", card: EcosystemCard{}},
		{name: "bad layout", card: EcosystemCard{Layout: "monorepo"}, wantErr: "layout"},
		{name: "empty remote name", card: EcosystemCard{Remotes: []EcosystemRemote{{URL: "u"}}}, wantErr: "empty name"},
		{name: "empty remote url", card: EcosystemCard{Remotes: []EcosystemRemote{{Name: "origin"}}}, wantErr: "empty url"},
		{
			name:    "duplicate remote",
			card:    EcosystemCard{Remotes: []EcosystemRemote{{Name: "origin", URL: "a"}, {Name: "origin", URL: "b"}}},
			wantErr: "declared twice",
		},
		{
			name:    "two defaults",
			card:    EcosystemCard{Notebooks: map[string]EcosystemNotebook{"a": {Default: true}, "b": {Default: true}}},
			wantErr: "default notebooks",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.card.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate() = %v, want an error containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestFindEcosystemManifest(t *testing.T) {
	dir := t.TempDir()
	if got := FindEcosystemManifest(dir); got != "" {
		t.Errorf("FindEcosystemManifest on an empty dir = %q, want \"\"", got)
	}
	writeFixture(t, filepath.Join(dir, "grove.yml"), "name: eco\n")
	if got := FindEcosystemManifest(dir); got != filepath.Join(dir, "grove.yml") {
		t.Errorf("FindEcosystemManifest = %q, want the grove.yml", got)
	}
	writeFixture(t, filepath.Join(dir, "grove.toml"), "name = \"eco\"\n")
	if got := FindEcosystemManifest(dir); got != filepath.Join(dir, "grove.toml") {
		t.Errorf("FindEcosystemManifest = %q, want grove.toml to win", got)
	}
}

func assertNoEcosystemExtension(t *testing.T, cfg *Config) {
	t.Helper()
	if cfg == nil {
		return
	}
	if _, ok := cfg.Extensions["ecosystem"]; ok {
		t.Errorf("the ecosystem card leaked into Extensions: %#v", cfg.Extensions["ecosystem"])
	}
}

func writeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readFixture(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
