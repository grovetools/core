package claudenotebook

import (
	"reflect"
	"testing"

	"github.com/mitchellh/mapstructure"
	toml "github.com/pelletier/go-toml/v2"
)

func boolPtr(b bool) *bool { return &b }

// TestMerge_UnionByDefault confirms the baseline accumulate behavior: arrays
// union and dedupe when inherit is unset.
func TestMerge_UnionByDefault(t *testing.T) {
	root := &ClaudeConfig{
		Permissions: ClaudePermissions{Allow: []string{"RootOnly(r:*)", "Shared(s:*)"}},
	}
	member := &ClaudeConfig{
		Permissions: ClaudePermissions{Allow: []string{"Shared(s:*)", "MemberOnly(m:*)"}},
	}

	root.Merge(member)

	want := []string{"RootOnly(r:*)", "Shared(s:*)", "MemberOnly(m:*)"}
	if !reflect.DeepEqual(root.Permissions.Allow, want) {
		t.Errorf("expected union %v, got %v", want, root.Permissions.Allow)
	}
}

// TestMerge_InheritFalseOverwrites confirms that other.Inherit=false replaces
// the receiver's arrays wholesale instead of unioning.
func TestMerge_InheritFalseOverwrites(t *testing.T) {
	root := &ClaudeConfig{
		Permissions: ClaudePermissions{Allow: []string{"RootOnly(r:*)"}},
		Sandbox: ClaudeSandbox{
			Filesystem: ClaudeSandboxFilesystem{AllowWrite: []string{"/root-dir"}},
			Network:    ClaudeSandboxNetwork{AllowedDomains: []string{"root.example.com"}},
		},
	}
	member := &ClaudeConfig{
		Inherit:     boolPtr(false),
		Permissions: ClaudePermissions{Allow: []string{"SvcBOnly(b:*)"}},
		Sandbox: ClaudeSandbox{
			Filesystem: ClaudeSandboxFilesystem{AllowWrite: []string{"/member-dir"}},
		},
	}

	root.Merge(member)

	if !reflect.DeepEqual(root.Permissions.Allow, []string{"SvcBOnly(b:*)"}) {
		t.Errorf("expected allow overwritten to [SvcBOnly(b:*)], got %v", root.Permissions.Allow)
	}
	if !reflect.DeepEqual(root.Sandbox.Filesystem.AllowWrite, []string{"/member-dir"}) {
		t.Errorf("expected allowWrite overwritten to [/member-dir], got %v", root.Sandbox.Filesystem.AllowWrite)
	}
	if len(root.Sandbox.Network.AllowedDomains) != 0 {
		t.Errorf("expected allowedDomains cleared (member had none), got %v", root.Sandbox.Network.AllowedDomains)
	}
}

// TestMerge_InheritTrueUnions confirms an explicit inherit=true unions like the
// unset default.
func TestMerge_InheritTrueUnions(t *testing.T) {
	root := &ClaudeConfig{Permissions: ClaudePermissions{Allow: []string{"RootOnly(r:*)"}}}
	member := &ClaudeConfig{
		Inherit:     boolPtr(true),
		Permissions: ClaudePermissions{Allow: []string{"MemberOnly(m:*)"}},
	}

	root.Merge(member)

	if len(root.Permissions.Allow) != 2 {
		t.Errorf("expected inherit=true to union, got %v", root.Permissions.Allow)
	}
}

// TestInheritNotInIsEmpty confirms a lone inherit=false does not count as
// content (must not force a settings write).
func TestInheritNotInIsEmpty(t *testing.T) {
	c := &ClaudeConfig{Inherit: boolPtr(false)}
	if !c.IsEmpty() {
		t.Errorf("expected a config with only inherit=false to be empty")
	}
}

// TestMerge_AllowGroveToolsGapFill confirms allowGroveTools is a root-wins-gap
// scalar: a member fills a nil root slot, but cannot override an explicit root
// value, and a set root value survives the merge.
func TestMerge_AllowGroveToolsGapFill(t *testing.T) {
	t.Run("root nil + member true -> true", func(t *testing.T) {
		root := &ClaudeConfig{}
		member := &ClaudeConfig{AllowGroveTools: boolPtr(true)}
		root.Merge(member)
		if root.AllowGroveTools == nil || !*root.AllowGroveTools {
			t.Errorf("expected member to fill nil root to true, got %v", root.AllowGroveTools)
		}
	})

	t.Run("root false + member true -> stays false", func(t *testing.T) {
		root := &ClaudeConfig{AllowGroveTools: boolPtr(false)}
		member := &ClaudeConfig{AllowGroveTools: boolPtr(true)}
		root.Merge(member)
		if root.AllowGroveTools == nil || *root.AllowGroveTools {
			t.Errorf("expected explicit root false to win, got %v", root.AllowGroveTools)
		}
	})

	t.Run("root true survives merge with nil member", func(t *testing.T) {
		root := &ClaudeConfig{AllowGroveTools: boolPtr(true)}
		member := &ClaudeConfig{}
		root.Merge(member)
		if root.AllowGroveTools == nil || !*root.AllowGroveTools {
			t.Errorf("expected root allowGroveTools=true to survive, got %v", root.AllowGroveTools)
		}
	})
}

