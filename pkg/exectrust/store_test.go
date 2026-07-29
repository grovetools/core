package exectrust

import (
	"os"
	"path/filepath"
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
