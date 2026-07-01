// Package claudetrust pre-seeds Claude Code's per-path folder-trust state so
// agents launched inside a freshly-created grove worktree don't stall at the
// interactive folder-trust prompt.
//
// Trust lives per-exact-path in ~/.claude.json under
// projects["<abs>"].hasTrustDialogAccepted = true. There is no subtree
// inheritance and no CLI skip flag, so grove writes the key directly at
// worktree creation. The file is large and user-owned, so it is treated as an
// opaque map[string]any: unknown keys (top-level and per-project) are preserved
// verbatim, and a malformed file is never overwritten.
//
// This package is a leaf: it must NOT import core/pkg/workspace (which imports
// it transitively via prepare.go), to avoid an import cycle.
package claudetrust

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// trustEnvVar gates seeding. When set to "0", "false", or "off" the seeder is a
// no-op and leaves ~/.claude.json untouched. Default (unset or anything else)
// is ON.
const trustEnvVar = "GROVE_PRESEED_CLAUDE_TRUST"

// trustKey is the per-project field Claude Code checks before prompting.
const trustKey = "hasTrustDialogAccepted"

// SeedTrust marks each of paths as folder-trusted in the user's ~/.claude.json
// so Claude Code does not prompt when an agent is launched there.
//
// Paths should already be canonicalized by the caller (see
// pathutil.CanonicalPath) so the key matches the cwd Claude actually compares
// against. SeedTrust does not canonicalize: it has no notion of which path form
// the launcher uses, and re-resolving here could diverge from the caller.
//
// Behavior:
//   - Gate off (GROVE_PRESEED_CLAUDE_TRUST in {0,false,off}) -> no-op, nil.
//   - No paths -> no-op, nil.
//   - Missing ~/.claude.json -> created with a minimal {"projects":{}} base.
//   - Malformed JSON -> returns an error WITHOUT touching the file (the caller
//     warns; we must never clobber an unparseable user-owned file).
//   - Unknown top-level keys and unrelated/other per-project fields are
//     preserved verbatim.
//
// The write is atomic (tmp file + rename), modeled on
// core/pkg/worktreeregistry/io.go, and preserves the existing file mode (0600
// for a fresh file, since the trust file can carry credentials).
func SeedTrust(paths ...string) error {
	switch os.Getenv(trustEnvVar) {
	case "0", "false", "off":
		return nil
	}
	if len(paths) == 0 {
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("locate home dir: %w", err)
	}
	configPath := filepath.Join(home, ".claude.json")

	// Default file mode for a freshly created trust file. The trust file can
	// hold OAuth/account data, so keep it owner-only.
	mode := os.FileMode(0o600)

	root := map[string]any{}
	data, readErr := os.ReadFile(configPath)
	switch {
	case readErr == nil:
		if info, statErr := os.Stat(configPath); statErr == nil {
			mode = info.Mode().Perm()
		}
		if uerr := json.Unmarshal(data, &root); uerr != nil {
			// Never overwrite a file we can't parse. Warn-and-continue is the
			// caller's job; here we just signal failure.
			return fmt.Errorf("parse %s: %w", configPath, uerr)
		}
	case os.IsNotExist(readErr):
		// Fresh file: start from a minimal base.
	default:
		return fmt.Errorf("read %s: %w", configPath, readErr)
	}

	// Ensure projects is a map[string]any, preserving any existing entries.
	projects, ok := root["projects"].(map[string]any)
	if !ok {
		projects = map[string]any{}
		root["projects"] = projects
	}

	for _, p := range paths {
		entry, ok := projects[p].(map[string]any)
		if !ok {
			entry = map[string]any{}
			projects[p] = entry
		}
		entry[trustKey] = true
	}

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", configPath, err)
	}

	tmpPath := configPath + ".tmp"
	if err := os.WriteFile(tmpPath, out, mode); err != nil {
		return fmt.Errorf("write tmp %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, configPath); err != nil {
		_ = os.Remove(tmpPath) // best-effort cleanup of orphaned tmp
		return fmt.Errorf("rename %s -> %s: %w", tmpPath, configPath, err)
	}
	return nil
}

