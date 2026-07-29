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
package exectrust

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/grovetools/core/pkg/paths"
)

// storeVersion is the on-disk schema version. A file carrying a newer version
// is treated as empty (fail closed: nothing is trusted) rather than being
// reinterpreted or clobbered.
const storeVersion = 1

// EnvStorePath overrides the store location. Set by tests and by sandboxed
// runs that must not touch the user's real trust decisions.
const EnvStorePath = "GROVE_EXEC_TRUST_FILE"

// Entry is one trusted config file's record.
type Entry struct {
	// Digest is the exec-value digest that was reviewed (see Digest).
	Digest string `json:"digest"`
	// TrustedAt is an RFC3339 timestamp, for `grove config trust --list`.
	TrustedAt string `json:"trusted_at"`
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

// Load reads the trust store. A missing, unreadable, malformed, or
// future-versioned file yields an empty store and no error: an unusable store
// must degrade to "nothing is trusted", never to "everything is trusted", and
// never block a config load.
func Load() *Store {
	s := &Store{Version: storeVersion, Files: map[string]Entry{}, path: StorePath()}
	if s.path == "" {
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
	if onDisk.Files != nil {
		s.Files = onDisk.Files
	}
	return s
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
// decision record.
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
