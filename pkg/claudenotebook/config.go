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
	// ProtectConfig, when true, makes the seeder inject self-protection entries
	// into settings.local.json so a sandboxed agent cannot edit the very config
	// files that sandbox it: a sandbox.filesystem.denyWrite block (OS-enforced for
	// shell writes, bypass-proof when sandbox.enabled) plus best-effort
	// permissions.deny Edit/Write rules (the native-tool seam) on the worktree's
	// grove.toml(s), member-repo configs, and ~/.config/grove. When explicitly
	// false the seeder ACTIVELY STRIPS those grove-owned entries (reversible, not
	// just skip-on-write), never touching user-authored deny rules. Unset = off
	// (opt-in); dispatched-agent launches opt in via the ecosystem grove.toml. The
	// dev escape hatch is the GROVE_UNLOCK_CONFIG=1 launch env var, which lives
	// OUTSIDE the protected files. Pointer distinguishes unset (nil) from explicit
	// false. Kept OUT of IsEmpty (a lone toggle is honored by ShouldSeed) so that
	// both true (write) and false (strip) reach the seeder even on an otherwise
	// empty [claude] block. Never protects tool INVOCATION (no Bash(grove:*) deny)
	// — only file PATHS.
	ProtectConfig *bool `yaml:"protectConfig" toml:"protectConfig" jsonschema:"description=When true, deny sandbox+native writes to grove config files (grove.toml, member configs, ~/.config/grove) so a sandboxed agent cannot edit the config that sandboxes it; false actively strips those grove-owned entries"`
	// ManageTrust, when true, lets grove manage Claude folder-trust in
	// ~/.claude.json (seed on worktree creation, prune orphans on daemon
	// reconcile). Lone flag: unset (nil) or false => grove does NOT touch
	// ~/.claude.json. Default is therefore OFF (opt-in). This deliberately flips
	// the historical default-ON behavior gated only by the
	// GROVE_PRESEED_CLAUDE_TRUST env var (which remains a low-level kill-switch
	// inside core/pkg/claudetrust). Pointer distinguishes unset (nil) from
	// explicit false, like the sandbox bools. Kept OUT of IsEmpty/ShouldSeed (a
	// lone toggle, like Inherit) — trust is a separate concern from settings
	// seeding and must not trigger a settings write on its own.
	ManageTrust *bool `yaml:"manageTrust" toml:"manageTrust" jsonschema:"description=When true, grove manages Claude folder-trust in ~/.claude.json (seed on worktree creation, prune orphans on daemon reconcile); unset/false means grove never touches ~/.claude.json (opt-in, default off)"`
	// AutoMode declaratively manages Claude Code's top-level `autoMode` classifier
	// (the sections consulted by the "auto" permission mode). Pointer distinguishes
	// unset (nil) from set. This IS a real setting (not a lone flag): it counts in
	// IsEmpty when any section array is non-empty, so a lone [claude.autoMode] block
	// forces a settings write. See ClaudeAutoMode for the "$defaults" splice
	// semantics and the snake_case key rationale.
	AutoMode *ClaudeAutoMode `yaml:"autoMode" toml:"autoMode" jsonschema:"description=Claude Code auto-mode classifier sections (allow/soft_deny/environment/hard_deny) consulted by the auto permission mode"`
	// UseAutoModeDuringPlan maps to Claude's top-level useAutoModeDuringPlan bool:
	// apply the auto-mode classifier during plan mode to auto-approve safe
	// read-only calls. Unlike the lone flags (manageTrust/inherit) this maps to a
	// written settings key, so it counts in IsEmpty. Grove writes it regardless of
	// defaultMode — it does NOT enforce Claude's "no effect unless auto" cross-field
	// rule (not grove's job). Pointer distinguishes unset (nil) from explicit false.
	UseAutoModeDuringPlan *bool `yaml:"useAutoModeDuringPlan" toml:"useAutoModeDuringPlan" jsonschema:"description=When true, apply the auto-mode classifier during plan mode to auto-approve safe read-only calls (has no effect in Claude unless defaultMode allows auto)"`
}

