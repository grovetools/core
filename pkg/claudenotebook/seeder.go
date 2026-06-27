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
	"strings"

	"github.com/grovetools/core/util/pathutil"
)

// seedNotebookDirsEnvVar gates notebook directory seeding. When set to "0",
// "false", or "off" the seeder is a no-op and leaves settings.local.json
// untouched. Default (unset or anything else) is ON. Mirrors claudetrust's
// GROVE_PRESEED_CLAUDE_TRUST gate.
const seedNotebookDirsEnvVar = "GROVE_SEED_CLAUDE_NOTEBOOK_DIRS"

// seedSettingsEnvVar gates ClaudeConfig seeding. When set to "0", "false", or
// "off" the SeedSettings function is a no-op for ClaudeConfig fields (it still
// seeds notebook dirs if that gate is open). Default (unset or anything else)
// is ON.
const seedSettingsEnvVar = "GROVE_SEED_CLAUDE_SETTINGS"

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
	switch os.Getenv(seedNotebookDirsEnvVar) {
	case "0", "false", "off":
		return nil
	}
	// Delegate to SeedSettings with nil config (notebook dirs only).
	return SeedSettings(worktreePath, nil, dirs)
}

// SeedSettings merges the given ClaudeConfig and notebook directories into the
// worktree's .claude/settings.local.json. This is the generalized entry point
// that handles both:
//
//   - Notebook directories (permissions.additionalDirectories + sandbox.filesystem.allowWrite)
//   - ClaudeConfig fields (permissions.allow, sandbox.*, sandbox.network.allowedDomains)
//
// The merge is additive and non-destructive: existing keys (including
// user-added entries and unrelated settings) are preserved verbatim.
//
// Behavior:
//   - Gate off (GROVE_SEED_CLAUDE_SETTINGS in {0,false,off}) -> ClaudeConfig fields skipped.
//   - No config and no dirs -> no-op, nil.
//   - Missing .claude/ dir -> created (0755).
//   - Missing settings.local.json -> created from an empty object.
//   - Malformed JSON -> returns an error WITHOUT touching the file.
//   - Unknown top-level keys and unrelated nested fields are preserved verbatim.
func SeedSettings(worktreePath string, cfg *ClaudeConfig, notebookDirs []string) error {
	notebookDirs = dedupeNonEmpty(notebookDirs)

	// Check if there's anything to do.
	settingsGateOff := isGateOff(seedSettingsEnvVar)
	// allowGroveTools is a content signal that lives OUTSIDE IsEmpty (a lone
	// flag must not be treated as empty), so the gate is widened to honor it:
	// a config whose only signal is allowGroveTools=true still writes.
	allowGroveTools := cfg != nil && cfg.AllowGroveTools != nil && *cfg.AllowGroveTools
	hasConfig := cfg != nil && (!cfg.IsEmpty() || allowGroveTools) && !settingsGateOff
	hasDirs := len(notebookDirs) > 0

	if !hasConfig && !hasDirs {
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

	// Merge notebook directories (always write to both keys).
	if hasDirs {
		mergeStringArray(root, []string{"permissions", "additionalDirectories"}, notebookDirs)
		mergeStringArray(root, []string{"sandbox", "filesystem", "allowWrite"}, notebookDirs)

		// Auto-derive Edit() allow rules: additionalDirectories grants no-prompt
		// READ, but in default permission mode the Edit/Write tool still prompts
		// on out-of-tree writes. An Edit(//<abs>/**) rule in permissions.allow is
		// the scoped write-permission complement, so it rides this same
		// notebook-dir gate (and emits even when cfg is nil). One rule per
		// notebook dir plus a narrow rule for just this worktree.
		//
		// Note: in local (single-repo) mode a notebook dir can resolve in-tree;
		// an in-tree Edit() rule is harmless and dedup-safe via mergeStringArray.
		editRules := make([]string, 0, len(notebookDirs)+1)
		for _, d := range notebookDirs {
			editRules = append(editRules, editRuleForAbsDir(d))
		}
		// Canonicalize the worktree path (resolve symlinks + macOS case) so the
		// Edit rule matches the cwd Claude actually compares against. The notebook
		// dirs above arrive pre-canonicalized from the resolver, and the trust
		// seeder canonicalizes likewise (prepare.go); an un-canonicalized
		// //var/... rule would silently miss a /private/var/... cwd on macOS.
		wtForEdit := worktreePath
		if canon, err := pathutil.CanonicalPath(worktreePath); err == nil {
			wtForEdit = canon
		}
		editRules = append(editRules, editRuleForAbsDir(wtForEdit))
		mergeStringArray(root, []string{"permissions", "allow"}, editRules)
	}

	// Merge ClaudeConfig fields (only if gate is open and config is non-empty).
	if hasConfig {
		// permissions.allow (config rules plus, when allowGroveTools is set, the
		// expanded canonical grove-tool Bash rules). Additive/dedup-safe via the
		// same mergeStringArray.
		allowRules := append([]string(nil), cfg.Permissions.Allow...)
		if allowGroveTools {
			allowRules = append(allowRules, groveToolBashRules()...)
		}
		if len(allowRules) > 0 {
			mergeStringArray(root, []string{"permissions", "allow"}, allowRules)
		}

		// permissions.defaultMode (scalar string; only write when non-empty so we
		// never clobber a user's existing value with an empty default). Like the
		// sandbox booleans below, an explicit grove.toml value OVERWRITES an
		// existing one (grove.toml wins).
		mergeString(root, []string{"permissions", "defaultMode"}, cfg.Permissions.DefaultMode)

		// sandbox booleans (only write if non-nil)
		mergeBool(root, []string{"sandbox", "enabled"}, cfg.Sandbox.Enabled)
		mergeBool(root, []string{"sandbox", "failIfUnavailable"}, cfg.Sandbox.FailIfUnavailable)
		mergeBool(root, []string{"sandbox", "autoAllowBashIfSandboxed"}, cfg.Sandbox.AutoAllowBashIfSandboxed)

		// sandbox.filesystem.allowWrite (from config, merged with notebook dirs)
		if len(cfg.Sandbox.Filesystem.AllowWrite) > 0 {
			mergeStringArray(root, []string{"sandbox", "filesystem", "allowWrite"}, cfg.Sandbox.Filesystem.AllowWrite)
		}

		// sandbox.network.allowedDomains
		if len(cfg.Sandbox.Network.AllowedDomains) > 0 {
			mergeStringArray(root, []string{"sandbox", "network", "allowedDomains"}, cfg.Sandbox.Network.AllowedDomains)
		}
	}

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

// editRuleForAbsDir returns the Claude Code permission rule that grants
// no-prompt Edit/Write access to everything under the given absolute directory.
// The leading "//" is the mandatory absolute anchor: ccsettings'
// resolveReadEditAnchor (grove-anthropic/pkg/ccsettings/path.go) drops one
// slash so "//Users/x" -> "/Users/x", whereas a single leading "/" would be
// interpreted as project-root-relative (wrong).
func editRuleForAbsDir(absDir string) string {
	return "Edit(//" + strings.TrimPrefix(filepath.ToSlash(absDir), "/") + "/**)"
}

// groveToolBashRules returns a Bash(<name>:*) allow rule for each canonical
// grove ecosystem CLI. These are the real [binary].name values from the
// ecosystem's grove.toml files — several differ from their repo directory
// names (e.g. aglogs, groved). This list is hardcoded because this is a LEAF
// package that must not import core/config or core/pkg/workspace.
func groveToolBashRules() []string {
	tools := []string{
		"grove", "flow", "cx", "nb", "tend", "groved",
		"grove-anthropic", "grove-gemini", "memory", "nav", "docgen",
		"skills", "aglogs", "grove-env-cloud", "grove-syncd",
		"git-viewer", "treemux", "tuimux", "grove-nvim",
	}
	rules := make([]string, len(tools))
	for i, t := range tools {
		rules[i] = "Bash(" + t + ":*)"
	}
	return rules
}

// isGateOff returns true if the given env var is set to "0", "false", or "off".
func isGateOff(envVar string) bool {
	switch os.Getenv(envVar) {
	case "0", "false", "off":
		return true
	}
	return false
}

// mergeBool walks/creates the nested object path in root and sets the leaf key
// to the given boolean value. Only writes if val is non-nil. If the path does
// not exist, it is created. Unlike mergeStringArray, this OVERWRITES an existing
// value (grove.toml booleans win over local settings when explicitly set).
func mergeBool(root map[string]any, path []string, val *bool) {
	if val == nil {
		return
	}
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
	parent[path[len(path)-1]] = *val
}

// mergeString walks/creates the nested object path in root and sets the leaf
// key to the given string value. Only writes if val is non-empty (empty string
// = unset, so a configured value in the file is never clobbered). If the path
// does not exist, it is created. Like mergeBool, this OVERWRITES an existing
// value (grove.toml scalars win over local settings when explicitly set).
func mergeString(root map[string]any, path []string, val string) {
	if val == "" {
		return
	}
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
	parent[path[len(path)-1]] = val
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
