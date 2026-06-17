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
