package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEcosystemCardIdentityOnlyLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "grove.toml")
	content := "title = \"kept\"\n\n[ecosystem]\nid = \"01JCARD0000000000000000000\"\nlayout = \"flat\"\n\n[[ecosystem.remotes]]\nname = \"origin\"\nurl = \"stale\"\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	card, err := LoadEcosystemCard(path)
	if err != nil {
		t.Fatal(err)
	}
	if card == nil || card.ID != "01JCARD0000000000000000000" {
		t.Fatalf("card = %#v", card)
	}
}

func TestWriteEcosystemCardSlimsAndPreservesManifest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "grove.toml")
	before := "# keep\ntitle = \"kept\"\n\n[ecosystem]\nid = \"01JCARD0000000000000000000\"\nlayout = \"flat\"\n\n[[ecosystem.remotes]]\nname = \"origin\"\nurl = \"stale\"\n\n[plugins.demo]\nenabled = true\n"
	if err := os.WriteFile(path, []byte(before), 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err := WriteEcosystemCard(path, EcosystemCard{ID: "01JCARD0000000000000000000"})
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	data, _ := os.ReadFile(path)
	got := string(data)
	for _, stale := range []string{"layout", "ecosystem.remotes", "url = \"stale\""} {
		if strings.Contains(got, stale) {
			t.Fatalf("stale card field %q survived:\n%s", stale, got)
		}
	}
	for _, kept := range []string{"# keep", "title = \"kept\"", "[plugins.demo]", "enabled = true"} {
		if !strings.Contains(got, kept) {
			t.Fatalf("lost %q:\n%s", kept, got)
		}
	}
}

func TestWriteEcosystemCardIDImmutable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "grove.toml")
	if err := os.WriteFile(path, []byte("[ecosystem]\nid = \"01JCARD0000000000000000000\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteEcosystemCard(path, EcosystemCard{ID: "01JOTHER000000000000000000"}); err == nil {
		t.Fatal("expected immutable id error")
	}
}

func TestWriteEcosystemCardIdempotentAndCreatesManifest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "grove.toml")
	card := EcosystemCard{ID: "01JCARD0000000000000000000"}
	changed, err := WriteEcosystemCard(path, card)
	if err != nil || !changed {
		t.Fatalf("first changed=%v err=%v", changed, err)
	}
	before, _ := os.ReadFile(path)
	changed, err = WriteEcosystemCard(path, card)
	if err != nil || changed {
		t.Fatalf("second changed=%v err=%v", changed, err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("idempotent write changed bytes")
	}
}

func TestEcosystemCardValidateAndFindManifest(t *testing.T) {
	card := EcosystemCard{ID: " spaced "}
	if err := card.Validate(); err == nil {
		t.Fatal("expected whitespace id error")
	}
	dir := t.TempDir()
	if got := FindEcosystemManifest(dir); got != "" {
		t.Fatalf("got %q", got)
	}
	for _, name := range []string{"grove.yml", "grove.toml"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("name: x\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if got := FindEcosystemManifest(dir); got != filepath.Join(dir, "grove.toml") {
		t.Fatalf("got %q", got)
	}
}

func TestWriteSlimYAMLCard(t *testing.T) {
	path := filepath.Join(t.TempDir(), "grove.yml")
	if err := os.WriteFile(path, []byte("name: kept\necosystem:\n  id: old\n  layout: flat\nother: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Existing non-empty id remains immutable, so use the same id.
	if _, err := WriteEcosystemCard(path, EcosystemCard{ID: "old"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	got := string(data)
	if strings.Contains(got, "layout:") || !strings.Contains(got, "other: true") {
		t.Fatalf("unexpected yaml:\n%s", got)
	}
}
