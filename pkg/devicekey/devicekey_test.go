package devicekey

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/grovetools/core/pkg/machine"
)

func sandbox(t *testing.T) {
	t.Helper()
	t.Setenv("GROVE_HOME", t.TempDir())
}

func TestEnsureIsStableSecureAndSigns(t *testing.T) {
	sandbox(t)

	first, err := Ensure()
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	second, err := Ensure()
	if err != nil {
		t.Fatalf("Ensure again: %v", err)
	}
	if first.DeviceID() == "" || first.DeviceID() != second.DeviceID() {
		t.Fatalf("device IDs = %q, %q", first.DeviceID(), second.DeviceID())
	}
	if first.PublicKeyString() != second.PublicKeyString() {
		t.Fatal("Ensure replaced the existing key")
	}
	info, err := os.Lstat(Path())
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %04o, want 0600", got)
	}

	message := []byte("enroll this device")
	signature := first.Sign(message)
	if !ed25519.Verify(first.PublicKey(), message, signature) {
		t.Fatal("signature did not verify")
	}
	if first.Fingerprint() != second.Fingerprint() {
		t.Fatal("fingerprint changed across Load/Ensure")
	}
}

func TestEnsureConcurrentCreatorsConverge(t *testing.T) {
	sandbox(t)
	if _, err := machine.EnsureIdentity(); err != nil {
		t.Fatalf("EnsureIdentity: %v", err)
	}

	const workers = 12
	keys := make([]*Key, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			keys[i], errs[i] = Ensure()
		}()
	}
	wg.Wait()
	for i := range workers {
		if errs[i] != nil {
			t.Fatalf("Ensure[%d]: %v", i, errs[i])
		}
		if keys[i].PublicKeyString() != keys[0].PublicKeyString() {
			t.Fatalf("Ensure[%d] returned a different key", i)
		}
	}
}

func TestLoadMissingAndCorruptRefusal(t *testing.T) {
	sandbox(t)
	if _, err := machine.EnsureIdentity(); err != nil {
		t.Fatalf("EnsureIdentity: %v", err)
	}
	if key, err := Load(); err != nil || key != nil {
		t.Fatalf("Load missing = (%v, %v), want (nil, nil)", key, err)
	}
	if err := os.WriteFile(Path(), []byte("not json"), 0o600); err != nil {
		t.Fatalf("write corrupt key: %v", err)
	}
	if _, err := Load(); err == nil {
		t.Fatal("Load accepted corrupt state")
	}
	if _, err := Ensure(); err == nil {
		t.Fatal("Ensure silently replaced corrupt state")
	}
}

func TestLoadRejectsLooseModeAndSymlink(t *testing.T) {
	sandbox(t)
	if _, err := Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if err := os.Chmod(Path(), 0o644); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if _, err := Load(); err == nil {
		t.Fatal("Load accepted mode 0644")
	}
	if err := os.Chmod(Path(), 0o600); err != nil {
		t.Fatalf("chmod restore: %v", err)
	}
	real := Path() + ".real"
	if err := os.Rename(Path(), real); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if err := os.Symlink(real, Path()); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if _, err := Load(); err == nil {
		t.Fatal("Load accepted symlinked private key")
	}
}

func TestLoadRejectsMachineIDMismatch(t *testing.T) {
	sandbox(t)
	key, err := Ensure()
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	identity, err := machine.Load()
	if err != nil {
		t.Fatalf("machine.Load: %v", err)
	}
	identity.ID = machine.NewID()
	data, err := json.Marshal(identity)
	if err != nil {
		t.Fatalf("marshal identity: %v", err)
	}
	if err := os.WriteFile(machine.IdentityPath(), data, 0o600); err != nil {
		t.Fatalf("replace identity: %v", err)
	}
	if _, err := Load(); err == nil {
		t.Fatalf("Load accepted key bound to old machine %q", key.DeviceID())
	}
	if _, err := Ensure(); err == nil {
		t.Fatal("Ensure silently re-minted on machine mismatch")
	}
}

func TestLoadRejectsInconsistentPrivateKey(t *testing.T) {
	sandbox(t)
	if _, err := Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	data, err := os.ReadFile(Path())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var stored diskKey
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// A same-length value with a valid seed but a corrupt cached public half
	// must not be accepted by ed25519.Sign.
	private, err := base64.StdEncoding.DecodeString(stored.PrivateKey)
	if err != nil {
		t.Fatalf("decode private key: %v", err)
	}
	private[len(private)-1] ^= 0xff
	stored.PrivateKey = base64.StdEncoding.EncodeToString(private)
	data, _ = json.Marshal(stored)
	if err := os.WriteFile(Path(), data, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := Load(); err == nil {
		t.Fatal("Load accepted inconsistent private key")
	}
}

func TestPathBesideMachineIdentity(t *testing.T) {
	sandbox(t)
	if got, want := filepath.Dir(Path()), filepath.Dir(machine.IdentityPath()); got != want {
		t.Fatalf("device key dir = %q, machine identity dir = %q", got, want)
	}
}