// TestMerge_DefaultModeGapFill confirms permissions.defaultMode is a
// root-wins-gap scalar string (empty = unset): a member fills an empty root
// slot, an explicit root value wins over a member value, a set root value
// survives a merge with an empty member, and it is never unioned.
func TestMerge_DefaultModeGapFill(t *testing.T) {
	t.Run("root empty + member set -> member fills gap", func(t *testing.T) {
		root := &ClaudeConfig{}
		member := &ClaudeConfig{Permissions: ClaudePermissions{DefaultMode: "bypassPermissions"}}
		root.Merge(member)
		if root.Permissions.DefaultMode != "bypassPermissions" {
			t.Errorf("expected member to fill empty root, got %q", root.Permissions.DefaultMode)
		}
	})

	t.Run("root set + member set -> root (highest) wins", func(t *testing.T) {
		root := &ClaudeConfig{Permissions: ClaudePermissions{DefaultMode: "acceptEdits"}}
		member := &ClaudeConfig{Permissions: ClaudePermissions{DefaultMode: "bypassPermissions"}}
		root.Merge(member)
		if root.Permissions.DefaultMode != "acceptEdits" {
			t.Errorf("expected explicit root value to win, got %q", root.Permissions.DefaultMode)
		}
	})

	t.Run("root set survives merge with empty member", func(t *testing.T) {
		root := &ClaudeConfig{Permissions: ClaudePermissions{DefaultMode: "plan"}}
		member := &ClaudeConfig{}
		root.Merge(member)
		if root.Permissions.DefaultMode != "plan" {
			t.Errorf("expected root value to survive, got %q", root.Permissions.DefaultMode)
		}
	})

	t.Run("inherit=false does not clear the scalar", func(t *testing.T) {
		// inherit=false only replaces arrays wholesale; the scalar gap-fill is
		// outside that branch, so a member's defaultMode still flows up.
		root := &ClaudeConfig{}
		member := &ClaudeConfig{
			Inherit:     boolPtr(false),
			Permissions: ClaudePermissions{DefaultMode: "bypassPermissions"},
		}
		root.Merge(member)
		if root.Permissions.DefaultMode != "bypassPermissions" {
			t.Errorf("expected defaultMode to survive inherit=false, got %q", root.Permissions.DefaultMode)
		}
	})
}

// TestDefaultModeInIsEmpty confirms a lone defaultMode counts as content, so a
// config whose only signal is defaultMode forces a settings write.
func TestDefaultModeInIsEmpty(t *testing.T) {
	c := &ClaudeConfig{Permissions: ClaudePermissions{DefaultMode: "bypassPermissions"}}
	if c.IsEmpty() {
		t.Errorf("expected a config with defaultMode set to be non-empty")
	}
	empty := &ClaudeConfig{}
	if !empty.IsEmpty() {
		t.Errorf("expected an all-zero config to be empty")
	}
}

// TestMerge_AllowUnixSocketsUnion confirms sandbox.network.allowUnixSockets
// unions across layers by default and is replaced wholesale under inherit=false,
// mirroring allowedDomains.
func TestMerge_AllowUnixSocketsUnion(t *testing.T) {
	t.Run("union by default", func(t *testing.T) {
		root := &ClaudeConfig{Sandbox: ClaudeSandbox{Network: ClaudeSandboxNetwork{
			AllowUnixSockets: []string{"/run/root.sock", "/run/shared.sock"},
		}}}
		member := &ClaudeConfig{Sandbox: ClaudeSandbox{Network: ClaudeSandboxNetwork{
			AllowUnixSockets: []string{"/run/shared.sock", "/run/member.sock"},
		}}}
		root.Merge(member)
		want := []string{"/run/root.sock", "/run/shared.sock", "/run/member.sock"}
		if !reflect.DeepEqual(root.Sandbox.Network.AllowUnixSockets, want) {
			t.Errorf("expected union %v, got %v", want, root.Sandbox.Network.AllowUnixSockets)
		}
	})

	t.Run("inherit=false replaces wholesale", func(t *testing.T) {
		root := &ClaudeConfig{Sandbox: ClaudeSandbox{Network: ClaudeSandboxNetwork{
			AllowUnixSockets: []string{"/run/root.sock"},
		}}}
		member := &ClaudeConfig{
			Inherit: boolPtr(false),
			Sandbox: ClaudeSandbox{Network: ClaudeSandboxNetwork{
				AllowUnixSockets: []string{"/run/member.sock"},
			}},
		}
		root.Merge(member)
		if !reflect.DeepEqual(root.Sandbox.Network.AllowUnixSockets, []string{"/run/member.sock"}) {
			t.Errorf("expected allowUnixSockets overwritten to [/run/member.sock], got %v", root.Sandbox.Network.AllowUnixSockets)
		}
	})
}

