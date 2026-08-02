package registry

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/grovetools/core/config"
)

func machineWithCard(id, name, eco, ecoPath string, card *NoteCard) Machine {
	return Machine{
		PathID: id,
		Path:   NotePath(id),
		Note: &Note{
			MachineID:  id,
			Name:       name,
			Ecosystems: []NoteEcosystem{{Name: eco, Path: ecoPath, Enabled: true, State: StatePresent, Card: card}},
		},
	}
}

func grovetoolsCard() *NoteCard {
	return &NoteCard{
		ID:      "01ECOSYSTEMAAAAAAAAAAAAAAA",
		Layout:  config.LayoutSuperrepo,
		Remotes: []NoteRemote{{Name: "origin", URL: "https://example.com/grovetools.git"}},
	}
}

// TestOffersDedupAcrossMachines: every machine publishes its own copy of the
// same card, which is exactly why there is no shared ecosystems/<name>.md note.
// Readers collapse the copies here.
func TestOffersDedupAcrossMachines(t *testing.T) {
	offers := Offers([]Machine{
		machineWithCard("01AAAAAAAAAAAAAAAAAAAAAAAA", "mbp", "grovetools", "/Users/a/code/grovetools", grovetoolsCard()),
		machineWithCard("01BBBBBBBBBBBBBBBBBBBBBBBB", "solm4", "grovetools", "/Users/b/src/grovetools", grovetoolsCard()),
	})
	if len(offers) != 1 {
		t.Fatalf("want one deduped offer, got %d: %+v", len(offers), offers)
	}
	got := offers[0]
	if got.Name != "grovetools" || got.Card.ID != "01ECOSYSTEMAAAAAAAAAAAAAAA" {
		t.Errorf("unexpected offer: %+v", got)
	}
	if len(got.Publishers) != 2 {
		t.Errorf("want both publishers, got %v", got.Publishers)
	}
	if len(got.Paths) != 2 {
		t.Errorf("want both advisory paths, got %v", got.Paths)
	}
	if got.Conflicting {
		t.Errorf("identical cards were reported as conflicting")
	}
	if r, ok := got.PrimaryRemote(); !ok || r.URL != "https://example.com/grovetools.git" {
		t.Errorf("primary remote = %+v (%t)", r, ok)
	}
}

// TestOffersFlagDisagreement: two machines advertising different remotes for
// one name is the case a user must be shown rather than have resolved for them.
func TestOffersFlagDisagreement(t *testing.T) {
	other := grovetoolsCard()
	other.Remotes = []NoteRemote{{Name: "origin", URL: "https://evil.example/grovetools.git"}}

	offers := Offers([]Machine{
		machineWithCard("01AAAAAAAAAAAAAAAAAAAAAAAA", "mbp", "grovetools", "/a", grovetoolsCard()),
		machineWithCard("01BBBBBBBBBBBBBBBBBBBBBBBB", "solm4", "grovetools", "/b", other),
	})
	if len(offers) != 1 {
		t.Fatalf("want one offer, got %d", len(offers))
	}
	if !offers[0].Conflicting {
		t.Errorf("disagreeing remotes were not flagged")
	}
	// The FIRST card read wins, and the caller is told there is a dispute —
	// silently preferring either one would be picking a clone source for the
	// user out of a document any token can write.
	if r, _ := offers[0].PrimaryRemote(); r.URL != "https://example.com/grovetools.git" {
		t.Errorf("offer did not keep the first card: %+v", r)
	}
}

// TestOffersIgnoreUnparsedNotes: a note that failed to parse contributes
// nothing, but must not take the rest of the listing down with it.
func TestOffersIgnoreUnparsedNotes(t *testing.T) {
	offers := Offers([]Machine{
		{PathID: "01CCCCCCCCCCCCCCCCCCCCCCCC", Err: os.ErrInvalid},
		machineWithCard("01AAAAAAAAAAAAAAAAAAAAAAAA", "mbp", "grovetools", "/a", grovetoolsCard()),
	})
	if len(offers) != 1 || offers[0].Name != "grovetools" {
		t.Fatalf("unexpected offers: %+v", offers)
	}
}

// TestOffersSkipCardlessEcosystems: a bare root or a not-yet-adopted ecosystem
// appears in a note without a card and is not materializable.
func TestOffersSkipCardlessEcosystems(t *testing.T) {
	if offers := Offers([]Machine{machineWithCard("01AAAAAAAAAAAAAAAAAAAAAAAA", "mbp", "chickens", "/a", nil)}); len(offers) != 0 {
		t.Fatalf("a cardless ecosystem was offered: %+v", offers)
	}
}

