package machine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// sandboxState points paths.StateDir() at a temp dir via GROVE_HOME so no
// test ever touches the developer's real ~/.local/state/grove.
func sandboxState(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GROVE_HOME", home)
	return filepath.Join(home, "state", "grove")
}

func TestEnsureIdentityMintsOnceAndIsStable(t *testing.T) {
	stateDir := sandboxState(t)

	first, err := EnsureIdentity()
	if err != nil {
		t.Fatalf("EnsureIdentity: %v", err)
	}
	if first.ID == "" {
		t.Fatal("minted identity has an empty id")
	}
	if first.MintedAt.IsZero() {
		t.Fatal("minted identity has a zero minted_at")
	}
	if got, want := IdentityPath(), filepath.Join(stateDir, IdentityFileName); got != want {
		t.Fatalf("IdentityPath() = %q, want %q", got, want)
	}

	// Acceptance: repeated runs / daemon restarts keep the same ID.
	for i := range 3 {
		again, err := EnsureIdentity()
		if err != nil {
			t.Fatalf("EnsureIdentity call %d: %v", i, err)
		}
		if again.ID != first.ID {
			t.Fatalf("call %d re-minted: got %q, want %q", i, again.ID, first.ID)
		}
	}

	if ID() != first.ID {
		t.Fatalf("ID() = %q, want %q", ID(), first.ID)
	}
}

func TestEnsureIdentityRemintsAfterDeletion(t *testing.T) {
	sandboxState(t)

	first, err := EnsureIdentity()
	if err != nil {
		t.Fatalf("EnsureIdentity: %v", err)
	}
	if err := os.Remove(IdentityPath()); err != nil {
		t.Fatalf("remove identity: %v", err)
	}

	second, err := EnsureIdentity()
	if err != nil {
		t.Fatalf("EnsureIdentity after delete: %v", err)
	}
	if second.ID == first.ID {
		t.Fatalf("deleting machine.json did not mint a fresh id (%q)", second.ID)
	}
}

// Two hosts restored from one dotfiles repo share config but not state; each
// must mint its own ID. Distinct state dirs model the two hosts.
func TestDistinctStateDirsMintDistinctIDs(t *testing.T) {
	var ids []string
	for range 2 {
		func() {
			sandboxState(t)
			id, err := EnsureIdentity()
			if err != nil {
				t.Fatalf("EnsureIdentity: %v", err)
			}
			ids = append(ids, id.ID)
		}()
	}
	if ids[0] == ids[1] {
		t.Fatalf("two hosts minted the same id %q", ids[0])
	}
}

func TestLoadMissingIsNotAnError(t *testing.T) {
	sandboxState(t)

	id, err := Load()
	if err != nil {
		t.Fatalf("Load on a fresh machine: %v", err)
	}
	if id != nil {
		t.Fatalf("Load on a fresh machine returned %+v, want nil", id)
	}
}

// A damaged identity must fail loudly: silently re-minting would orphan this
// machine's registry note under a new ID.
func TestLoadRejectsCorruptIdentity(t *testing.T) {
	stateDir := sandboxState(t)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}

	for name, body := range map[string]string{
		"not json":     "{{{",
		"empty id":     `{"id":"","minted_at":"2026-01-01T00:00:00Z"}`,
		"missing keys": `{}`,
	} {
		if err := os.WriteFile(IdentityPath(), []byte(body), 0o644); err != nil {
			t.Fatalf("%s: write: %v", name, err)
		}
		if _, err := Load(); err == nil {
			t.Errorf("%s: Load accepted corrupt identity %q", name, body)
		}
		if _, err := EnsureIdentity(); err == nil {
			t.Errorf("%s: EnsureIdentity re-minted over corrupt identity %q", name, body)
		}
		// ID() degrades to "" rather than panicking at a plumbing site.
		if got := ID(); got != "" {
			t.Errorf("%s: ID() = %q, want \"\"", name, got)
		}
	}
}

func TestPersistedShapeIsStable(t *testing.T) {
	sandboxState(t)

	minted, err := EnsureIdentity()
	if err != nil {
		t.Fatalf("EnsureIdentity: %v", err)
	}
	data, err := os.ReadFile(IdentityPath())
	if err != nil {
		t.Fatalf("read identity: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("identity file is not valid json: %v", err)
	}
	if len(raw) != 2 || raw["id"] == nil || raw["minted_at"] == nil {
		t.Fatalf("identity file keys = %v, want exactly id + minted_at", raw)
	}
	if raw["id"] != minted.ID {
		t.Fatalf("persisted id %v != returned id %q", raw["id"], minted.ID)
	}
	if _, err := time.Parse(time.RFC3339, raw["minted_at"].(string)); err != nil {
		t.Fatalf("minted_at is not RFC3339: %v", err)
	}
}

func TestNewIDIsAULID(t *testing.T) {
	seen := map[string]bool{}
	for range 100 {
		id := NewID()
		if len(id) != 26 {
			t.Fatalf("NewID() = %q (len %d), want a 26-char ULID", id, len(id))
		}
		if strings.ContainsAny(id, "ILOU") {
			t.Fatalf("NewID() = %q contains a non-Crockford-base32 character", id)
		}
		if seen[id] {
			t.Fatalf("NewID() repeated %q", id)
		}
		seen[id] = true
	}
}

func TestShortIDAndDescribe(t *testing.T) {
	long := "01K1ABCDEFGHJKMNPQRSTVWXYZ"
	if got, want := ShortID(long), "NPQRSTVWXYZ"[3:]; got != want {
		t.Errorf("ShortID(%q) = %q, want the trailing %q", long, got, want)
	}
	if got := ShortID("short"); got != "short" {
		t.Errorf("ShortID short input = %q, want unchanged", got)
	}

	for _, tc := range []struct{ name, id, want string }{
		{"", "", "unknown"},
		{"mbp", "", "mbp"},
		{"", long, "RSTVWXYZ"},
		{"mbp", long, "mbp (RSTVWXYZ)"},
	} {
		if got := Describe(tc.name, tc.id); got != tc.want {
			t.Errorf("Describe(%q, %q) = %q, want %q", tc.name, tc.id, got, tc.want)
		}
	}
}

// The guard, at its hardest: two hosts restored back-to-back from one
// dotfiles repo share a name AND mint within the same millisecond, so their
// ULIDs share every timestamp character. The short form must still tell them
// apart — which is why it is drawn from the random tail, not the clock head.
func TestShortIDDisambiguatesSameMillisecondMints(t *testing.T) {
	sandboxState(t)

	seen := map[string]string{}
	for range 200 {
		id := NewID()
		short := ShortID(id)
		if prev, dup := seen[short]; dup {
			t.Fatalf("short form %q collided between %q and %q", short, prev, id)
		}
		seen[short] = id
	}

	// And concretely: same timestamp head, different tails.
	a := "01K1ABCDEFGHJKMNPQRSTVWXYZ"
	b := "01K1ABCDEF0123456789ABCDEF"
	if a[:10] != b[:10] {
		t.Fatalf("test fixture is wrong: %q and %q should share a timestamp head", a, b)
	}
	if Describe("mbp", a) == Describe("mbp", b) {
		t.Fatalf("two same-millisecond machines named mbp rendered identically as %q", Describe("mbp", a))
	}
}
