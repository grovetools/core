package exectrust

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// isolate points the store at a temp file so tests never touch the developer's
// real trust decisions.
func isolate(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "exec-trust.json")
	t.Setenv(EnvStorePath, path)
	return path
}

func TestLoadMissingStoreTrustsNothing(t *testing.T) {
	isolate(t)

	s := Load()
	if s.IsTrusted("/repo/grove.toml", "sha256:abc") {
		t.Error("a missing store must trust nothing")
	}
	if len(s.Entries()) != 0 {
		t.Errorf("expected an empty store, got %v", s.Entries())
	}
}

func TestTrustRoundTrip(t *testing.T) {
	isolate(t)
	const digest = "sha256:abc"

	s := Load()
	s.Trust("/repo/grove.toml", digest, time.Now())
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reloaded := Load()
	if !reloaded.IsTrusted("/repo/grove.toml", digest) {
		t.Error("a trusted file must survive a save/load round trip")
	}
	entries := reloaded.Entries()
	if len(entries) != 1 || entries[0].TrustedAt == "" {
		t.Errorf("expected one dated entry, got %+v", entries)
	}
}

func TestTrustIsBoundToTheDigest(t *testing.T) {
	isolate(t)

	s := Load()
	s.Trust("/repo/grove.toml", "sha256:reviewed", time.Now())

	if s.IsTrusted("/repo/grove.toml", "sha256:changed") {
		t.Error("a different digest must not be trusted — that is the whole point of recording one")
	}
	if s.IsTrusted("/other/grove.toml", "sha256:reviewed") {
		t.Error("trust must not leak to another path")
	}
}

func TestEmptyDigestIsNeverTrusted(t *testing.T) {
	isolate(t)

	s := Load()
	s.Trust("/repo/grove.toml", "", time.Now())
	if s.IsTrusted("/repo/grove.toml", "") {
		t.Error("an empty digest must never satisfy the gate")
	}
}

func TestRevoke(t *testing.T) {
	isolate(t)

	s := Load()
	s.Trust("/repo/grove.toml", "sha256:abc", time.Now())
	if !s.Revoke("/repo/grove.toml") {
		t.Error("Revoke must report that it removed a record")
	}
	if s.Revoke("/repo/grove.toml") {
		t.Error("Revoke must report false for a path it does not hold")
	}
	if s.IsTrusted("/repo/grove.toml", "sha256:abc") {
		t.Error("a revoked path must not be trusted")
	}
}

func TestMalformedStoreFailsClosed(t *testing.T) {
	path := isolate(t)
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write store: %v", err)
	}

	s := Load()
	if s.IsTrusted("/repo/grove.toml", "sha256:abc") {
		t.Error("an unparseable store must trust nothing, not everything")
	}
}

func TestFutureVersionStoreFailsClosed(t *testing.T) {
	path := isolate(t)
	if err := os.WriteFile(path, []byte(`{"version":99,"files":{"/repo/grove.toml":{"digest":"sha256:abc"}}}`), 0o600); err != nil {
		t.Fatalf("write store: %v", err)
	}

	s := Load()
	if s.IsTrusted("/repo/grove.toml", "sha256:abc") {
		t.Error("a store written by a newer grove must not be reinterpreted as trust")
	}
}

func TestSaveIsOwnerOnly(t *testing.T) {
	path := isolate(t)

	s := Load()
	s.Trust("/repo/grove.toml", "sha256:abc", time.Now())
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat store: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("trust store mode = %o, want 600", perm)
	}
}

func TestDigestIsOrderIndependentAndCollisionSafe(t *testing.T) {
	a := Digest([]string{"build_cmd=make", "hooks.on_stop=[echo hi]"})
	b := Digest([]string{"hooks.on_stop=[echo hi]", "build_cmd=make"})
	if a != b {
		t.Errorf("digest must not depend on ordering: %q vs %q", a, b)
	}
	if Digest(nil) != "" {
		t.Error("no exec values must produce an empty digest")
	}
	if Digest([]string{"a", "bc"}) == Digest([]string{"ab", "c"}) {
		t.Error("digest must not be vulnerable to naive concatenation collisions")
	}
	if Digest([]string{"build_cmd=make"}) == Digest([]string{"build_cmd=make evil"}) {
		t.Error("changing a value must change the digest")
	}
}

func TestStorePathHonorsOverride(t *testing.T) {
	path := isolate(t)
	if got := StorePath(); got != path {
		t.Errorf("StorePath() = %q, want the override %q", got, path)
	}
}

// --- store integrity (F4) -------------------------------------------------

// forgeStore writes a trust store by hand — exactly what a process running
// inside a workspace would do if it could reach the store path — and returns
// the config path it tries to self-trust.
func forgeStore(t *testing.T, storePath string, entry Entry) string {
	t.Helper()
	const target = "/repo/grove.toml"
	body := `{"version":2,"files":{"` + target + `":{"digest":"` + entry.Digest +
		`","trusted_at":"` + entry.TrustedAt + `","mac":"` + entry.MAC + `"}}}`
	if err := os.WriteFile(storePath, []byte(body), 0o600); err != nil {
		t.Fatalf("write forged store: %v", err)
	}
	return target
}