// ClaudePermissions holds the permissions.* settings.
type ClaudePermissions struct {
	// Allow is a list of Claude Code permission rules (e.g. "Bash(git:*)")
	// that are granted without prompting.
	Allow []string `yaml:"allow" toml:"allow" jsonschema:"description=List of Claude Code permission rules to allow without prompting"`
	// DefaultMode sets Claude Code's permissions.defaultMode — the
	// settings.local.json equivalent of the --dangerously-skip-permissions flag.
	// Commonly used values: default, acceptEdits, plan, bypassPermissions, auto
	// (bypassPermissions skips permission prompts; auto auto-approves tool calls
	// with background safety checks that verify actions align with your request).
	// Claude also recognizes the undocumented modes delegate and dontAsk. Empty
	// string means unset: the key is not written, so an existing user value is
	// never clobbered. Unlike Allow this is a scalar string — it is NOT unioned
	// across layers; highest cascade layer wins with lower layers filling an empty
	// gap (see Merge). Kept a plain, free-form string on purpose (NO enum): Claude
	// adds modes over time and grove passes them through verbatim, matching the
	// tolerant/passthrough philosophy of the read side (ccsettings).
	DefaultMode string `yaml:"defaultMode" toml:"defaultMode" jsonschema:"description=Claude Code default permission mode; commonly default, acceptEdits, plan, bypassPermissions, or auto (bypassPermissions skips prompts; auto auto-approves with background safety checks); delegate and dontAsk also exist; free-form passthrough, empty means unset"`
	// Deny is a list of Claude Code permission rules (e.g. "Edit(//path/**)")
	// that are denied. Unioned across layers like Allow. The self-protection
	// toggle (ProtectConfig) appends grove-owned Edit/Write/MultiEdit rules to
	// this same array; user-authored Deny entries are preserved and never
	// stripped.
	Deny []string `yaml:"deny" toml:"deny" jsonschema:"description=List of Claude Code permission rules to deny"`
}

// ClaudeAutoMode mirrors Claude Code's top-level `autoMode` classifier object —
// the sections the "auto" permission mode consults when auto-approving tool
// calls. Each section REPLACES Claude's built-in section entirely unless the
// array includes the literal string "$defaults", which splices the built-ins in
// at that position.
//
// Key casing is deliberately snake_case (soft_deny/hard_deny) to MATCH Claude's
// literal JSON keys — the written settings.local.json keys MUST be exactly
// soft_deny/hard_deny, so keeping the grove.toml keys identical avoids a mapping
// layer. (This is the one place [claude] fields are not camelCase, on purpose.)
type ClaudeAutoMode struct {
	// Allow is the auto-mode allow section (calls auto-approved without a prompt).
	Allow []string `yaml:"allow" toml:"allow" jsonschema:"description=Auto-mode allow section; use \"$defaults\" to splice in Claude's built-ins"`
	// SoftDeny is the auto-mode soft_deny section (calls the classifier declines
	// softly). JSON key MUST be snake_case soft_deny.
	SoftDeny []string `yaml:"soft_deny" toml:"soft_deny" jsonschema:"description=Auto-mode soft_deny section; use \"$defaults\" to splice in Claude's built-ins"`
	// Environment is the auto-mode environment section.
	Environment []string `yaml:"environment" toml:"environment" jsonschema:"description=Auto-mode environment section; use \"$defaults\" to splice in Claude's built-ins"`
	// HardDeny is the auto-mode hard_deny section (calls always refused). JSON key
	// MUST be snake_case hard_deny.
	HardDeny []string `yaml:"hard_deny" toml:"hard_deny" jsonschema:"description=Auto-mode hard_deny section; use \"$defaults\" to splice in Claude's built-ins"`
}

