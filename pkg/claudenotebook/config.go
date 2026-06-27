// Package claudenotebook provides configuration structures and seeding logic
// for Claude Code's settings.local.json.
//
// ClaudeConfig defines the [claude] grove.toml extension, which propagates
// permissions.allow + sandbox{enabled, failIfUnavailable, autoAllowBashIfSandboxed,
// filesystem.allowWrite, network.allowedDomains} into each workspace/worktree's
// .claude/settings.local.json.
//
// This is a LEAF package (like claudetrust): it does NOT import core/config or
// core/pkg/workspace. Call sites use core's UnmarshalExtension("claude", &cfg)
// to extract ClaudeConfig from the loaded grove.toml.
package claudenotebook

// ClaudeConfig defines the [claude] grove.toml extension for Claude Code
// agent settings. It mirrors the FlowConfig pattern in flow/pkg/orchestration/config.go.
//
// Arrays are unioned across member repos when seeding ecosystem worktrees.
// Booleans use pointers to distinguish unset (nil) vs explicit false.
//
// Example grove.toml:
//
//	[claude.permissions]
//	allow = ["Bash(git:*)"]
//
//	[claude.sandbox]
//	enabled = true
//	failIfUnavailable = false
//	autoAllowBashIfSandboxed = true
//
//	[claude.sandbox.filesystem]
//	allowWrite = ["/tmp/myproject"]
//
//	[claude.sandbox.network]
//	allowedDomains = ["api.github.com"]
type ClaudeConfig struct {
	Permissions ClaudePermissions `yaml:"permissions" toml:"permissions" jsonschema:"description=Claude Code permissions configuration"`
	Sandbox     ClaudeSandbox     `yaml:"sandbox" toml:"sandbox" jsonschema:"description=Claude Code sandbox configuration"`
	// Inherit, when explicitly false, opts this [claude] block out of the
	// default accumulate-down (union) behavior: instead of unioning its arrays
	// with the receiver (lower cascade layers / ecosystem root), the block's
	// arrays REPLACE them wholesale (clean slate). Absent/true keeps the union.
	// Pointer distinguishes unset (nil) from explicit false, like the sandbox
	// bools. Kept OUT of IsEmpty so a lone `inherit = false` does not force a
	// settings write.
	//
	// DRIFT NOTE: this mirrors the raw-map cascade semantics in core/config
	// (deepMergeMapsUnionWithInherit / unionRawArrays, core/config/merge.go).
	// The typed Merge here (Axis B member-union) and that raw union (Axis A
	// cascade) are two impls of one semantics across a package boundary that
	// forbids sharing — keep them behaviorally in sync.
	Inherit *bool `yaml:"inherit" toml:"inherit" jsonschema:"description=When false, this block's arrays replace (rather than union with) lower cascade layers"`
	// AllowGroveTools, when true, expands at seed time into a Bash(<tool>:*)
	// permission rule for every canonical grove ecosystem CLI (grove, flow, cx,
	// nb, ...), so agents launched in the worktree can invoke the grove tools
	// without a per-command permission prompt. The expansion lives in the seeder
	// (groveToolBashRules); this is just the opt-in flag. Pointer distinguishes
	// unset (nil) from explicit false, like the sandbox bools. Kept OUT of
	// IsEmpty (a lone flag is handled by a dedicated predicate in SeedSettings),
	// but unlike a no-op it DOES force a write — see SeedSettings' widened gate.
	AllowGroveTools *bool `yaml:"allowGroveTools" toml:"allowGroveTools" jsonschema:"description=When true, allow all canonical grove ecosystem CLIs (grove, flow, cx, nb, ...) via Bash(<tool>:*) rules"`
}

// ClaudePermissions holds the permissions.* settings.
type ClaudePermissions struct {
	// Allow is a list of Claude Code permission rules (e.g. "Bash(git:*)")
	// that are granted without prompting.
	Allow []string `yaml:"allow" toml:"allow" jsonschema:"description=List of Claude Code permission rules to allow without prompting"`
	// DefaultMode sets Claude Code's permissions.defaultMode — the
	// settings.local.json equivalent of the --dangerously-skip-permissions flag.
	// Valid values: default, acceptEdits, plan, bypassPermissions
	// (bypassPermissions skips permission prompts). Empty string means unset: the
	// key is not written, so an existing user value is never clobbered. Unlike
	// Allow this is a scalar string — it is NOT unioned across layers; highest
	// cascade layer wins with lower layers filling an empty gap (see Merge).
	DefaultMode string `yaml:"defaultMode" toml:"defaultMode" jsonschema:"description=Claude Code default permission mode; one of default, acceptEdits, plan, bypassPermissions (bypassPermissions skips permission prompts); empty means unset"`
}

// ClaudeSandbox holds the sandbox.* settings.
type ClaudeSandbox struct {
	// Enabled enables OS-level sandboxing of tool calls.
	Enabled *bool `yaml:"enabled" toml:"enabled" jsonschema:"description=Enable OS-level sandboxing of tool calls"`
	// FailIfUnavailable fails if sandboxing is requested but unavailable on the OS.
	FailIfUnavailable *bool `yaml:"failIfUnavailable" toml:"failIfUnavailable" jsonschema:"description=Fail if sandboxing is requested but unavailable"`
	// AutoAllowBashIfSandboxed automatically allows Bash commands when sandboxed.
	AutoAllowBashIfSandboxed *bool `yaml:"autoAllowBashIfSandboxed" toml:"autoAllowBashIfSandboxed" jsonschema:"description=Auto-allow Bash commands when sandboxed"`
	// Filesystem holds filesystem sandbox settings.
	Filesystem ClaudeSandboxFilesystem `yaml:"filesystem" toml:"filesystem" jsonschema:"description=Filesystem sandbox configuration"`
	// Network holds network sandbox settings.
	Network ClaudeSandboxNetwork `yaml:"network" toml:"network" jsonschema:"description=Network sandbox configuration"`
}

