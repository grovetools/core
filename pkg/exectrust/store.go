// Package exectrust records which grove config files the user has reviewed
// and trusted to carry exec-bearing keys.
//
// Grove's config cascade merges workspace/project/ecosystem/notebook
// grove.toml files into the effective config, and several config keys carry
// shell commands grove (or its satellites) will execute — [[hooks.on_stop]],
// [daemon.hooks.on_skill_sync], [tui.plugins], [tui.panels.bindings], and
// friends. Those files come from cloned repositories, so honoring them
// unconditionally hands a repo author code execution the moment the user runs
// an agent session inside a clone. core/config gates them on this store.
//
// The unit of trust is one config FILE plus a DIGEST of the exec values it
// carried when the user reviewed it. Binding trust to the digest means a
// repository that later adds or edits a command falls back to untrusted
// instead of riding on a decision the user made about different content.
//
// This package is deliberately a leaf: it imports only core/pkg/paths so that
// core/config can depend on it without a cycle. It knows nothing about which
// config keys are exec-bearing — that classification lives in core/config.
//
// # Store integrity
//
// The store is the thing that decides whether the gate opens, so it has to be
// at least as hard to write as the config it gates. Three properties, all
// enforced in Load (fail closed — any of them failing yields an EMPTY store,
// never a permissive one):
//
//  1. LOCATION. It lives under the XDG state dir (paths.StateDir()), outside
//     every workspace and worktree, so no workspace-resident path reaches it.
//  2. OWNERSHIP AND MODE. The store file and its directory must be owned by
//     the current user, the file must not be group- or world-readable/writable
//     (0600), and the directory must not be group- or world-writable.
//  3. AUTHENTICATION. Every entry carries a MAC over (path, digest,
//     trusted_at) keyed by a per-machine secret that Save mints next to the
//     store (exec-trust.key, 0600). Entries whose MAC is absent or does not
//     verify are DROPPED on load, so hand-writing an entry into the JSON — the
//     forgery a process running inside a workspace would attempt — does not
//     trust anything. Only Save, i.e. the `grove config trust` path, produces
//     verifiable entries.
//
// The MAC is an authenticity check, not a secrecy barrier: a process that can
// read exec-trust.key can mint entries. That is why the seeder's protectConfig
// rules (core/pkg/claudenotebook.protectedConfigPaths) deny an agent writes to
// the whole exec-trust* set — store, its .tmp, and the key — and why the store
// lives outside the sandbox's writable boundary. The MAC closes the residual
// write-without-read paths (a restored backup, a synced dotfile tree, a
// process that can create files in the state dir but never read the key).
package exectrust

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/grovetools/core/pkg/paths"
)

// storeVersion is the on-disk schema version. A file carrying a newer version
// is treated as empty (fail closed: nothing is trusted) rather than being
// reinterpreted or clobbered.
//
// v2 added the per-entry MAC. A v1 store loads without error but every entry
// fails MAC verification and is dropped, so upgrading users re-run
// `grove config trust` once. That is the correct direction to fail.
const storeVersion = 2

// EnvStorePath overrides the store location. Set by tests and by sandboxed
// runs that must not touch the user's real trust decisions.
const EnvStorePath = "GROVE_EXEC_TRUST_FILE"

// keyFileName is the per-machine MAC secret, kept next to the store.
const keyFileName = "exec-trust.key"

// Entry is one trusted config file's record.
type Entry struct {
	// Digest is the exec-value digest that was reviewed (see Digest).
	Digest string `json:"digest"`
	// TrustedAt is an RFC3339 timestamp, for `grove config trust --list`.
	TrustedAt string `json:"trusted_at"`
	// MAC authenticates this record against the store key (see entryMAC). An
	// entry without a verifying MAC was not written by `grove config trust`
	// and is dropped on load.
	MAC string `json:"mac"`
}

// Store is the on-disk document: absolute config file path -> Entry.
type Store struct {
	Version int              `json:"version"`
	Files   map[string]Entry `json:"files"`

	path string // resolved store path; empty when unresolvable
}

// StorePath returns the trust store location, honoring EnvStorePath. Returns
// "" when neither the override nor the XDG state dir can be resolved, in
// which case nothing can be trusted and the gate stays closed.
func StorePath() string {
	if override := strings.TrimSpace(os.Getenv(EnvStorePath)); override != "" {
		return override
	}
	stateDir := paths.StateDir()
	if stateDir == "" {
		return ""
	}
	return filepath.Join(stateDir, "exec-trust.json")
}

// KeyPath returns the location of the store's MAC secret, which lives next to
// the store itself. Empty when the store path is unresolvable. Exported so the
// settings seeder can put it behind the same deny rules as the store.
func KeyPath() string {
	store := StorePath()
	if store == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(store), keyFileName)
}

// Load reads the trust store. A missing, unreadable, malformed, badly-owned,
// loosely-permissioned or future-versioned file yields an empty store and no
// error, and so does any entry that fails MAC verification: an unusable store
// must degrade to "nothing is trusted", never to "everything is trusted", and
// never block a config load.
func Load() *Store {
	s := &Store{Version: storeVersion, Files: map[string]Entry{}, path: StorePath()}
	if s.path == "" {
		return s
	}
	if !ownedAndTight(filepath.Dir(s.path), 0o022) || !ownedAndTight(s.path, 0o077) {
		return s
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return s
	}
	var onDisk Store
	if err := json.Unmarshal(data, &onDisk); err != nil {
		return s
	}
	if onDisk.Version > storeVersion {
		return s
	}
	if len(onDisk.Files) == 0 {
		return s
	}
	key, err := loadKey(s.keyPath())
	if err != nil {
		// No verifiable key means no verifiable entries. Fail closed.
		return s
	}
	for path, entry := range onDisk.Files {
		if entry.MAC == "" {
			continue
		}
		want := entryMAC(key, path, entry)
		if subtle.ConstantTimeCompare([]byte(entry.MAC), []byte(want)) == 1 {
			s.Files[path] = entry
		}
	}
	return s
}