// isEmpty reports whether every auto-mode section array is empty. A non-nil
// AutoMode whose sections are all empty is treated as unset (no write, no
// ShouldSeed) so grove never forces empty arrays into settings.local.json.
func (a *ClaudeAutoMode) isEmpty() bool {
	return a == nil ||
		(len(a.Allow) == 0 &&
			len(a.SoftDeny) == 0 &&
			len(a.Environment) == 0 &&
			len(a.HardDeny) == 0)
}

// ClaudeSandbox holds the sandbox.* settings.
type ClaudeSandbox struct {
	// Enabled enables OS-level sandboxing of tool calls.
	Enabled *bool `yaml:"enabled" toml:"enabled" jsonschema:"description=Enable OS-level sandboxing of tool calls"`
	// FailIfUnavailable fails if sandboxing is requested but unavailable on the OS.
	FailIfUnavailable *bool `yaml:"failIfUnavailable" toml:"failIfUnavailable" jsonschema:"description=Fail if sandboxing is requested but unavailable"`
	// AutoAllowBashIfSandboxed automatically allows Bash commands when sandboxed.
	AutoAllowBashIfSandboxed *bool `yaml:"autoAllowBashIfSandboxed" toml:"autoAllowBashIfSandboxed" jsonschema:"description=Auto-allow Bash commands when sandboxed"`
	// AllowUnsandboxedCommands controls Claude's per-call sandbox escape hatch: the
	// Bash tool's dangerouslyDisableSandbox parameter. The INTENDED value here is
	// false — when false, dangerouslyDisableSandbox is completely ignored and every
	// command must run sandboxed, turning the sandbox into a hard floor the agent
	// cannot wave itself out of per-call (critical under defaultMode "auto" /
	// bypassPermissions, where the escape-hatch approval prompt is auto-answered).
	// The sanctioned way to still run specific vetted tools unsandboxed is
	// ExcludedCommands, not this blanket hatch. Pointer distinguishes unset (nil,
	// leaving Claude's true default) from explicit false — and explicit false is
	// the whole point, so it must survive IsEmpty→Merge→seeder and land as literal
	// JSON false. toml/yaml key matches Claude's JSON key verbatim.
	AllowUnsandboxedCommands *bool `yaml:"allowUnsandboxedCommands" toml:"allowUnsandboxedCommands" jsonschema:"description=When false, ignore the Bash dangerouslyDisableSandbox escape hatch so every command must run sandboxed (locks the sandbox as a hard floor); nil leaves Claude's default"`
	// ExcludedCommands is the curated allowlist of named commands (e.g.
	// ["git","docker"]) permitted to run UNSANDBOXED — the sanctioned replacement
	// for the blanket dangerouslyDisableSandbox hatch that AllowUnsandboxedCommands
	// locks. Only bites when Enabled is also true (grove writes it regardless, not
	// enforcing that cross-field rule). Unioned across layers like
	// Network.AllowedDomains.
	ExcludedCommands []string `yaml:"excludedCommands" toml:"excludedCommands" jsonschema:"description=Named commands allowed to run unsandboxed (the vetted replacement for the blanket per-call escape hatch)"`
	// Filesystem holds filesystem sandbox settings.
	Filesystem ClaudeSandboxFilesystem `yaml:"filesystem" toml:"filesystem" jsonschema:"description=Filesystem sandbox configuration"`
	// Network holds network sandbox settings.
	Network ClaudeSandboxNetwork `yaml:"network" toml:"network" jsonschema:"description=Network sandbox configuration"`
}

// ClaudeSandboxFilesystem holds the sandbox.filesystem.* settings.
type ClaudeSandboxFilesystem struct {
	// AllowWrite is a list of directories the sandbox allows writing to.
	AllowWrite []string `yaml:"allowWrite" toml:"allowWrite" jsonschema:"description=Directories the sandbox allows writing to"`
	// DenyWrite is a list of paths the OS sandbox forbids writing to, enforced
	// for Bash commands and their child processes independently of permission
	// mode (it holds even under bypassPermissions when sandbox.enabled). Unioned
	// across layers like AllowWrite. The self-protection toggle (ProtectConfig)
	// appends grove-owned config-file paths here; user-authored DenyWrite entries
	// are preserved and never stripped.
	DenyWrite []string `yaml:"denyWrite" toml:"denyWrite" jsonschema:"description=Paths the OS sandbox forbids writing to (Bash/child-process writes), enforced even under bypassPermissions"`
}