// ClaudeSandboxFilesystem holds the sandbox.filesystem.* settings.
type ClaudeSandboxFilesystem struct {
	// AllowWrite is a list of directories the sandbox allows writing to.
	AllowWrite []string `yaml:"allowWrite" toml:"allowWrite" jsonschema:"description=Directories the sandbox allows writing to"`
}

// ClaudeSandboxNetwork holds the sandbox.network.* settings.
type ClaudeSandboxNetwork struct {
	// AllowedDomains is a list of domains the sandbox allows network access to.
	AllowedDomains []string `yaml:"allowedDomains" toml:"allowedDomains" jsonschema:"description=Domains the sandbox allows network access to"`
}

// IsEmpty returns true if no configuration is set.
func (c *ClaudeConfig) IsEmpty() bool {
	return len(c.Permissions.Allow) == 0 &&
		c.Permissions.DefaultMode == "" &&
		c.Sandbox.Enabled == nil &&
		c.Sandbox.FailIfUnavailable == nil &&
		c.Sandbox.AutoAllowBashIfSandboxed == nil &&
		len(c.Sandbox.Filesystem.AllowWrite) == 0 &&
		len(c.Sandbox.Network.AllowedDomains) == 0
}

// Merge combines two ClaudeConfigs. Arrays are unioned and deduped.
// For booleans, `other` values take precedence over `c` when non-nil
// (ecosystem-root-wins semantics: call as root.Merge(member)).
func (c *ClaudeConfig) Merge(other *ClaudeConfig) {
	if other == nil {
		return
	}
	if other.Inherit != nil && !*other.Inherit {
		// inherit=false: the incoming layer opts out of accumulation, so its
		// arrays replace the receiver's wholesale instead of unioning. Mirrors
		// deepMergeMapsUnionWithInherit in core/config/merge.go.
		c.Permissions.Allow = append([]string(nil), other.Permissions.Allow...)
		c.Sandbox.Filesystem.AllowWrite = append([]string(nil), other.Sandbox.Filesystem.AllowWrite...)
		c.Sandbox.Network.AllowedDomains = append([]string(nil), other.Sandbox.Network.AllowedDomains...)
	} else {
		c.Permissions.Allow = unionStrings(c.Permissions.Allow, other.Permissions.Allow)
		c.Sandbox.Filesystem.AllowWrite = unionStrings(c.Sandbox.Filesystem.AllowWrite, other.Sandbox.Filesystem.AllowWrite)
		c.Sandbox.Network.AllowedDomains = unionStrings(c.Sandbox.Network.AllowedDomains, other.Sandbox.Network.AllowedDomains)
	}

	// For booleans, other (member) fills in gaps only if c (root) is nil.
	// This implements ecosystem-root-wins precedence.
	if c.Sandbox.Enabled == nil && other.Sandbox.Enabled != nil {
		c.Sandbox.Enabled = other.Sandbox.Enabled
	}
	if c.Sandbox.FailIfUnavailable == nil && other.Sandbox.FailIfUnavailable != nil {
		c.Sandbox.FailIfUnavailable = other.Sandbox.FailIfUnavailable
	}
	if c.Sandbox.AutoAllowBashIfSandboxed == nil && other.Sandbox.AutoAllowBashIfSandboxed != nil {
		c.Sandbox.AutoAllowBashIfSandboxed = other.Sandbox.AutoAllowBashIfSandboxed
	}
	// AllowGroveTools is a root-wins-gap scalar like the sandbox bools: a member
	// fills the slot only when the root left it nil. Note this scalar lives
	// outside the array branch above, so inherit=false (which only REPLACES
	// arrays wholesale) does NOT un-inherit it — a member's allowGroveTools=true
	// still flows up through this gap-fill regardless of the inherit flag.
	if c.AllowGroveTools == nil && other.AllowGroveTools != nil {
		c.AllowGroveTools = other.AllowGroveTools
	}
	// Permissions.DefaultMode is a root-wins-gap scalar string (empty = unset),
	// mirroring the *bool gap-fills above: a member fills the slot only when the
	// root left it empty, and an explicit root value survives. It is a scalar, so
	// it is NOT unioned; it also lives outside the array branch above, so
	// inherit=false (which only REPLACES arrays wholesale) does not clear it.
	if c.Permissions.DefaultMode == "" && other.Permissions.DefaultMode != "" {
		c.Permissions.DefaultMode = other.Permissions.DefaultMode
	}
}

// unionStrings returns the union of two string slices, preserving order
// (a's elements first, then b's new elements).
func unionStrings(a, b []string) []string {
	seen := make(map[string]struct{}, len(a))
	result := make([]string, 0, len(a)+len(b))
	for _, s := range a {
		if _, ok := seen[s]; !ok && s != "" {
			seen[s] = struct{}{}
			result = append(result, s)
		}
	}
	for _, s := range b {
		if _, ok := seen[s]; !ok && s != "" {
			seen[s] = struct{}{}
			result = append(result, s)
		}
	}
	return result
}
