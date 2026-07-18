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
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/grovetools/core/pkg/paths"
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

// unlockConfigEnvVar is the dev escape hatch for the protectConfig toggle. When
// set to "1" at seed time, the seeder treats protection as off for this launch:
// it strips any grove-owned self-protection entries instead of writing them, so
// an explicitly-unlocked agent can edit grove config. It lives OUTSIDE the
// protected files by design — you cannot edit a locked grove.toml to unlock it.
// flow injects this into dispatched-agent launches via its AgentEnv mechanism.
const unlockConfigEnvVar = "GROVE_UNLOCK_CONFIG"

// debugEnvVar gates verbose, human-readable tracing of the seeding path to
// stderr. It is OFF by default (this is a leaf package that cannot import
// core/logging without a cycle: core/logging -> core/pkg/workspace ->
// claudenotebook). Set GROVE_CLAUDE_SETTINGS_DEBUG to 1/true/on to watch which
// worktree/root is being seeded, with which member repos, to which file, and
// whether a write actually happened — the observability surface the
// ecosystem-root seeding investigation needs.
const debugEnvVar = "GROVE_CLAUDE_SETTINGS_DEBUG"

// Debugf writes a gated trace line to stderr when GROVE_CLAUDE_SETTINGS_DEBUG is
// truthy. It is exported so the workspace-level resolver
// (workspace.SeedClaudeSettingsForWorktree) can share the same gate and prefix.
func Debugf(format string, args ...any) {
	switch os.Getenv(debugEnvVar) {
	case "1", "true", "on":
		fmt.Fprintf(os.Stderr, "[claude-settings] "+format+"\n", args...)
	}
}

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
	// Delegate to SeedSettings with nil config (notebook dirs only). No repos:
	// the local single-repo path has no ecosystem members to protect, and a nil
	// config means the self-protection toggle never fires here anyway.
	return SeedSettings(worktreePath, nil, nil, dirs)
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
func SeedSettings(worktreePath string, repos []string, cfg *ClaudeConfig, notebookDirs []string) error {
	_, err := SeedSettingsChanged(worktreePath, repos, cfg, notebookDirs)
	return err
}