// ClaudeSandboxNetwork holds the sandbox.network.* settings.
type ClaudeSandboxNetwork struct {
	// AllowedDomains is a list of domains the sandbox allows network access to.
	AllowedDomains []string `yaml:"allowedDomains" toml:"allowedDomains" jsonschema:"description=Domains the sandbox allows network access to"`
	// AllowUnixSockets is a list of unix-domain socket paths the sandbox allows
	// connecting to (connect-only, per path). Does NOT grant bind() — for that a
	// caller needs AllowAllUnixSockets (path-scoped bind is not yet a Claude Code
	// feature). Unioned across layers like AllowedDomains.
	AllowUnixSockets []string `yaml:"allowUnixSockets" toml:"allowUnixSockets" jsonschema:"description=Unix-domain socket paths the sandbox allows connecting to (connect-only, per path)"`
	// AllowAllUnixSockets, when true, lets sandboxed processes connect AND bind
	// unix-domain sockets at ANY path. This is the only knob that currently
	// enables socket bind() (e.g. a tuimux daemon's listening socket). It is
	// coarse and a security tradeoff: it opens docker.sock, the SSH agent, and
	// GPG agent sockets to the sandboxed process. Prefer the per-path
	// AllowUnixSockets when connect-only access suffices; reach for this only when
	// bind() is required. Pointer distinguishes unset (nil) vs explicit false,
	// like the sandbox bools.
	AllowAllUnixSockets *bool `yaml:"allowAllUnixSockets" toml:"allowAllUnixSockets" jsonschema:"description=When true, allow connecting AND binding unix-domain sockets at any path (coarse; enables socket bind but opens docker.sock/SSH/GPG agents)"`
	// AllowLocalBinding, when true, lets sandboxed processes bind localhost TCP
	// ports. This is needed to bind grove's own daemon (groved) inside a
	// sandboxed worktree — it listens on a 127.0.0.1 port, which the sandbox
	// otherwise blocks. Pointer distinguishes unset (nil) vs explicit false, like
	// the sandbox bools.
	AllowLocalBinding *bool `yaml:"allowLocalBinding" toml:"allowLocalBinding" jsonschema:"description=When true, allow binding localhost TCP ports"`
}

// IsEmpty returns true if no configuration is set. ProtectConfig,
// AllowGroveTools and ManageTrust are deliberately excluded: they are lone-flag
// signals honored elsewhere (ShouldSeed / the seeder's gate for the first two;
// the trust gate in the workspace/daemon callers for ManageTrust), not by
// IsEmpty (mirroring Inherit).
func (c *ClaudeConfig) IsEmpty() bool {
	return len(c.Permissions.Allow) == 0 &&
		len(c.Permissions.Deny) == 0 &&
		c.Permissions.DefaultMode == "" &&
		c.Sandbox.Enabled == nil &&
		c.Sandbox.FailIfUnavailable == nil &&
		c.Sandbox.AutoAllowBashIfSandboxed == nil &&
		c.Sandbox.AllowUnsandboxedCommands == nil &&
		len(c.Sandbox.ExcludedCommands) == 0 &&
		len(c.Sandbox.Filesystem.AllowWrite) == 0 &&
		len(c.Sandbox.Filesystem.DenyWrite) == 0 &&
		len(c.Sandbox.Network.AllowedDomains) == 0 &&
		len(c.Sandbox.Network.AllowUnixSockets) == 0 &&
		c.Sandbox.Network.AllowAllUnixSockets == nil &&
		c.Sandbox.Network.AllowLocalBinding == nil &&
		c.AutoMode.isEmpty() &&
		c.UseAutoModeDuringPlan == nil
}