// keyPath is the store's own MAC-secret location, derived from its resolved
// path so a test-redirected store gets a test-local key.
func (s *Store) keyPath() string {
	if s == nil || s.path == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(s.path), keyFileName)
}

// ownedAndTight reports whether path exists, is owned by the current user, and
// carries none of the permission bits in forbidden. A path that does not exist
// yet passes (there is nothing to distrust); anything else failing is a reason
// to treat the store as unusable.
func ownedAndTight(path string, forbidden fs.FileMode) bool {
	info, err := os.Stat(path)
	if err != nil {
		return os.IsNotExist(err)
	}
	if info.Mode().Perm()&forbidden != 0 {
		return false
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Getuid() {
		return false
	}
	return true
}

// loadKey reads the per-machine MAC secret. A missing, unreadable, or
// loosely-permissioned key is an error, which Load turns into an empty store.
func loadKey(path string) ([]byte, error) {
	if path == "" {
		return nil, fmt.Errorf("exec-trust key path unresolvable")
	}
	if !ownedAndTight(path, 0o077) {
		return nil, fmt.Errorf("exec-trust key %s is not owned by this user or is too permissive", path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	key, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(key) < 32 {
		return nil, fmt.Errorf("exec-trust key %s is malformed", path)
	}
	return key, nil
}

// ensureKey returns the per-machine MAC secret, minting it on first use.
func ensureKey(path string) ([]byte, error) {
	if key, err := loadKey(path); err == nil {
		return key, nil
	}
	if path == "" {
		return nil, fmt.Errorf("exec-trust key path unresolvable")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create exec-trust dir: %w", err)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate exec-trust key: %w", err)
	}
	if err := os.WriteFile(path, []byte(hex.EncodeToString(key)), 0o600); err != nil {
		return nil, fmt.Errorf("write %s: %w", path, err)
	}
	return key, nil
}

// entryMAC authenticates one record. The store KEY is bound in as well as the
// value, so an entry cannot be moved to a different path or replayed against a
// different digest.
func entryMAC(key []byte, path string, e Entry) string {
	h := hmac.New(sha256.New, key)
	for _, part := range []string{path, e.Digest, e.TrustedAt} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return "hmac-sha256:" + hex.EncodeToString(h.Sum(nil))
}

// IsTrusted reports whether path was trusted with exactly this digest.
// A path trusted under a different digest — the file changed since review —
// is not trusted.
func (s *Store) IsTrusted(path, digest string) bool {
	if s == nil || len(s.Files) == 0 {
		return false
	}
	entry, ok := s.Files[canonical(path)]
	return ok && entry.Digest == digest && digest != ""
}

// Entries returns every record, sorted by path, for listing UX.
func (s *Store) Entries() []struct {
	Path string
	Entry
} {
	out := make([]struct {
		Path string
		Entry
	}, 0, len(s.Files))
	for p, e := range s.Files {
		out = append(out, struct {
			Path string
			Entry
		}{Path: p, Entry: e})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// Trust records path as trusted at digest, replacing any prior record for it.
// now is injected so callers (and tests) control the timestamp.
func (s *Store) Trust(path, digest string, now time.Time) {
	if s.Files == nil {
		s.Files = map[string]Entry{}
	}
	s.Files[canonical(path)] = Entry{
		Digest:    digest,
		TrustedAt: now.UTC().Format(time.RFC3339),
	}
}

// Revoke drops the record for path. Reports whether anything was removed.
func (s *Store) Revoke(path string) bool {
	key := canonical(path)
	if _, ok := s.Files[key]; !ok {
		return false
	}
	delete(s.Files, key)
	return true
}

// Save writes the store atomically (tmp file + rename), 0600 because the
// records name paths on the user's machine and the file is a security
// decision record. Every entry is (re)MACed here, which is what makes Save —
// reached only through `grove config trust` — the sole way to produce a record
// Load will honor.
func (s *Store) Save() error {
	if s.path == "" {
		return fmt.Errorf("cannot resolve exec-trust store path (set %s or XDG_STATE_HOME)", EnvStorePath)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create exec-trust dir: %w", err)
	}
	s.Version = storeVersion
	if s.Files == nil {
		s.Files = map[string]Entry{}
	}
	key, err := ensureKey(s.keyPath())
	if err != nil {
		return err
	}
	for path, entry := range s.Files {
		entry.MAC = entryMAC(key, path, entry)
		s.Files[path] = entry
	}
	out, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal exec-trust store: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename %s -> %s: %w", tmp, s.path, err)
	}
	return nil
}

// Digest hashes the exec values a config file carries. parts are
// "<key path>=<value>" strings; order does not matter (they are sorted here)
// so callers need not stabilize their walk.
//
// The digest is what binds a trust decision to reviewed content: any added,
// removed, or edited exec value produces a different digest and re-closes the
// gate.
func Digest(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	sorted := append([]string(nil), parts...)
	sort.Strings(sorted)
	h := sha256.New()
	for _, p := range sorted {
		h.Write([]byte(p))
		h.Write([]byte{0}) // NUL-separate so "a"+"bc" != "ab"+"c"
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// canonical normalizes a config file path into a store key. Symlinks are
// resolved so the same file trusted through two paths is one record.
func canonical(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return abs
}