// TestForgedEntryWithoutAMACIsRejected is the core of F4: writing the store
// file is not enough to trust anything. Only Save — reached through
// `grove config trust` — mints entries Load will honor.
func TestForgedEntryWithoutAMACIsRejected(t *testing.T) {
	storePath := isolate(t)
	const digest = "sha256:selftrusted"

	// The forger even mints the key file first, in case Load only checked for
	// its existence.
	if _, err := ensureKey(filepath.Join(filepath.Dir(storePath), keyFileName)); err != nil {
		t.Fatalf("ensureKey: %v", err)
	}
	target := forgeStore(t, storePath, Entry{Digest: digest, TrustedAt: time.Now().UTC().Format(time.RFC3339)})

	if Load().IsTrusted(target, digest) {
		t.Fatal("an entry written directly into the store must not be trusted")
	}
}

// TestForgedEntryWithAWrongMACIsRejected covers the forger that knows the
// scheme but not the machine's key — a store copied in from elsewhere, or a
// MAC guessed from a public constant.
func TestForgedEntryWithAWrongMACIsRejected(t *testing.T) {
	storePath := isolate(t)
	const digest = "sha256:selftrusted"
	now := time.Now().UTC().Format(time.RFC3339)

	if _, err := ensureKey(filepath.Join(filepath.Dir(storePath), keyFileName)); err != nil {
		t.Fatalf("ensureKey: %v", err)
	}
	foreign := entryMAC([]byte("an attacker's own 32-byte-ish key!!"), "/repo/grove.toml",
		Entry{Digest: digest, TrustedAt: now})
	target := forgeStore(t, storePath, Entry{Digest: digest, TrustedAt: now, MAC: foreign})

	if Load().IsTrusted(target, digest) {
		t.Fatal("an entry MACed with a foreign key must not be trusted")
	}
}

// TestTamperedEntryIsRejected covers editing a legitimately trusted record in
// place — swapping the digest so a repo's NEW commands ride the old decision.
func TestTamperedEntryIsRejected(t *testing.T) {
	storePath := isolate(t)
	const target = "/repo/grove.toml"

	s := Load()
	s.Trust(target, "sha256:reviewed", time.Now())
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !Load().IsTrusted(target, "sha256:reviewed") {
		t.Fatal("a legitimately saved entry must load as trusted")
	}

	data, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatalf("read store: %v", err)
	}
	tampered := strings.Replace(string(data), "sha256:reviewed", "sha256:sneaked-in", 1)
	if err := os.WriteFile(storePath, []byte(tampered), 0o600); err != nil {
		t.Fatalf("write tampered store: %v", err)
	}

	loaded := Load()
	if loaded.IsTrusted(target, "sha256:sneaked-in") {
		t.Error("swapping the digest under a valid MAC must not be trusted")
	}
	if loaded.IsTrusted(target, "sha256:reviewed") {
		t.Error("a tampered record must be dropped entirely, not silently repaired")
	}
}

// TestStoreWithoutItsKeyTrustsNothing: deleting or losing the key must fail
// closed, not open.
func TestStoreWithoutItsKeyTrustsNothing(t *testing.T) {
	storePath := isolate(t)
	const target = "/repo/grove.toml"

	s := Load()
	s.Trust(target, "sha256:abc", time.Now())
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := os.Remove(filepath.Join(filepath.Dir(storePath), keyFileName)); err != nil {
		t.Fatalf("remove key: %v", err)
	}

	if Load().IsTrusted(target, "sha256:abc") {
		t.Error("a store whose key is gone must trust nothing")
	}
}

// TestLooselyPermissionedStoreTrustsNothing: a store any other account can
// rewrite is not a decision record.
func TestLooselyPermissionedStoreTrustsNothing(t *testing.T) {
	storePath := isolate(t)
	const target = "/repo/grove.toml"

	s := Load()
	s.Trust(target, "sha256:abc", time.Now())
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !Load().IsTrusted(target, "sha256:abc") {
		t.Fatal("precondition: the saved store must load as trusted")
	}

	if err := os.Chmod(storePath, 0o666); err != nil {
		t.Fatalf("chmod store: %v", err)
	}
	if Load().IsTrusted(target, "sha256:abc") {
		t.Error("a world-writable store must trust nothing")
	}

	if err := os.Chmod(storePath, 0o600); err != nil {
		t.Fatalf("restore store mode: %v", err)
	}
	if err := os.Chmod(filepath.Dir(storePath), 0o777); err != nil {
		t.Fatalf("chmod store dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Dir(storePath), 0o700) })
	if Load().IsTrusted(target, "sha256:abc") {
		t.Error("a store in a world-writable directory must trust nothing")
	}
}

// TestKeyFileIsNotWorldReadable pins the key's mode: the MAC is only worth
// anything while the key stays private to this account.
func TestKeyFileIsNotWorldReadable(t *testing.T) {
	storePath := isolate(t)

	s := Load()
	s.Trust("/repo/grove.toml", "sha256:abc", time.Now())
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(filepath.Join(filepath.Dir(storePath), keyFileName))
	if err != nil {
		t.Fatalf("stat key: %v", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Errorf("exec-trust key mode %v must be 0600", info.Mode().Perm())
	}
}
