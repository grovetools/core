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
