// Package devicekey owns the Ed25519 key that proves this machine's durable
// identity. The private key is local state and must never be copied as part of
// configuration or enrollment.
package devicekey

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/grovetools/core/pkg/machine"
	"github.com/grovetools/core/pkg/paths"
)

const (
	// FileName is the basename of the device key file in paths.StateDir().
	FileName    = "device-key.json"
	fileVersion = 1
)

// Key is a machine-bound Ed25519 signing key. Its private material is kept
// unexported so callers cannot accidentally serialize it.
type Key struct {
	deviceID string
	private  ed25519.PrivateKey
}

type diskKey struct {
	Version    int    `json:"version"`
	DeviceID   string `json:"device_id"`
	PrivateKey string `json:"private_key"`
}

// Path returns the device key's state path, or "" when no state directory
// can be resolved.
func Path() string {
	stateDir := paths.StateDir()
	if stateDir == "" {
		return ""
	}
	return filepath.Join(stateDir, FileName)
}

// Load reads and validates the existing key. A missing key returns (nil,
// nil). A present key must be a regular, non-symlink 0600 file bound to the
// current machine identity; malformed or mismatched state is never replaced.
func Load() (*Key, error) {
	path := Path()
	if path == "" {
		return nil, fmt.Errorf("cannot resolve grove state directory")
	}
	identity, err := machine.Load()
	if err != nil {
		return nil, err
	}
	if identity == nil {
		return nil, fmt.Errorf("cannot load device key without a machine identity")
	}
	return loadFrom(path, identity.ID)
}

func loadFrom(path, machineID string) (*Key, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to inspect device key %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("device key %s is not a regular file", path)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		return nil, fmt.Errorf("device key %s has mode %04o, want 0600", path, got)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read device key %s: %w", path, err)
	}
	var stored diskKey
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&stored); err != nil {
		return nil, fmt.Errorf("failed to parse device key %s: %w", path, err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("failed to parse device key %s: trailing data", path)
	}
	if stored.Version != fileVersion {
		return nil, fmt.Errorf("device key %s has unsupported version %d", path, stored.Version)
	}
	if stored.DeviceID == "" || stored.DeviceID != machineID {
		return nil, fmt.Errorf("device key %s belongs to machine %q, current machine is %q", path, stored.DeviceID, machineID)
	}
	private, err := base64.StdEncoding.Strict().DecodeString(stored.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("device key %s has invalid private_key encoding: %w", path, err)
	}
	if len(private) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("device key %s has a %d-byte private key, want %d", path, len(private), ed25519.PrivateKeySize)
	}
	// Ed25519 private keys contain a cached public half. Derive it again from
	// the seed so corruption in either half cannot be accepted silently.
	derived := ed25519.NewKeyFromSeed(private[:ed25519.SeedSize])
	if !bytes.Equal(private, derived) {
		return nil, fmt.Errorf("device key %s is internally inconsistent", path)
	}
	return &Key{deviceID: machineID, private: derived}, nil
}

// Ensure returns the existing machine-bound key or atomically installs a new
// one. Concurrent creators converge on the first linked file and re-read it.
func Ensure() (*Key, error) {
	path := Path()
	if path == "" {
		return nil, fmt.Errorf("cannot resolve grove state directory")
	}
	identity, err := machine.EnsureIdentity()
	if err != nil {
		return nil, err
	}
	if existing, err := loadFrom(path, identity.ID); err != nil {
		return nil, err
	} else if existing != nil {
		return existing, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create state directory for %s: %w", path, err)
	}

	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate device key: %w", err)
	}
	stored := diskKey{
		Version:    fileVersion,
		DeviceID:   identity.ID,
		PrivateKey: base64.StdEncoding.EncodeToString(private),
	}
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to encode device key: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), ".device-key-*.json")
	if err != nil {
		return nil, fmt.Errorf("failed to create device key temp file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		cleanup()
		return nil, fmt.Errorf("failed to secure device key temp file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return nil, fmt.Errorf("failed to write device key: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return nil, fmt.Errorf("failed to sync device key: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return nil, fmt.Errorf("failed to close device key: %w", err)
	}

	// Link gives the destination O_EXCL semantics that Rename lacks: an
	// existing key is never overwritten, including under concurrent Ensure.
	if err := os.Link(tmpName, path); err != nil && !os.IsExist(err) {
		cleanup()
		return nil, fmt.Errorf("failed to install device key %s: %w", path, err)
	}
	cleanup()

	settled, err := loadFrom(path, identity.ID)
	if err != nil {
		return nil, err
	}
	if settled == nil {
		return nil, fmt.Errorf("device key %s disappeared after installation", path)
	}
	return settled, nil
}

// DeviceID returns the machine identity to which this key is bound.
func (k *Key) DeviceID() string { return k.deviceID }

// PublicKey returns a defensive copy of the Ed25519 public key.
func (k *Key) PublicKey() ed25519.PublicKey {
	public := k.private.Public().(ed25519.PublicKey)
	return append(ed25519.PublicKey(nil), public...)
}

// PublicKeyString returns the canonical base64 encoding used on the wire.
func (k *Key) PublicKeyString() string {
	return base64.StdEncoding.EncodeToString(k.PublicKey())
}

// Sign signs message with this machine's device key.
func (k *Key) Sign(message []byte) []byte {
	return ed25519.Sign(k.private, message)
}

// Fingerprint returns the full lowercase hexadecimal SHA-256 digest of the
// raw 32-byte public key.
func (k *Key) Fingerprint() string {
	sum := sha256.Sum256(k.PublicKey())
	return hex.EncodeToString(sum[:])
}
