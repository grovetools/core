// Package machine owns this host's durable, non-portable identity.
//
// The identity is a ULID minted once and persisted to
// $XDG_STATE_HOME/grove/machine.json (via paths.StateDir()). It is *state*,
// not config: it is never symlinked, never hand-edited, and never travels in
// a dotfiles repo. Two hosts restored from the same dotfiles repository each
// mint their own ID — that is the supported fast path, not a fault.
//
// The display *name* is the config half of the identity and lives in
// ~/.config/grove/machine.toml (config.LoadMachineConfig). Because names can
// collide after a dotfiles restore, every user-facing surface renders
// "name (short id)" — see ShortID — and never the name alone as an
// identifier.
package machine

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/grovetools/core/pkg/paths"
)

// IdentityFileName is the basename of the state file inside paths.StateDir().
const IdentityFileName = "machine.json"

// ShortIDLen is how many ULID characters surfaces render as the
// disambiguating short form beside a machine name.
const ShortIDLen = 8

// Identity is the on-disk contents of machine.json.
type Identity struct {
	// ID is a ULID minted on this machine and never regenerated while the
	// file survives.
	ID string `json:"id"`
	// MintedAt records when this machine first got an identity.
	MintedAt time.Time `json:"minted_at"`
}

// IdentityPath returns the absolute path of the machine identity state file,
// or "" when no state directory can be resolved.
func IdentityPath() string {
	stateDir := paths.StateDir()
	if stateDir == "" {
		return ""
	}
	return filepath.Join(stateDir, IdentityFileName)
}

// Load reads the identity state file. A missing file is not an error: it
// returns (nil, nil), meaning "this machine has no identity yet". A present
// but unreadable or malformed file IS an error — callers must not silently
// re-mint over damaged state, because a fresh ID would orphan this machine's
// registry note.
func Load() (*Identity, error) {
	path := IdentityPath()
	if path == "" {
		return nil, fmt.Errorf("cannot resolve grove state directory")
	}
	return loadFrom(path)
}

func loadFrom(path string) (*Identity, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read machine identity %s: %w", path, err)
	}

	var id Identity
	if err := json.Unmarshal(data, &id); err != nil {
		return nil, fmt.Errorf("failed to parse machine identity %s: %w", path, err)
	}
	if id.ID == "" {
		return nil, fmt.Errorf("machine identity %s has an empty id", path)
	}
	return &id, nil
}

// EnsureIdentity is the idempotent read-or-create entry point: it returns the
// existing identity when machine.json is present, and mints + persists a new
// one when it is not. It is safe to call from both the CLI and daemon
// startup — whichever runs first mints, and every later call reads.
//
// Deleting machine.json is how an operator asks for a fresh identity; the
// next call mints one.
func EnsureIdentity() (*Identity, error) {
	path := IdentityPath()
	if path == "" {
		return nil, fmt.Errorf("cannot resolve grove state directory")
	}
	return ensureAt(path)
}

func ensureAt(path string) (*Identity, error) {
	if existing, err := loadFrom(path); err != nil {
		return nil, err
	} else if existing != nil {
		return existing, nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create state directory for %s: %w", path, err)
	}

	minted := &Identity{ID: NewID(), MintedAt: time.Now().UTC()}
	data, err := json.MarshalIndent(minted, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to encode machine identity: %w", err)
	}
	data = append(data, '\n')

	// Write via a temp file in the same directory so a crash mid-write can
	// never leave a truncated identity behind. O_EXCL on the rename target is
	// not available, so a concurrent minter may win the race — both wrote a
	// valid file, and the re-read below adopts whichever landed.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".machine-*.json")
	if err != nil {
		return nil, fmt.Errorf("failed to create machine identity temp file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return nil, fmt.Errorf("failed to write machine identity: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return nil, fmt.Errorf("failed to close machine identity temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return nil, fmt.Errorf("failed to install machine identity %s: %w", path, err)
	}

	// Re-read so a concurrent minter's file wins consistently for everyone.
	settled, err := loadFrom(path)
	if err != nil {
		return nil, err
	}
	if settled == nil {
		return minted, nil
	}
	return settled, nil
}

// ID is the plumbing-site convenience wrapper: it returns the ensured machine
// ID, or "" when the identity cannot be resolved. Sync clients thread it as
// DeviceID, where an empty string is exactly the pre-identity behavior — so a
// broken state directory degrades to today's semantics instead of failing a
// connection.
func ID() string {
	id, err := EnsureIdentity()
	if err != nil || id == nil {
		return ""
	}
	return id.ID
}

// ShortID returns the disambiguating short form of a machine ID — the
// ShortIDLen characters every surface renders beside the (collidable) name.
// Short or empty inputs are returned unchanged.
//
// It takes the TRAILING characters, not the leading ones. A ULID's first ten
// characters encode its 48-bit millisecond timestamp; an 8-character leading
// slice is therefore 40 bits of pure clock, identical for any two IDs minted
// within ~256ms of each other. That is exactly the case this short form must
// distinguish — two hosts restored back-to-back from one dotfiles repo — so a
// leading slice would collide precisely where disambiguation matters. The
// trailing characters are drawn from the ULID's 80 random bits.
func ShortID(id string) string {
	if len(id) <= ShortIDLen {
		return id
	}
	return id[len(id)-ShortIDLen:]
}

// Describe renders the canonical "name (short id)" identity label. It never
// renders a bare name: names collide across machines restored from one
// dotfiles repo, so the prefix is what makes the label an identifier.
func Describe(name, id string) string {
	short := ShortID(id)
	switch {
	case name == "" && short == "":
		return "unknown"
	case short == "":
		return name
	case name == "":
		return short
	default:
		return fmt.Sprintf("%s (%s)", name, short)
	}
}

// NewID mints a fresh ULID string (Crockford base32, 26 chars): a
// millisecond timestamp prefix plus 80 bits of cryptographic randomness, so
// IDs sort by mint time and two machines restored from one dotfiles repo can
// never collide.
func NewID() string {
	return ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader).String()
}