// SeedSettingsChanged is SeedSettings but additionally reports whether the
// settings file was actually (re)written. When the merged output is
// byte-identical to what is already on disk the write is skipped entirely and
// changed=false is returned — this is what keeps timer-driven reconciles
// (daemon SettingsHandler) from rewriting every worktree's settings file on
// every pass.
func SeedSettingsChanged(worktreePath string, repos []string, cfg *ClaudeConfig, notebookDirs []string) (changed bool, err error) {
	notebookDirs = dedupeNonEmpty(notebookDirs)

	// Check if there's anything to do.
	settingsGateOff := isGateOff(seedSettingsEnvVar)
	allowGroveTools := cfg != nil && cfg.AllowGroveTools != nil && *cfg.AllowGroveTools
	// ShouldSeed widens the bare IsEmpty() check to honor the lone-flag signals
	// (allowGroveTools and protectConfig) that deliberately live outside IsEmpty.
	// Without it a config whose only signal is protectConfig (true OR false) would
	// be treated as empty and skipped, so the lock would never write and the
	// strip-on-false could never fire.
	hasConfig := cfg.ShouldSeed() && !settingsGateOff
	hasDirs := len(notebookDirs) > 0

	if !hasConfig && !hasDirs {
		Debugf("SeedSettings SKIP (nothing to seed): path=%s repos=%v hasConfig=%v hasDirs=%v settingsGateOff=%v",
			worktreePath, repos, hasConfig, hasDirs, settingsGateOff)
		return false, nil
	}

	claudeDir := filepath.Join(worktreePath, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		return false, fmt.Errorf("create .claude dir %s: %w", claudeDir, err)
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
			return false, fmt.Errorf("parse %s: %w", settingsPath, uerr)
		}
	case os.IsNotExist(readErr):
		// Fresh file: start from an empty object.
	default:
		return false, fmt.Errorf("read %s: %w", settingsPath, readErr)
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

	// Auto-add the canonicalized worktree root to the sandbox writable boundary.
	// This is the write-side complement to the notebook-dir allowWrite merge
	// above: additionalDirectories/notebook-dir entries extend the boundary OUT
	// to out-of-tree notebooks, but the agent's OWN repo tree is only writable
	// under the sandbox if it is in allowWrite too. Claude Code's default sandbox
	// boundary makes only the literal cwd node + session temp writable — NOT the
	// repo subtree — so a sandboxed agent in a primary checkout (e.g.
	// ~/Code/<ecosystem>) cannot Bash-write its own member subdirs (core/, hooks/,
	// …): gofumpt -w, go build, sed -i, and tmp+rename all fail with "operation
	// not permitted". allowWrite is RECURSIVE (a directory entry covers its whole
	// subtree — see grove-anthropic/pkg/ccsettings ComputeFilesystemBoundary +
	// schema allowWrite description), so this single root entry covers every
	// member subdir. XDG worktrees under ~/.local/share/grove/worktrees are
	// already covered by that broad allowWrite prefix; the gap this closes is the
	// primary checkout, which has no covering prefix. Fires whenever settings are
	// seeded for a worktree, mirroring the notebook-dir merge. It is scoped to the
	// SPECIFIC worktree path (recursive), never a broad ~/Code. Canonicalize like
	// wtForEdit so the entry matches the path Claude resolves (symlinks + macOS
	// case). denyWrite TAKES PRECEDENCE over allowWrite (schema:
	// sandbox.filesystem.denyWrite), so the grove-owned denyWrite entries that
	// protectConfig adds below for grove.toml / member configs still win even
	// though those files live inside this worktree-root subtree — adding the root
	// here does NOT un-protect them. Order is irrelevant: precedence is evaluated
	// at runtime, not by array position.
	wtForWrite := worktreePath
	if canon, err := pathutil.CanonicalPath(worktreePath); err == nil {
		wtForWrite = canon
	}
	mergeStringArray(root, []string{"sandbox", "filesystem", "allowWrite"}, []string{wtForWrite})

	// Default build-cache allowWrite entries, riding the same always-on merge as
	// the worktree root above: the Go module cache (~/go/pkg/mod, honoring
	// GOPATH) and the npm cache (~/.npm) live in $HOME, OUTSIDE every worktree,
	// so under an enabled sandbox ordinary builds fail hard — `go build` cannot
	// populate ~/go/pkg/mod/cache/download ("operation not permitted") and npm
	// installs cannot write ~/.npm. Absolute paths, matching the worktree-root /
	// notebook-dir convention; harmless when the sandbox is off; additive and
	// dedup-safe via mergeStringArray like every other allowWrite entry.
	mergeStringArray(root, []string{"sandbox", "filesystem", "allowWrite"}, defaultCacheAllowWrite())

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

		// autoMode classifier sections. Written ADDITIVELY per-section via
		// mergeStringArray at the snake_case JSON keys Claude requires
		// (soft_deny/hard_deny), only when non-empty — this preserves any
		// user-authored entries (including "$defaults") and appends grove's, same as
		// permissions.allow.
		//
		// Semantics note: Claude's autoMode arrays REPLACE the built-in section
		// unless "$defaults" is present. Grove writes additively (its philosophy for
		// permission arrays), so a grove-managed autoMode.allow fully replaces
		// Claude's built-in allow list from Claude's POV — if the user wants the
		// built-ins too, they add "$defaults" to the grove.toml array and grove
		// passes it through verbatim.
		if am := cfg.AutoMode; !am.isEmpty() {
			if len(am.Allow) > 0 {
				mergeStringArray(root, []string{"autoMode", "allow"}, am.Allow)
			}
			if len(am.SoftDeny) > 0 {
				mergeStringArray(root, []string{"autoMode", "soft_deny"}, am.SoftDeny)
			}
			if len(am.Environment) > 0 {
				mergeStringArray(root, []string{"autoMode", "environment"}, am.Environment)
			}
			if len(am.HardDeny) > 0 {
				mergeStringArray(root, []string{"autoMode", "hard_deny"}, am.HardDeny)
			}
		}

		// useAutoModeDuringPlan (top-level bool). nil = no-op; explicit value
		// OVERWRITES like the other bools. Grove does not enforce Claude's "no effect
		// unless defaultMode allows auto" cross-field rule.
		mergeBool(root, []string{"useAutoModeDuringPlan"}, cfg.UseAutoModeDuringPlan)

		// Top-level scalar passthroughs (model / effortLevel / editorMode / tui):
		// only written when non-empty so an unset field never clobbers a user's
		// existing value; an explicit grove.toml value OVERWRITES, same as
		// defaultMode.
		mergeString(root, []string{"model"}, cfg.Model)
		mergeString(root, []string{"effortLevel"}, cfg.EffortLevel)
		mergeString(root, []string{"editorMode"}, cfg.EditorMode)
		mergeString(root, []string{"tui"}, cfg.TUI)

		// Top-level bools (skip prompts / push notifications). nil = no-op;
		// explicit values OVERWRITE like the sandbox bools.
		mergeBool(root, []string{"skipDangerousModePermissionPrompt"}, cfg.SkipDangerousModePermissionPrompt)
		mergeBool(root, []string{"skipWorkflowUsageWarning"}, cfg.SkipWorkflowUsageWarning)
		mergeBool(root, []string{"agentPushNotifEnabled"}, cfg.AgentPushNotifEnabled)

		// enabledPlugins (top-level object, plugin id -> bool). Merged per key:
		// user-added plugin entries are preserved, grove.toml wins on the keys it
		// sets — the map analog of mergeStringArray + mergeBool.
		mergeBoolMap(root, []string{"enabledPlugins"}, cfg.EnabledPlugins)

		// sandbox booleans (only write if non-nil)
		mergeBool(root, []string{"sandbox", "enabled"}, cfg.Sandbox.Enabled)
		mergeBool(root, []string{"sandbox", "failIfUnavailable"}, cfg.Sandbox.FailIfUnavailable)
		mergeBool(root, []string{"sandbox", "autoAllowBashIfSandboxed"}, cfg.Sandbox.AutoAllowBashIfSandboxed)
		// sandbox.allowUnsandboxedCommands: the escape-hatch lock. nil = no-op;
		// explicit value OVERWRITES (grove.toml wins), same as the sandbox bools
		// above. Its intended value is false, and false MUST land as literal JSON
		// false — mergeBool writes *val verbatim, so an explicit false survives.
		mergeBool(root, []string{"sandbox", "allowUnsandboxedCommands"}, cfg.Sandbox.AllowUnsandboxedCommands)

		// sandbox.filesystem.allowWrite (from config, merged with notebook dirs)
		if len(cfg.Sandbox.Filesystem.AllowWrite) > 0 {
			mergeStringArray(root, []string{"sandbox", "filesystem", "allowWrite"}, cfg.Sandbox.Filesystem.AllowWrite)
		}

		// sandbox.excludedCommands: the vetted unsandboxed allowlist (additive
		// union, preserves user entries), next to allowedDomains. Only write when
		// non-empty.
		if len(cfg.Sandbox.ExcludedCommands) > 0 {
			mergeStringArray(root, []string{"sandbox", "excludedCommands"}, cfg.Sandbox.ExcludedCommands)
		}

		// sandbox.network.allowedDomains (config domains plus the default npm
		// registry). Whenever a sandbox network section is being emitted — grove
		// manages the sandbox (enabled set, true OR false) or the config already
		// carries an allowlist — registry.npmjs.org is added so npm installs
		// work under the sandbox's network boundary. Additive and dedup-safe
		// like every other domain entry.
		domains := append([]string(nil), cfg.Sandbox.Network.AllowedDomains...)
		if cfg.Sandbox.Enabled != nil || len(domains) > 0 {
			domains = append(domains, "registry.npmjs.org")
		}
		if len(domains) > 0 {
			mergeStringArray(root, []string{"sandbox", "network", "allowedDomains"}, domains)
		}

		// sandbox.network.allowUnixSockets (connect-only socket paths; unioned
		// like allowedDomains)
		if len(cfg.Sandbox.Network.AllowUnixSockets) > 0 {
			mergeStringArray(root, []string{"sandbox", "network", "allowUnixSockets"}, cfg.Sandbox.Network.AllowUnixSockets)
		}

		// sandbox.network socket/local-bind booleans (only write if non-nil;
		// OVERWRITE like the other sandbox bools — grove.toml wins)
		mergeBool(root, []string{"sandbox", "network", "allowAllUnixSockets"}, cfg.Sandbox.Network.AllowAllUnixSockets)
		mergeBool(root, []string{"sandbox", "network", "allowLocalBinding"}, cfg.Sandbox.Network.AllowLocalBinding)

		// Config self-protection: deny writes to the grove config files that
		// govern this worktree's sandbox/permissions, so a sandboxed agent can't
		// edit the config that sandboxes it. The two layers cover the two seams:
		// sandbox.filesystem.denyWrite is the OS-enforced block for shell writes
		// (bypass-proof when sandbox.enabled), and permissions.deny Edit/Write
		// rules best-effort cover the native-tool seam. We compute the grove-owned
		// entries and either ADD them (protect) or actively STRIP them (toggle off
		// / unlocked), then re-merge the user's own deny arrays so user-authored
		// entries are always preserved — including any that happen to match a
		// grove-owned path.
		unlocked := os.Getenv(unlockConfigEnvVar) == "1"
		want := cfg.ProtectConfig != nil && *cfg.ProtectConfig
		protect := want && !unlocked
		// Strip whenever an explicit toggle is present but protection is not in
		// effect (explicit false, or true-but-unlocked-for-this-launch), so a
		// previously written lock is reversible. Unset (nil) touches nothing.
		strip := cfg.ProtectConfig != nil && !protect

		if protect || strip {
			protPaths := protectedConfigPaths(worktreePath, repos)
			denyWritePaths := make([]string, 0, len(protPaths))
			denyRules := make([]string, 0, len(protPaths)*3)
			for _, p := range protPaths {
				denyWritePaths = append(denyWritePaths, p.path)
				denyRules = append(denyRules, denyRulesForPath(p)...)
			}
			if protect {
				mergeStringArray(root, []string{"sandbox", "filesystem", "denyWrite"}, denyWritePaths)
				mergeStringArray(root, []string{"permissions", "deny"}, denyRules)
				if cfg.Sandbox.Enabled == nil || !*cfg.Sandbox.Enabled {
					fmt.Fprintln(os.Stderr, "Warning: [claude] protectConfig is enabled but sandbox is disabled. "+
						"Config protection is relying solely on native-tool permissions (permissions.deny), "+
						"which agents can bypass via shell commands. Set [claude.sandbox] enabled = true for the OS-enforced block.")
				}
			} else {
				removeFromStringArray(root, []string{"sandbox", "filesystem", "denyWrite"}, denyWritePaths)
				removeFromStringArray(root, []string{"permissions", "deny"}, denyRules)
			}
		}

		// User-authored deny arrays are merged AFTER the protection add/strip so a
		// user entry is always present, even one that coincides with a grove-owned
		// path that strip just removed. These are normal additive config arrays.
		if len(cfg.Permissions.Deny) > 0 {
			mergeStringArray(root, []string{"permissions", "deny"}, cfg.Permissions.Deny)
		}
		if len(cfg.Sandbox.Filesystem.DenyWrite) > 0 {
			mergeStringArray(root, []string{"sandbox", "filesystem", "denyWrite"}, cfg.Sandbox.Filesystem.DenyWrite)
		}
		// sandbox.filesystem.denyRead (read-side complement to denyWrite; a plain
		// additive config array, not part of the protectConfig add/strip).
		if len(cfg.Sandbox.Filesystem.DenyRead) > 0 {
			mergeStringArray(root, []string{"sandbox", "filesystem", "denyRead"}, cfg.Sandbox.Filesystem.DenyRead)
		}
	}

	out, merr := json.MarshalIndent(root, "", "  ")
	if merr != nil {
		return false, fmt.Errorf("marshal %s: %w", settingsPath, merr)
	}
	out = append(out, '\n')

	// No-op skip: when the merged output is byte-identical to what was read
	// from disk, skip the tmp+rename entirely. This kills the steady-state
	// write churn (and most of the tmp-rename race with concurrent worktree
	// teardown) caused by reconciles that change nothing.
	if readErr == nil && bytes.Equal(out, data) {
		Debugf("SeedSettings NO-OP (unchanged) %s (repos=%v hasConfig=%v hasDirs=%v)", settingsPath, repos, hasConfig, hasDirs)
		return false, nil
	}

	tmpPath := settingsPath + ".tmp"
	if err := os.WriteFile(tmpPath, out, mode); err != nil {
		return false, fmt.Errorf("write tmp %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, settingsPath); err != nil {
		_ = os.Remove(tmpPath) // best-effort cleanup of orphaned tmp
		return false, fmt.Errorf("rename %s -> %s: %w", tmpPath, settingsPath, err)
	}
	Debugf("SeedSettings WROTE %s (repos=%v hasConfig=%v hasDirs=%v)", settingsPath, repos, hasConfig, hasDirs)
	return true, nil
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

// protectedPath is a single config-file or config-directory target of the
// self-protection toggle. isDir distinguishes the global config directory
// (~/.config/grove), whose deny rules need a /** subtree glob to cover the files
// inside it, from the individual config FILE paths (grove.toml/.yml/.yaml),
// which match the file exactly with no glob.
type protectedPath struct {
	path  string
	isDir bool
}

// configFileNames are the grove config filenames protected at each repo/worktree
// root. grove.toml is the canonical one; the .yml/.yaml variants are included so
// switching format can't sidestep the lock. Non-existent variants are harmless:
// a deny rule for a path that doesn't exist yet simply blocks creating it there.
var configFileNames = []string{"grove.toml", "grove.yml", "grove.yaml"}

// protectedConfigPaths returns the canonicalized set of config paths the
// self-protection toggle locks: the global grove config dir, the worktree-root
// grove config files, and each member repo's grove config files. The worktree
// root is canonicalized once (resolving symlinks + macOS case) and the config
// filenames joined onto it, so the rules match the path Claude actually compares
// against even for files that don't exist yet (CanonicalPath of a non-existent
// leaf is unreliable; a canonical existing-dir prefix is not).
func protectedConfigPaths(worktreePath string, repos []string) []protectedPath {
	var out []protectedPath
	seen := map[string]struct{}{}
	add := func(p string, isDir bool) {
		if p == "" {
			return
		}
		if _, dup := seen[p]; dup {
			return
		}
		seen[p] = struct{}{}
		out = append(out, protectedPath{path: p, isDir: isDir})
	}

	// Global config directory. Resolved via paths.ConfigDir() (GROVE_HOME →
	// XDG_CONFIG_HOME → ~/.config), the SAME resolver grove uses to load its
	// global grove.yml — so we protect the dir grove actually reads, and a
	// sandboxed XDG_CONFIG_HOME (e.g. tend e2e) resolves inside the sandbox
	// instead of the developer's real ~/.config/grove.
	if cfgDir := paths.ConfigDir(); cfgDir != "" {
		if canon, cerr := pathutil.CanonicalPath(cfgDir); cerr == nil {
			add(canon, true)
		} else {
			add(cfgDir, true)
		}
	}

	// Canonical worktree root prefix; fall back to the raw path if it can't be
	// canonicalized (it almost always exists at seed time).
	canonWt := worktreePath
	if canon, err := pathutil.CanonicalPath(worktreePath); err == nil {
		canonWt = canon
	}

	for _, name := range configFileNames {
		add(filepath.Join(canonWt, name), false)
	}
	for _, repo := range repos {
		if repo == "" {
			continue
		}
		for _, name := range configFileNames {
			add(filepath.Join(canonWt, repo, name), false)
		}
	}
	return out
}

// denyRulesForPath returns the permissions.deny rules (Edit/Write/MultiEdit) for
// a single protected path, mirroring editRuleForAbsDir's anchor convention: the
// leading "//" absolute anchor with the leading slash stripped. A directory uses
// the "/**" subtree glob so edits to files INSIDE it are denied; a file matches
// exactly with no glob.
func denyRulesForPath(p protectedPath) []string {
	anchored := "//" + strings.TrimPrefix(filepath.ToSlash(p.path), "/")
	if p.isDir {
		anchored += "/**"
	}
	return []string{
		"Edit(" + anchored + ")",
		"Write(" + anchored + ")",
		"MultiEdit(" + anchored + ")",
	}
}

// defaultCacheAllowWrite returns the build-cache directories every seeded
// settings file gets in sandbox.filesystem.allowWrite: the Go module cache
// (GOPATH/pkg/mod — first GOPATH list entry when set, else ~/go/pkg/mod) and
// the npm cache (~/.npm). Absolute paths, matching the worktree-root /
// notebook-dir convention. Anything unresolvable (no home dir, empty GOPATH)
// is simply omitted.
func defaultCacheAllowWrite() []string {
	home, homeErr := os.UserHomeDir()
	var out []string
	goPath := ""
	if env := os.Getenv("GOPATH"); env != "" {
		if list := filepath.SplitList(env); len(list) > 0 {
			goPath = list[0]
		}
	}
	if goPath == "" && homeErr == nil {
		goPath = filepath.Join(home, "go")
	}
	if goPath != "" {
		out = append(out, filepath.Join(goPath, "pkg", "mod"))
	}
	if homeErr == nil {
		out = append(out, filepath.Join(home, ".npm"))
	}
	return out
}

// groveToolBashRules returns the allow rules for each canonical grove
// ecosystem CLI: a PATH form Bash(<name>:*) plus a worktree-relative built
// form Bash(./<repo>/bin/<name>:*), so an agent can invoke both the installed
// tool and the binary it just built inside the worktree (./flow/bin/flow …)
// without a prompt. The names are the real [binary].name values from the
// ecosystem's grove.toml files — several differ from their repo directory
// names (see groveToolRepoDirs). This list is hardcoded because this is a LEAF
// package that must not import core/config or core/pkg/workspace.
func groveToolBashRules() []string {
	tools := []string{
		"grove", "flow", "cx", "nb", "tend", "groved",
		"grove-anthropic", "grove-gemini", "memory", "nav", "docgen",
		"skills", "aglogs", "grove-env-cloud", "grove-syncd",
		"git-viewer", "treemux", "tuimux", "grove-nvim",
	}
	rules := make([]string, 0, len(tools)*2)
	for _, t := range tools {
		rules = append(rules, "Bash("+t+":*)")
	}
	for _, t := range tools {
		rules = append(rules, "Bash(./"+groveToolRepoDir(t)+"/bin/"+t+":*)")
	}
	return rules
}

// groveToolRepoDirs maps the CLIs whose [binary].name differs from their repo
// directory to that directory (verified against each repo Makefile's
// BINARY_NAME). Tools absent from this map live in a repo named after the tool.
var groveToolRepoDirs = map[string]string{
	"aglogs":          "agentlogs",
	"grove-syncd":     "sync",
	"grove-nvim":      "grove.nvim",
	"grove-env-cloud": "cloud",
	"groved":          "daemon",
}

// groveToolRepoDir returns the ecosystem repo directory that builds the given
// CLI (default: the tool's own name).
func groveToolRepoDir(tool string) string {
	if repo, ok := groveToolRepoDirs[tool]; ok {
		return repo
	}
	return tool
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

// mergeBoolMap walks/creates the nested object path in root and sets each key
// of vals on the leaf object. Only fires when vals is non-empty. Existing leaf
// keys not named in vals are preserved (user plugin entries survive); keys in
// vals are OVERWRITTEN per key, mirroring mergeBool. A non-object leaf is
// replaced with a fresh object, like mergeStringArray's malformed-shape rule.
func mergeBoolMap(root map[string]any, path []string, vals map[string]bool) {
	if len(vals) == 0 {
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
	leafKey := path[len(path)-1]
	leaf, ok := parent[leafKey].(map[string]any)
	if !ok {
		leaf = map[string]any{}
		parent[leafKey] = leaf
	}
	for k, v := range vals {
		leaf[k] = v
	}
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

// removeFromStringArray is the additive-inverse of mergeStringArray: it descends
// the nested object path and, if the leaf array exists, filters out exactly the
// elements equal to one of valuesToRemove, leaving every other element (and its
// order) untouched. This is the strip-on-false mechanism for self-protection —
// it removes only grove-owned entries, never a user's own deny rules. A missing
// path or leaf is a no-op (nothing to remove). Non-string and unmatched elements
// are preserved verbatim. If removal empties the array, the empty array is left
// in place (a present-but-empty key is harmless and avoids resurrecting a stale
// shape).
func removeFromStringArray(root map[string]any, path []string, valuesToRemove []string) {
	if len(valuesToRemove) == 0 {
		return
	}
	// Descend WITHOUT creating: if any intermediate is absent, there is nothing
	// to strip.
	parent := root
	for _, key := range path[:len(path)-1] {
		child, ok := parent[key].(map[string]any)
		if !ok {
			return
		}
		parent = child
	}
	leafKey := path[len(path)-1]
	raw, ok := parent[leafKey].([]any)
	if !ok {
		return
	}

	remove := make(map[string]struct{}, len(valuesToRemove))
	for _, v := range valuesToRemove {
		remove[v] = struct{}{}
	}

	filtered := make([]any, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			if _, drop := remove[s]; drop {
				continue
			}
		}
		filtered = append(filtered, item)
	}
	parent[leafKey] = filtered
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
