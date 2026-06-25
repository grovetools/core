// Package claudenotebook seeds a grove worktree's Claude Code
// .claude/settings.local.json with the union of every member repo's paired
// notebook directory, so flow agents launched inside the worktree can READ
// (briefings, plans, concepts) and WRITE (.artifacts/, token usage, plan
// updates) the out-of-tree notebooks without a permission prompt or a sandbox
// boundary violation.
//
// Two distinct walls motivate the two settings keys this package writes:
//
//   - permissions.additionalDirectories — grants no-prompt READ access to dirs
//     OUTSIDE the working tree. Without it, every out-of-tree notebook read
//     prompts in default permission mode.
//   - sandbox.filesystem.allowWrite — when /sandbox is enabled the OS-level
//     writable boundary is (working dir + temp) only; the paired notebook lives
//     outside it, so writes fail. Adding the notebook roots here extends the
//     writable boundary to cover them.
//
// This package is a deliberate LEAF (mirrors core/pkg/claudetrust): it does NOT
// import core/pkg/workspace (workspace imports IT, via the union resolver in
// workspace/claude_notebook.go). It takes a pre-resolved, pre-deduped list of
// absolute notebook directories and performs a non-destructive, additive merge
// into settings.local.json — preserving every existing key and user-added
// entry, mirroring the mergeHooks pattern in hooks/commands/install.go.
package claudenotebook

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// seedEnvVar gates seeding. When set to "0", "false", or "off" the seeder is a
// no-op and leaves settings.local.json untouched. Default (unset or anything
// else) is ON. Mirrors claudetrust's GROVE_PRESEED_CLAUDE_TRUST gate.
const seedEnvVar = "GROVE_SEED_CLAUDE_NOTEBOOK_DIRS"

// SeedNotebookDirs merges the given absolute notebook directories into the
// worktree's .claude/settings.local.json under BOTH:
//
//   - permissions.additionalDirectories (no-prompt out-of-tree reads), and
//   - sandbox.filesystem.allowWrite (OS-level sandbox writes).
//
// The merge is additive and non-destructive: existing keys (including
// user-added directories and unrelated settings) are preserved verbatim; only
// missing notebook dirs are appended, and each array is de-duplicated. The
// write is atomic (tmp file + rename), modeled on
// core/pkg/worktreeregistry/io.go.
//
// Behavior:
//   - Gate off (GROVE_SEED_CLAUDE_NOTEBOOK_DIRS in {0,false,off}) -> no-op, nil.
//   - No dirs -> no-op, nil.
//   - Missing .claude/ dir -> created (0755).
//   - Missing settings.local.json -> created from an empty object.
//   - Malformed JSON -> returns an error WITHOUT touching the file (the caller
//     warns; we must never clobber an unparseable user-owned file).
//   - Unknown top-level keys and unrelated nested fields are preserved verbatim.
func SeedNotebookDirs(worktreePath string, dirs []string) error {
	switch os.Getenv(seedEnvVar) {
	case "0", "false", "off":
		return nil
	}
	dirs = dedupeNonEmpty(dirs)
	if len(dirs) == 0 {
		return nil
	}

	claudeDir := filepath.Join(worktreePath, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		return fmt.Errorf("create .claude dir %s: %w", claudeDir, err)
	}
	settingsPath := filepath.Join(claudeDir, "settings.local.json")

	// Default mode for a freshly created local-settings file. This file is
	// gitignored but not secret, so the conventional 0644 is fine.
	mode := os.FileMode(0o644)

	root := map[string]any{}
	data, readErr := os.ReadFile(settingsPath)
	switch {
	case readErr == nil:
		if info, statErr := os.Stat(settingsPath); statErr == nil {
			mode = info.Mode().Perm()
		}
		// An empty or whitespace-only file is treated as an empty object so a
		// touch-created stub doesn't fail the parse.
		if len(trimSpace(data)) == 0 {
			root = map[string]any{}
		} else if uerr := json.Unmarshal(data, &root); uerr != nil {
			// Never overwrite a file we can't parse.
			return fmt.Errorf("parse %s: %w", settingsPath, uerr)
		}
	case os.IsNotExist(readErr):
		// Fresh file: start from an empty object.
	default:
		return fmt.Errorf("read %s: %w", settingsPath, readErr)
	}

	mergeStringArray(root, []string{"permissions", "additionalDirectories"}, dirs)
	mergeStringArray(root, []string{"sandbox", "filesystem", "allowWrite"}, dirs)

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", settingsPath, err)
	}
	out = append(out, '\n')

	tmpPath := settingsPath + ".tmp"
	if err := os.WriteFile(tmpPath, out, mode); err != nil {
		return fmt.Errorf("write tmp %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, settingsPath); err != nil {
		_ = os.Remove(tmpPath) // best-effort cleanup of orphaned tmp
		return fmt.Errorf("rename %s -> %s: %w", tmpPath, settingsPath, err)
	}
	return nil
}

// mergeStringArray walks/creates the nested object path in root and appends any
// of values that are not already present in the string array living at the leaf
// key, preserving existing entries and their order. A non-object intermediate
// or a non-array (or mixed) leaf is replaced with a fresh array of values — the
// security-relevant keys are arrays of path strings by contract, so a malformed
// shape is corrected rather than honored.
func mergeStringArray(root map[string]any, path []string, values []string) {
	// Descend to the parent object of the leaf key, creating objects as needed.
	parent := root
	for _, key := range path[:len(path)-1] {
		child, ok := parent[key].(map[string]any)
		if !ok {
			child = map[string]any{}
			parent[key] = child
		}
		parent = child
	}
	leafKey := path[len(path)-1]

	// Collect existing string entries (in order), tracking presence.
	var existing []string
	present := map[string]struct{}{}
	if raw, ok := parent[leafKey].([]any); ok {
		for _, item := range raw {
			if s, ok := item.(string); ok {
				if _, dup := present[s]; !dup {
					existing = append(existing, s)
					present[s] = struct{}{}
				}
			}
		}
	}

	// Append only the missing values, preserving caller order.
	for _, v := range values {
		if _, ok := present[v]; ok {
			continue
		}
		existing = append(existing, v)
		present[v] = struct{}{}
	}

	// Store back as []any so it round-trips identically to a parsed JSON array.
	merged := make([]any, len(existing))
	for i, s := range existing {
		merged[i] = s
	}
	parent[leafKey] = merged
}

// dedupeNonEmpty returns the sorted, de-duplicated, non-empty subset of in.
// Sorting yields a deterministic on-disk order regardless of member-repo
// discovery order.
func dedupeNonEmpty(in []string) []string {
	set := map[string]struct{}{}
	for _, s := range in {
		if s == "" {
			continue
		}
		set[s] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// trimSpace reports the input with leading/trailing ASCII whitespace removed,
// used only to detect an effectively-empty settings file without pulling in the
// strings package for a one-liner.
func trimSpace(b []byte) []byte {
	start := 0
	for start < len(b) && isSpace(b[start]) {
		start++
	}
	end := len(b)
	for end > start && isSpace(b[end-1]) {
		end--
	}
	return b[start:end]
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}