func TestEcosystemCardRoundTripsThroughTheNote(t *testing.T) {
	source := config.EcosystemCard{
		ID:      "01ECOSYSTEMAAAAAAAAAAAAAAA",
		Layout:  config.LayoutFlat,
		Remotes: []config.EcosystemRemote{{Name: "origin", URL: "https://example.com/a.git"}, {Name: "backup", URL: "https://mirror.example/a.git"}},
		Notebooks: map[string]config.EcosystemNotebook{
			"personal": {Default: true},
			"org":      {Audience: "org"},
		},
	}
	// Build reads the card off the ecosystem's own manifest, which is where it
	// is canonical — the note only ever carries a copy.
	ecoDir := t.TempDir()
	manifest := filepath.Join(ecoDir, "grove.toml")
	if err := os.WriteFile(manifest, []byte("name = \"a\"\nworkspaces = [\"*\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := config.WriteEcosystemCard(manifest, source); err != nil {
		t.Fatal(err)
	}

	note := Build(BuildInput{
		MachineID: "01AAAAAAAAAAAAAAAAAAAAAAAA",
		Machine: &config.MachineConfig{Machine: config.MachineSettings{
			Ecosystems: map[string]config.MachineEcosystem{"a": {Path: ecoDir}},
		}},
	})
	if len(note.Ecosystems) != 1 || note.Ecosystems[0].Card == nil {
		t.Fatalf("card did not reach the note: %+v", note.Ecosystems)
	}
	back := note.Ecosystems[0].Card.EcosystemCard()
	if back.ID != source.ID || back.Layout != source.Layout {
		t.Errorf("identity lost: %+v", back)
	}
	if len(back.Remotes) != 2 || back.Remotes[0] != source.Remotes[0] {
		t.Errorf("remotes lost: %+v", back.Remotes)
	}
	if !back.Notebooks["personal"].Default || back.Notebooks["org"].Audience != "org" {
		t.Errorf("notebook bindings lost: %+v", back.Notebooks)
	}
}

// TestPlannedRootNeverUsesTheBuiltinDefault is the guard behind `grove join`'s
// directory creation: with notebooks declared, the planned root must be under
// one of them — never under the locator's `~/.grove/notebooks/nb` fallback,
// which nothing on a configured machine reads.
func TestPlannedRootNeverUsesTheBuiltinDefault(t *testing.T) {
	nbRoot := filepath.Join(t.TempDir(), "notebooks", "nb")
	cfg := &config.Config{Notebooks: &config.NotebooksConfig{
		Definitions: map[string]*config.Notebook{"nb": {RootDir: nbRoot}},
	}}

	root := PlannedRoot(cfg, "registry")
	want := filepath.Join(nbRoot, "workspaces", "registry")
	if root != want {
		t.Fatalf("PlannedRoot = %q, want %q", root, want)
	}

	// Once created, WorkspaceRoot — the read surface and the daemon's rule —
	// must agree with where PlannedRoot put it. That agreement is the whole
	// point: join creates it, the daemon finds it.
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := WorkspaceRoot(cfg, "registry"); got != want {
		t.Errorf("WorkspaceRoot = %q, want %q — the creator and the reader disagree", got, want)
	}
}

// TestPlannedRootPrefersTheConfiguredDefaultNotebook.
func TestPlannedRootPrefersTheConfiguredDefaultNotebook(t *testing.T) {
	home := t.TempDir()
	cfg := &config.Config{Notebooks: &config.NotebooksConfig{
		Definitions: map[string]*config.Notebook{
			"archive":  {RootDir: filepath.Join(home, "archive")},
			"personal": {RootDir: filepath.Join(home, "personal")},
		},
		Rules: &config.NotebookRules{Default: "personal"},
	}}
	got := PlannedRoot(cfg, "registry")
	want := filepath.Join(home, "personal", "workspaces", "registry")
	if got != want {
		t.Errorf("PlannedRoot = %q, want the default notebook's root %q", got, want)
	}
}

// TestPlannedRootRefusesWithoutNotebooks: no declared notebook means nowhere
// legitimate to create one, and the caller must be told rather than handed a
// home-anchored guess.
func TestPlannedRootRefusesWithoutNotebooks(t *testing.T) {
	if root := PlannedRoot(&config.Config{}, "registry"); root != "" {
		t.Errorf("PlannedRoot invented %q with no notebooks declared", root)
	}
}
