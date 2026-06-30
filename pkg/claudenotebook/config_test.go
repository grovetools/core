package claudenotebook

import (
	"reflect"
	"testing"

	"github.com/mitchellh/mapstructure"
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