// TestMerge_SocketBoolGapFill confirms allowAllUnixSockets and allowLocalBinding
// are root-wins-gap *bool scalars like the other sandbox bools: a member fills a
// nil root slot, an explicit root value survives.
func TestMerge_SocketBoolGapFill(t *testing.T) {
	t.Run("root nil + member set -> member fills gap", func(t *testing.T) {
		root := &ClaudeConfig{}
		member := &ClaudeConfig{Sandbox: ClaudeSandbox{Network: ClaudeSandboxNetwork{
			AllowAllUnixSockets: boolPtr(true),
			AllowLocalBinding:   boolPtr(true),
		}}}
		root.Merge(member)
		if root.Sandbox.Network.AllowAllUnixSockets == nil || !*root.Sandbox.Network.AllowAllUnixSockets {
			t.Errorf("expected member to fill allowAllUnixSockets, got %v", root.Sandbox.Network.AllowAllUnixSockets)
		}
		if root.Sandbox.Network.AllowLocalBinding == nil || !*root.Sandbox.Network.AllowLocalBinding {
			t.Errorf("expected member to fill allowLocalBinding, got %v", root.Sandbox.Network.AllowLocalBinding)
		}
	})

	t.Run("explicit root false wins over member true", func(t *testing.T) {
		root := &ClaudeConfig{Sandbox: ClaudeSandbox{Network: ClaudeSandboxNetwork{
			AllowAllUnixSockets: boolPtr(false),
		}}}
		member := &ClaudeConfig{Sandbox: ClaudeSandbox{Network: ClaudeSandboxNetwork{
			AllowAllUnixSockets: boolPtr(true),
		}}}
		root.Merge(member)
		if root.Sandbox.Network.AllowAllUnixSockets == nil || *root.Sandbox.Network.AllowAllUnixSockets {
			t.Errorf("expected explicit root false to win, got %v", root.Sandbox.Network.AllowAllUnixSockets)
		}
	})
}

// TestSocketKnobsInIsEmpty confirms each new sandbox.network knob counts as
// content, so a config whose only signal is one of them forces a settings write.
func TestSocketKnobsInIsEmpty(t *testing.T) {
	cases := map[string]*ClaudeConfig{
		"allowUnixSockets":    {Sandbox: ClaudeSandbox{Network: ClaudeSandboxNetwork{AllowUnixSockets: []string{"/run/a.sock"}}}},
		"allowAllUnixSockets": {Sandbox: ClaudeSandbox{Network: ClaudeSandboxNetwork{AllowAllUnixSockets: boolPtr(true)}}},
		"allowLocalBinding":   {Sandbox: ClaudeSandbox{Network: ClaudeSandboxNetwork{AllowLocalBinding: boolPtr(false)}}},
	}
	for name, c := range cases {
		if c.IsEmpty() {
			t.Errorf("expected config with %s set to be non-empty", name)
		}
	}
	if !(&ClaudeConfig{}).IsEmpty() {
		t.Errorf("expected an all-zero config to be empty")
	}
}

// TestDecode_InheritRoundTrips confirms a raw map decodes `inherit` into the
// *bool field (mirrors UnmarshalExtension's mapstructure/yaml-tag decode).
func TestDecode_InheritRoundTrips(t *testing.T) {
	raw := map[string]interface{}{
		"inherit": false,
		"permissions": map[string]interface{}{
			"allow": []interface{}{"SvcBOnly(b:*)"},
		},
	}

	var cfg ClaudeConfig
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  &cfg,
		TagName: "yaml",
	})
	if err != nil {
		t.Fatalf("new decoder: %v", err)
	}
	if err := decoder.Decode(raw); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if cfg.Inherit == nil {
		t.Fatal("expected inherit decoded into *bool, got nil")
	}
	if *cfg.Inherit != false {
		t.Errorf("expected inherit=false, got %v", *cfg.Inherit)
	}
	if !reflect.DeepEqual(cfg.Permissions.Allow, []string{"SvcBOnly(b:*)"}) {
		t.Errorf("expected allow decoded, got %v", cfg.Permissions.Allow)
	}
}