// ShouldSeed reports whether this config carries any signal the seeder must act
// on. It is the gate the seeder and its upstream callers use INSTEAD of a bare
// !IsEmpty() check, because two lone-flag signals live outside IsEmpty:
//   - AllowGroveTools=true expands into Bash(<tool>:*) allow rules.
//   - ProtectConfig set (true OR false) must reach the seeder — true to write the
//     self-protection entries, false to actively strip them. A protectConfig-only
//     grove.toml is IsEmpty()==true, so without this predicate the upstream
//     IsEmpty guards would drop it before the seeder ever runs.
func (c *ClaudeConfig) ShouldSeed() bool {
	if c == nil {
		return false
	}
	if !c.IsEmpty() {
		return true
	}
	if c.ProtectConfig != nil {
		return true
	}
	if c.AllowGroveTools != nil && *c.AllowGroveTools {
		return true
	}
	return false
}

// ManagesTrust reports whether grove should manage Claude folder-trust for
// this resolved profile. Default (nil) is false. This is the config master
// enable checked by the workspace seed sites and the daemon; the env
// kill-switch (GROVE_PRESEED_CLAUDE_TRUST) is enforced independently inside
// core/pkg/claudetrust.
func (c *ClaudeConfig) ManagesTrust() bool {
	return c != nil && c.ManageTrust != nil && *c.ManageTrust
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
		c.Permissions.Deny = append([]string(nil), other.Permissions.Deny...)
		c.Sandbox.Filesystem.AllowWrite = append([]string(nil), other.Sandbox.Filesystem.AllowWrite...)
		c.Sandbox.Filesystem.DenyWrite = append([]string(nil), other.Sandbox.Filesystem.DenyWrite...)
		c.Sandbox.Network.AllowedDomains = append([]string(nil), other.Sandbox.Network.AllowedDomains...)
		c.Sandbox.Network.AllowUnixSockets = append([]string(nil), other.Sandbox.Network.AllowUnixSockets...)
		c.Sandbox.ExcludedCommands = append([]string(nil), other.Sandbox.ExcludedCommands...)
		// autoMode arrays are replaced wholesale too (clone the incoming layer's
		// classifier, clearing c's when other has none) — same clean-slate rule as
		// the permission/sandbox arrays above.
		c.AutoMode = cloneAutoMode(other.AutoMode)
	} else {
		c.Permissions.Allow = unionStrings(c.Permissions.Allow, other.Permissions.Allow)
		c.Permissions.Deny = unionStrings(c.Permissions.Deny, other.Permissions.Deny)
		c.Sandbox.Filesystem.AllowWrite = unionStrings(c.Sandbox.Filesystem.AllowWrite, other.Sandbox.Filesystem.AllowWrite)
		c.Sandbox.Filesystem.DenyWrite = unionStrings(c.Sandbox.Filesystem.DenyWrite, other.Sandbox.Filesystem.DenyWrite)
		c.Sandbox.Network.AllowedDomains = unionStrings(c.Sandbox.Network.AllowedDomains, other.Sandbox.Network.AllowedDomains)
		c.Sandbox.Network.AllowUnixSockets = unionStrings(c.Sandbox.Network.AllowUnixSockets, other.Sandbox.Network.AllowUnixSockets)
		c.Sandbox.ExcludedCommands = unionStrings(c.Sandbox.ExcludedCommands, other.Sandbox.ExcludedCommands)
		// autoMode: adopt other's if root has none, else union each of the four
		// section arrays (the "$defaults" token is just another string entry that
		// union/dedup handles), same as Permissions.Allow.
		switch {
		case other.AutoMode == nil:
			// nothing to merge in
		case c.AutoMode == nil:
			c.AutoMode = cloneAutoMode(other.AutoMode)
		default:
			c.AutoMode.Allow = unionStrings(c.AutoMode.Allow, other.AutoMode.Allow)
			c.AutoMode.SoftDeny = unionStrings(c.AutoMode.SoftDeny, other.AutoMode.SoftDeny)
			c.AutoMode.Environment = unionStrings(c.AutoMode.Environment, other.AutoMode.Environment)
			c.AutoMode.HardDeny = unionStrings(c.AutoMode.HardDeny, other.AutoMode.HardDeny)
		}
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
	// AllowUnsandboxedCommands: root-wins gap-fill mirroring Sandbox.Enabled above.
	// An explicit root value (including false — the whole point of the lock) wins;
	// a member fills the slot only when the root left it nil.
	if c.Sandbox.AllowUnsandboxedCommands == nil && other.Sandbox.AllowUnsandboxedCommands != nil {
		c.Sandbox.AllowUnsandboxedCommands = other.Sandbox.AllowUnsandboxedCommands
	}
	if c.Sandbox.Network.AllowAllUnixSockets == nil && other.Sandbox.Network.AllowAllUnixSockets != nil {
		c.Sandbox.Network.AllowAllUnixSockets = other.Sandbox.Network.AllowAllUnixSockets
	}
	if c.Sandbox.Network.AllowLocalBinding == nil && other.Sandbox.Network.AllowLocalBinding != nil {
		c.Sandbox.Network.AllowLocalBinding = other.Sandbox.Network.AllowLocalBinding
	}
	// AllowGroveTools is a root-wins-gap scalar like the sandbox bools: a member
	// fills the slot only when the root left it nil. Note this scalar lives
	// outside the array branch above, so inherit=false (which only REPLACES
	// arrays wholesale) does NOT un-inherit it — a member's allowGroveTools=true
	// still flows up through this gap-fill regardless of the inherit flag.
	if c.AllowGroveTools == nil && other.AllowGroveTools != nil {
		c.AllowGroveTools = other.AllowGroveTools
	}
	// ProtectConfig is a root-wins-gap scalar like AllowGroveTools: a member fills
	// the slot only when the root left it nil, so an explicit root value (true or
	// false) survives. It lives outside the array branch above, so inherit=false
	// (which only REPLACES arrays wholesale) does not clear it.
	if c.ProtectConfig == nil && other.ProtectConfig != nil {
		c.ProtectConfig = other.ProtectConfig
	}
	// ManageTrust: root-wins gap-fill (outside the array branch, so
	// inherit=false does not clear it) — same as AllowGroveTools/ProtectConfig.
	if c.ManageTrust == nil && other.ManageTrust != nil {
		c.ManageTrust = other.ManageTrust
	}
	// Permissions.DefaultMode is a root-wins-gap scalar string (empty = unset),
	// mirroring the *bool gap-fills above: a member fills the slot only when the
	// root left it empty, and an explicit root value survives. It is a scalar, so
	// it is NOT unioned; it also lives outside the array branch above, so
	// inherit=false (which only REPLACES arrays wholesale) does not clear it.
	if c.Permissions.DefaultMode == "" && other.Permissions.DefaultMode != "" {
		c.Permissions.DefaultMode = other.Permissions.DefaultMode
	}
	// UseAutoModeDuringPlan: root-wins gap-fill like the sandbox bools (outside the
	// array branch, so inherit=false does not clear it).
	if c.UseAutoModeDuringPlan == nil && other.UseAutoModeDuringPlan != nil {
		c.UseAutoModeDuringPlan = other.UseAutoModeDuringPlan
	}
}

// cloneAutoMode returns a deep copy of an autoMode classifier (nil-safe), so a
// merged config never aliases another layer's section slices. Used by Merge for
// both the gap-adopt and the inherit=false clean-slate paths.
func cloneAutoMode(a *ClaudeAutoMode) *ClaudeAutoMode {
	if a == nil {
		return nil
	}
	return &ClaudeAutoMode{
		Allow:       append([]string(nil), a.Allow...),
		SoftDeny:    append([]string(nil), a.SoftDeny...),
		Environment: append([]string(nil), a.Environment...),
		HardDeny:    append([]string(nil), a.HardDeny...),
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