// PruneOrphanTrust garbage-collects folder-trust entries for grove worktrees
// that no longer exist on disk. SeedTrust is write-only — it adds a trust key
// for each new worktree's container + member-repo subdirs — so without a sweep,
// every finished worktree leaves a dead projects[] entry in ~/.claude.json
// forever (unbounded growth in a large, user-owned file).
//
// At prune time the worktree dir is already gone, so its canonicalized seeded
// key form can no longer be recomputed to match precisely. Instead this does a
// GC sweep: any projects[] key that lives under worktreesDir AND no longer
// exists on disk is removed. That is robust and needs no path-form matching.
//
// Contract mirrors SeedTrust exactly:
//   - Gate off (GROVE_PRESEED_CLAUDE_TRUST in {0,false,off}) -> no-op, nil.
//   - Missing ~/.claude.json, or absent/empty projects -> no-op, nil (nothing
//     to prune; unlike SeedTrust we never create the file here).
//   - Malformed JSON -> returns an error WITHOUT touching the file.
//   - The write is atomic (tmp + rename) and preserves the existing file mode.
//
// Preserved: every live (still-existing) path, every projects[] key OUTSIDE
// worktreesDir (never prune non-grove-managed trust), unrelated per-project
// fields, and all unknown top-level keys. The file is only rewritten when at
// least one orphan key is actually removed.
func PruneOrphanTrust(worktreesDir string) error {
	switch os.Getenv(trustEnvVar) {
	case "0", "false", "off":
		return nil
	}
	if worktreesDir == "" {
		return nil
	}
	worktreesDir = filepath.Clean(worktreesDir)

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("locate home dir: %w", err)
	}
	configPath := filepath.Join(home, ".claude.json")

	data, readErr := os.ReadFile(configPath)
	switch {
	case readErr == nil:
		// Parse below.
	case os.IsNotExist(readErr):
		return nil // No file, nothing to prune.
	default:
		return fmt.Errorf("read %s: %w", configPath, readErr)
	}

	mode := os.FileMode(0o600)
	if info, statErr := os.Stat(configPath); statErr == nil {
		mode = info.Mode().Perm()
	}

	root := map[string]any{}
	if uerr := json.Unmarshal(data, &root); uerr != nil {
		// Never overwrite a file we can't parse.
		return fmt.Errorf("parse %s: %w", configPath, uerr)
	}

	projects, ok := root["projects"].(map[string]any)
	if !ok || len(projects) == 0 {
		return nil // No projects, nothing to prune.
	}

	prefix := worktreesDir + string(os.PathSeparator)
	removed := false
	for key := range projects {
		if key != worktreesDir && !strings.HasPrefix(key, prefix) {
			continue // Outside worktreesDir — never prune non-grove trust.
		}
		if _, statErr := os.Stat(key); os.IsNotExist(statErr) {
			delete(projects, key)
			removed = true
		}
	}
	if !removed {
		return nil // Nothing changed; leave the file byte-for-byte untouched.
	}

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", configPath, err)
	}

	tmpPath := configPath + ".tmp"
	if err := os.WriteFile(tmpPath, out, mode); err != nil {
		return fmt.Errorf("write tmp %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, configPath); err != nil {
		_ = os.Remove(tmpPath) // best-effort cleanup of orphaned tmp
		return fmt.Errorf("rename %s -> %s: %w", tmpPath, configPath, err)
	}
	return nil
}

// IsPermissionDenied reports whether err is the OS-sandbox rejection that
// SeedTrust returns when ~/.claude.json (outside the sandbox's writable
// boundary) cannot be written. It is the signal callers use to decide whether
// to delegate the privileged write to the unsandboxed daemon.
//
// We check both os.IsPermission (EACCES/EPERM unwrapped via errors.Is) AND the
// "operation not permitted" substring, because the sandbox's seatbelt denial
// surfaces as EPERM whose text is "operation not permitted" — and some wrap
// layers can obscure the errno from os.IsPermission while preserving the text.
func IsPermissionDenied(err error) bool {
	if err == nil {
		return false
	}
	return os.IsPermission(err) || strings.Contains(err.Error(), "operation not permitted")
}