// TestDecode_InheritAbsentLeavesNil confirms an absent inherit key leaves the
// pointer nil (unset), so the default union path is taken.
func TestDecode_InheritAbsentLeavesNil(t *testing.T) {
	raw := map[string]interface{}{
		"permissions": map[string]interface{}{"allow": []interface{}{"X(x:*)"}},
	}

	var cfg ClaudeConfig
	decoder, _ := mapstructure.NewDecoder(&mapstructure.DecoderConfig{Result: &cfg, TagName: "yaml"})
	if err := decoder.Decode(raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if cfg.Inherit != nil {
		t.Errorf("expected inherit nil when absent, got %v", *cfg.Inherit)
	}
}

// ============================================================================
// autoMode classifier + useAutoModeDuringPlan (Part B)
// ============================================================================

// TestMerge_AutoModeArrayUnion confirms the four autoMode section arrays union
// across layers by default (deduping, and passing "$defaults" through as an
// ordinary entry), and are replaced wholesale under inherit=false.
func TestMerge_AutoModeArrayUnion(t *testing.T) {
	t.Run("union by default", func(t *testing.T) {
		root := &ClaudeConfig{AutoMode: &ClaudeAutoMode{
			Allow:    []string{"$defaults", "Bash(git:*)"},
			SoftDeny: []string{"Read(secret)"},
		}}
		member := &ClaudeConfig{AutoMode: &ClaudeAutoMode{
			Allow:    []string{"Bash(git:*)", "Bash(ls:*)"}, // Bash(git:*) dup
			HardDeny: []string{"Bash(rm:*)"},
		}}
		root.Merge(member)

		wantAllow := []string{"$defaults", "Bash(git:*)", "Bash(ls:*)"}
		if !reflect.DeepEqual(root.AutoMode.Allow, wantAllow) {
			t.Errorf("expected allow union %v, got %v", wantAllow, root.AutoMode.Allow)
		}
		if !reflect.DeepEqual(root.AutoMode.SoftDeny, []string{"Read(secret)"}) {
			t.Errorf("expected soft_deny preserved, got %v", root.AutoMode.SoftDeny)
		}
		if !reflect.DeepEqual(root.AutoMode.HardDeny, []string{"Bash(rm:*)"}) {
			t.Errorf("expected hard_deny adopted, got %v", root.AutoMode.HardDeny)
		}
	})

	t.Run("inherit=false replaces wholesale", func(t *testing.T) {
		root := &ClaudeConfig{AutoMode: &ClaudeAutoMode{Allow: []string{"RootRule"}}}
		member := &ClaudeConfig{
			Inherit:  boolPtr(false),
			AutoMode: &ClaudeAutoMode{Allow: []string{"MemberRule"}},
		}
		root.Merge(member)
		if !reflect.DeepEqual(root.AutoMode.Allow, []string{"MemberRule"}) {
			t.Errorf("expected autoMode.allow overwritten to [MemberRule], got %v", root.AutoMode.Allow)
		}
	})
}

// TestMerge_AutoModeGapAdopt confirms a nil root autoMode adopts the member's
// (deep copy, not aliased), and that a member with no autoMode leaves the root's
// intact.
func TestMerge_AutoModeGapAdopt(t *testing.T) {
	t.Run("nil root adopts member (deep copy)", func(t *testing.T) {
		root := &ClaudeConfig{}
		member := &ClaudeConfig{AutoMode: &ClaudeAutoMode{Allow: []string{"MemberRule"}}}
		root.Merge(member)
		if root.AutoMode == nil || !reflect.DeepEqual(root.AutoMode.Allow, []string{"MemberRule"}) {
			t.Fatalf("expected root to adopt member autoMode, got %v", root.AutoMode)
		}
		// Mutating the member's slice must not affect the adopted copy.
		member.AutoMode.Allow[0] = "Mutated"
		if root.AutoMode.Allow[0] != "MemberRule" {
			t.Errorf("expected deep copy, adopted slice aliased the member's")
		}
	})

	t.Run("nil member leaves root intact", func(t *testing.T) {
		root := &ClaudeConfig{AutoMode: &ClaudeAutoMode{Allow: []string{"RootRule"}}}
		root.Merge(&ClaudeConfig{})
		if root.AutoMode == nil || !reflect.DeepEqual(root.AutoMode.Allow, []string{"RootRule"}) {
			t.Errorf("expected root autoMode preserved, got %v", root.AutoMode)
		}
	})
}

// TestMerge_UseAutoModeDuringPlanGapFill confirms useAutoModeDuringPlan is a
// root-wins-gap *bool: a member fills a nil root slot, an explicit root value
// (including false) survives.
func TestMerge_UseAutoModeDuringPlanGapFill(t *testing.T) {
	t.Run("root nil + member true -> true", func(t *testing.T) {
		root := &ClaudeConfig{}
		member := &ClaudeConfig{UseAutoModeDuringPlan: boolPtr(true)}
		root.Merge(member)
		if root.UseAutoModeDuringPlan == nil || !*root.UseAutoModeDuringPlan {
			t.Errorf("expected member to fill nil root, got %v", root.UseAutoModeDuringPlan)
		}
	})

	t.Run("explicit root false wins over member true", func(t *testing.T) {
		root := &ClaudeConfig{UseAutoModeDuringPlan: boolPtr(false)}
		member := &ClaudeConfig{UseAutoModeDuringPlan: boolPtr(true)}
		root.Merge(member)
		if root.UseAutoModeDuringPlan == nil || *root.UseAutoModeDuringPlan {
			t.Errorf("expected explicit root false to win, got %v", root.UseAutoModeDuringPlan)
		}
	})
}

// TestAutoModeInIsEmpty confirms autoMode (with any non-empty section) and
// useAutoModeDuringPlan each count as content (ShouldSeed fires), while an
// autoMode present-but-all-empty is treated as unset.
func TestAutoModeInIsEmpty(t *testing.T) {
	t.Run("autoMode with content is non-empty", func(t *testing.T) {
		c := &ClaudeConfig{AutoMode: &ClaudeAutoMode{HardDeny: []string{"Bash(rm:*)"}}}
		if c.IsEmpty() {
			t.Errorf("expected autoMode with a section to be non-empty")
		}
		if !c.ShouldSeed() {
			t.Errorf("expected autoMode-only config to ShouldSeed")
		}
	})

	t.Run("useAutoModeDuringPlan-only is non-empty", func(t *testing.T) {
		c := &ClaudeConfig{UseAutoModeDuringPlan: boolPtr(false)}
		if c.IsEmpty() {
			t.Errorf("expected useAutoModeDuringPlan set to be non-empty")
		}
		if !c.ShouldSeed() {
			t.Errorf("expected useAutoModeDuringPlan-only config to ShouldSeed")
		}
	})

	t.Run("autoMode present but all-empty is treated as unset", func(t *testing.T) {
		c := &ClaudeConfig{AutoMode: &ClaudeAutoMode{}}
		if !c.IsEmpty() {
			t.Errorf("expected an all-empty autoMode to be empty (no forced write)")
		}
	})
}

// TestDecode_AutoModeRoundTrips confirms a TOML [claude.autoMode]-style block
// decodes into ClaudeAutoMode via the snake_case toml keys, preserving the
// "$defaults" token verbatim.
func TestDecode_AutoModeRoundTrips(t *testing.T) {
	src := `
useAutoModeDuringPlan = true

[autoMode]
allow = ["$defaults", "Bash(git:*)"]
soft_deny = ["Read(secret)"]
environment = ["CI=1"]
hard_deny = ["Bash(rm:*)"]
`
	var cfg ClaudeConfig
	if err := toml.Unmarshal([]byte(src), &cfg); err != nil {
		t.Fatalf("toml unmarshal: %v", err)
	}
	if cfg.AutoMode == nil {
		t.Fatal("expected autoMode decoded, got nil")
	}
	if !reflect.DeepEqual(cfg.AutoMode.Allow, []string{"$defaults", "Bash(git:*)"}) {
		t.Errorf("expected allow with $defaults preserved, got %v", cfg.AutoMode.Allow)
	}
	if !reflect.DeepEqual(cfg.AutoMode.SoftDeny, []string{"Read(secret)"}) {
		t.Errorf("expected soft_deny decoded via snake_case key, got %v", cfg.AutoMode.SoftDeny)
	}
	if !reflect.DeepEqual(cfg.AutoMode.HardDeny, []string{"Bash(rm:*)"}) {
		t.Errorf("expected hard_deny decoded via snake_case key, got %v", cfg.AutoMode.HardDeny)
	}
	if !reflect.DeepEqual(cfg.AutoMode.Environment, []string{"CI=1"}) {
		t.Errorf("expected environment decoded, got %v", cfg.AutoMode.Environment)
	}
	if cfg.UseAutoModeDuringPlan == nil || !*cfg.UseAutoModeDuringPlan {
		t.Errorf("expected useAutoModeDuringPlan=true, got %v", cfg.UseAutoModeDuringPlan)
	}
}

// ============================================================================
// sandbox escape-hatch lock: allowUnsandboxedCommands + excludedCommands (Part C)
// ============================================================================

// TestMerge_AllowUnsandboxedCommandsGapFill confirms allowUnsandboxedCommands is
// a root-wins-gap *bool mirroring Sandbox.Enabled. Critically, an explicit
// false (the lock) must survive and never be confused with unset.
func TestMerge_AllowUnsandboxedCommandsGapFill(t *testing.T) {
	t.Run("root nil + member false -> false fills gap", func(t *testing.T) {
		root := &ClaudeConfig{}
		member := &ClaudeConfig{Sandbox: ClaudeSandbox{AllowUnsandboxedCommands: boolPtr(false)}}
		root.Merge(member)
		if root.Sandbox.AllowUnsandboxedCommands == nil {
			t.Fatal("expected member false to fill nil root, got nil")
		}
		if *root.Sandbox.AllowUnsandboxedCommands {
			t.Errorf("expected explicit false to survive, got true")
		}
	})

	t.Run("explicit root false wins over member true", func(t *testing.T) {
		root := &ClaudeConfig{Sandbox: ClaudeSandbox{AllowUnsandboxedCommands: boolPtr(false)}}
		member := &ClaudeConfig{Sandbox: ClaudeSandbox{AllowUnsandboxedCommands: boolPtr(true)}}
		root.Merge(member)
		if root.Sandbox.AllowUnsandboxedCommands == nil || *root.Sandbox.AllowUnsandboxedCommands {
			t.Errorf("expected explicit root false (the lock) to win, got %v", root.Sandbox.AllowUnsandboxedCommands)
		}
	})

	t.Run("root false survives merge with nil member", func(t *testing.T) {
		root := &ClaudeConfig{Sandbox: ClaudeSandbox{AllowUnsandboxedCommands: boolPtr(false)}}
		root.Merge(&ClaudeConfig{})
		if root.Sandbox.AllowUnsandboxedCommands == nil || *root.Sandbox.AllowUnsandboxedCommands {
			t.Errorf("expected root false to survive, got %v", root.Sandbox.AllowUnsandboxedCommands)
		}
	})
}

// TestMerge_ExcludedCommandsUnion confirms sandbox.excludedCommands unions across
// layers by default and is replaced wholesale under inherit=false, mirroring
// allowedDomains.
func TestMerge_ExcludedCommandsUnion(t *testing.T) {
	t.Run("union by default", func(t *testing.T) {
		root := &ClaudeConfig{Sandbox: ClaudeSandbox{ExcludedCommands: []string{"git", "docker"}}}
		member := &ClaudeConfig{Sandbox: ClaudeSandbox{ExcludedCommands: []string{"docker", "flow"}}}
		root.Merge(member)
		want := []string{"git", "docker", "flow"}
		if !reflect.DeepEqual(root.Sandbox.ExcludedCommands, want) {
			t.Errorf("expected union %v, got %v", want, root.Sandbox.ExcludedCommands)
		}
	})

	t.Run("inherit=false replaces wholesale", func(t *testing.T) {
		root := &ClaudeConfig{Sandbox: ClaudeSandbox{ExcludedCommands: []string{"git"}}}
		member := &ClaudeConfig{
			Inherit: boolPtr(false),
			Sandbox: ClaudeSandbox{ExcludedCommands: []string{"flow"}},
		}
		root.Merge(member)
		if !reflect.DeepEqual(root.Sandbox.ExcludedCommands, []string{"flow"}) {
			t.Errorf("expected excludedCommands overwritten to [flow], got %v", root.Sandbox.ExcludedCommands)
		}
	})
}

// TestAllowUnsandboxedInIsEmpty confirms an allowUnsandboxedCommands=false-only
// profile counts as content (ShouldSeed fires) — explicit false must NOT be
// treated as absent — and likewise for an excludedCommands-only profile.
func TestAllowUnsandboxedInIsEmpty(t *testing.T) {
	lockOnly := &ClaudeConfig{Sandbox: ClaudeSandbox{AllowUnsandboxedCommands: boolPtr(false)}}
	if lockOnly.IsEmpty() {
		t.Errorf("expected allowUnsandboxedCommands=false-only config to be non-empty")
	}
	if !lockOnly.ShouldSeed() {
		t.Errorf("expected allowUnsandboxedCommands=false-only config to ShouldSeed")
	}

	excludedOnly := &ClaudeConfig{Sandbox: ClaudeSandbox{ExcludedCommands: []string{"git"}}}
	if excludedOnly.IsEmpty() {
		t.Errorf("expected excludedCommands-only config to be non-empty")
	}

	if !(&ClaudeConfig{}).IsEmpty() {
		t.Errorf("expected an all-zero config to be empty")
	}
}

// TestDecode_MergeSubTableIgnored confirms a reserved [claude.merge] sub-table
// decodes without error and is ignored (mapstructure does not ErrorUnused).
func TestDecode_MergeSubTableIgnored(t *testing.T) {
	raw := map[string]interface{}{
		"inherit": true,
		"merge": map[string]interface{}{
			"permissions": "union",
		},
		"permissions": map[string]interface{}{"allow": []interface{}{"X(x:*)"}},
	}

	var cfg ClaudeConfig
	decoder, _ := mapstructure.NewDecoder(&mapstructure.DecoderConfig{Result: &cfg, TagName: "yaml"})
	if err := decoder.Decode(raw); err != nil {
		t.Fatalf("expected decode to ignore [claude.merge], got error: %v", err)
	}
	if !reflect.DeepEqual(cfg.Permissions.Allow, []string{"X(x:*)"}) {
		t.Errorf("expected allow decoded alongside ignored merge sub-table, got %v", cfg.Permissions.Allow)
	}
}

// ============================================================================
// Expanded managed fields (model/effortLevel/editorMode/tui, top-level bools,
// enabledPlugins, sandbox.filesystem.denyRead)
// ============================================================================

// TestMerge_ScalarSettingsGapFill confirms the four new scalar strings are
// root-wins-gap scalars like defaultMode: a member fills an empty root slot,
// and an explicit root value wins.
func TestMerge_ScalarSettingsGapFill(t *testing.T) {
	t.Run("member fills empty root", func(t *testing.T) {
		root := &ClaudeConfig{}
		member := &ClaudeConfig{Model: "opus", EffortLevel: "high", EditorMode: "vim", TUI: "auto"}
		root.Merge(member)
		if root.Model != "opus" || root.EffortLevel != "high" || root.EditorMode != "vim" || root.TUI != "auto" {
			t.Errorf("expected member to fill empty root scalars, got %+v", root)
		}
	})

	t.Run("root wins over member", func(t *testing.T) {
		root := &ClaudeConfig{Model: "sonnet", EffortLevel: "low", EditorMode: "normal", TUI: "plain"}
		member := &ClaudeConfig{Model: "opus", EffortLevel: "high", EditorMode: "vim", TUI: "auto"}
		root.Merge(member)
		if root.Model != "sonnet" || root.EffortLevel != "low" || root.EditorMode != "normal" || root.TUI != "plain" {
			t.Errorf("expected explicit root scalars to win, got %+v", root)
		}
	})
}

// TestMerge_TopLevelBoolsGapFill confirms the three new top-level bools follow
// the sandbox-bool root-wins gap-fill.
func TestMerge_TopLevelBoolsGapFill(t *testing.T) {
	t.Run("member fills nil root", func(t *testing.T) {
		root := &ClaudeConfig{}
		member := &ClaudeConfig{
			SkipDangerousModePermissionPrompt: boolPtr(true),
			SkipWorkflowUsageWarning:          boolPtr(true),
			AgentPushNotifEnabled:             boolPtr(true),
		}
		root.Merge(member)
		if root.SkipDangerousModePermissionPrompt == nil || !*root.SkipDangerousModePermissionPrompt ||
			root.SkipWorkflowUsageWarning == nil || !*root.SkipWorkflowUsageWarning ||
			root.AgentPushNotifEnabled == nil || !*root.AgentPushNotifEnabled {
			t.Errorf("expected member to fill nil root bools, got %+v", root)
		}
	})

	t.Run("explicit root false wins", func(t *testing.T) {
		root := &ClaudeConfig{SkipDangerousModePermissionPrompt: boolPtr(false)}
		member := &ClaudeConfig{SkipDangerousModePermissionPrompt: boolPtr(true)}
		root.Merge(member)
		if root.SkipDangerousModePermissionPrompt == nil || *root.SkipDangerousModePermissionPrompt {
			t.Errorf("expected explicit root false to win, got %v", root.SkipDangerousModePermissionPrompt)
		}
	})
}

// TestMerge_EnabledPluginsUnion confirms enabledPlugins unions per key with
// root winning on collision, and is replaced wholesale under inherit=false.
func TestMerge_EnabledPluginsUnion(t *testing.T) {
	t.Run("union per key, root wins collisions", func(t *testing.T) {
		root := &ClaudeConfig{EnabledPlugins: map[string]bool{"shared@p": false, "root@p": true}}
		member := &ClaudeConfig{EnabledPlugins: map[string]bool{"shared@p": true, "member@p": true}}
		root.Merge(member)
		want := map[string]bool{"shared@p": false, "root@p": true, "member@p": true}
		if !reflect.DeepEqual(root.EnabledPlugins, want) {
			t.Errorf("expected %v, got %v", want, root.EnabledPlugins)
		}
	})

	t.Run("inherit=false replaces wholesale", func(t *testing.T) {
		root := &ClaudeConfig{EnabledPlugins: map[string]bool{"root@p": true}}
		member := &ClaudeConfig{Inherit: boolPtr(false), EnabledPlugins: map[string]bool{"member@p": true}}
		root.Merge(member)
		want := map[string]bool{"member@p": true}
		if !reflect.DeepEqual(root.EnabledPlugins, want) {
			t.Errorf("expected wholesale replace to %v, got %v", want, root.EnabledPlugins)
		}
	})

	t.Run("inherit=false with no member map clears", func(t *testing.T) {
		root := &ClaudeConfig{EnabledPlugins: map[string]bool{"root@p": true}}
		member := &ClaudeConfig{Inherit: boolPtr(false)}
		root.Merge(member)
		if len(root.EnabledPlugins) != 0 {
			t.Errorf("expected cleared map, got %v", root.EnabledPlugins)
		}
	})
}

// TestMerge_DenyReadUnion confirms sandbox.filesystem.denyRead unions like the
// other filesystem arrays and is replaced wholesale under inherit=false.
func TestMerge_DenyReadUnion(t *testing.T) {
	t.Run("union by default", func(t *testing.T) {
		root := &ClaudeConfig{Sandbox: ClaudeSandbox{Filesystem: ClaudeSandboxFilesystem{DenyRead: []string{"/a", "/shared"}}}}
		member := &ClaudeConfig{Sandbox: ClaudeSandbox{Filesystem: ClaudeSandboxFilesystem{DenyRead: []string{"/shared", "/b"}}}}
		root.Merge(member)
		want := []string{"/a", "/shared", "/b"}
		if !reflect.DeepEqual(root.Sandbox.Filesystem.DenyRead, want) {
			t.Errorf("expected union %v, got %v", want, root.Sandbox.Filesystem.DenyRead)
		}
	})

	t.Run("inherit=false replaces wholesale", func(t *testing.T) {
		root := &ClaudeConfig{Sandbox: ClaudeSandbox{Filesystem: ClaudeSandboxFilesystem{DenyRead: []string{"/root-only"}}}}
		member := &ClaudeConfig{Inherit: boolPtr(false), Sandbox: ClaudeSandbox{Filesystem: ClaudeSandboxFilesystem{DenyRead: []string{"/member-only"}}}}
		root.Merge(member)
		if !reflect.DeepEqual(root.Sandbox.Filesystem.DenyRead, []string{"/member-only"}) {
			t.Errorf("expected wholesale replace, got %v", root.Sandbox.Filesystem.DenyRead)
		}
	})
}

// TestNewManagedFieldsInIsEmpty confirms each new field counts as content in
// IsEmpty (they all map to written settings keys, unlike the lone flags).
func TestNewManagedFieldsInIsEmpty(t *testing.T) {
	cases := map[string]*ClaudeConfig{
		"model":                             {Model: "opus"},
		"effortLevel":                       {EffortLevel: "high"},
		"editorMode":                        {EditorMode: "vim"},
		"tui":                               {TUI: "auto"},
		"skipDangerousModePermissionPrompt": {SkipDangerousModePermissionPrompt: boolPtr(false)},
		"skipWorkflowUsageWarning":          {SkipWorkflowUsageWarning: boolPtr(false)},
		"agentPushNotifEnabled":             {AgentPushNotifEnabled: boolPtr(false)},
		"enabledPlugins":                    {EnabledPlugins: map[string]bool{"p@m": true}},
		"denyRead":                          {Sandbox: ClaudeSandbox{Filesystem: ClaudeSandboxFilesystem{DenyRead: []string{"/x"}}}},
	}
	for name, cfg := range cases {
		if cfg.IsEmpty() {
			t.Errorf("expected config with only %s set to be non-empty", name)
		}
	}
}

// TestDecode_NewManagedFieldsRoundTrip confirms the new fields decode from a
// grove.toml [claude] block through the same toml -> mapstructure path
// UnmarshalExtension uses.
func TestDecode_NewManagedFieldsRoundTrip(t *testing.T) {
	src := `
model = "opus"
effortLevel = "high"
editorMode = "vim"
tui = "auto"
skipDangerousModePermissionPrompt = true
skipWorkflowUsageWarning = false
agentPushNotifEnabled = true

[enabledPlugins]
"acme@marketplace" = true

[sandbox.filesystem]
denyRead = ["/Users/dev/secrets"]
`
	var raw map[string]interface{}
	if err := toml.Unmarshal([]byte(src), &raw); err != nil {
		t.Fatalf("toml unmarshal: %v", err)
	}

	var cfg ClaudeConfig
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{Result: &cfg, TagName: "yaml"})
	if err != nil {
		t.Fatalf("new decoder: %v", err)
	}
	if err := decoder.Decode(raw); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if cfg.Model != "opus" || cfg.EffortLevel != "high" || cfg.EditorMode != "vim" || cfg.TUI != "auto" {
		t.Errorf("scalar fields did not round-trip: %+v", cfg)
	}
	if cfg.SkipDangerousModePermissionPrompt == nil || !*cfg.SkipDangerousModePermissionPrompt {
		t.Errorf("expected skipDangerousModePermissionPrompt=true, got %v", cfg.SkipDangerousModePermissionPrompt)
	}
	if cfg.SkipWorkflowUsageWarning == nil || *cfg.SkipWorkflowUsageWarning {
		t.Errorf("expected skipWorkflowUsageWarning=false, got %v", cfg.SkipWorkflowUsageWarning)
	}
	if cfg.AgentPushNotifEnabled == nil || !*cfg.AgentPushNotifEnabled {
		t.Errorf("expected agentPushNotifEnabled=true, got %v", cfg.AgentPushNotifEnabled)
	}
	if !reflect.DeepEqual(cfg.EnabledPlugins, map[string]bool{"acme@marketplace": true}) {
		t.Errorf("expected enabledPlugins decoded, got %v", cfg.EnabledPlugins)
	}
	if !reflect.DeepEqual(cfg.Sandbox.Filesystem.DenyRead, []string{"/Users/dev/secrets"}) {
		t.Errorf("expected denyRead decoded, got %v", cfg.Sandbox.Filesystem.DenyRead)
	}
}
