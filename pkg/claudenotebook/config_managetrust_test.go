package claudenotebook

import (
	"testing"

	"github.com/mitchellh/mapstructure"
)

// TestMerge_ManageTrustGapFill confirms manageTrust is a root-wins-gap scalar:
// a member fills a nil root slot, but an explicit root value survives the merge
// (same semantics as allowGroveTools/protectConfig).
func TestMerge_ManageTrustGapFill(t *testing.T) {
	t.Run("root nil + member true -> true", func(t *testing.T) {
		root := &ClaudeConfig{}
		member := &ClaudeConfig{ManageTrust: boolPtr(true)}
		root.Merge(member)
		if root.ManageTrust == nil || !*root.ManageTrust {
			t.Errorf("expected member to fill nil root to true, got %v", root.ManageTrust)
		}
	})

	t.Run("root false + member true -> stays false", func(t *testing.T) {
		root := &ClaudeConfig{ManageTrust: boolPtr(false)}
		member := &ClaudeConfig{ManageTrust: boolPtr(true)}
		root.Merge(member)
		if root.ManageTrust == nil || *root.ManageTrust {
			t.Errorf("expected explicit root false to win, got %v", root.ManageTrust)
		}
	})

	t.Run("root true survives merge with nil member", func(t *testing.T) {
		root := &ClaudeConfig{ManageTrust: boolPtr(true)}
		member := &ClaudeConfig{}
		root.Merge(member)
		if root.ManageTrust == nil || !*root.ManageTrust {
			t.Errorf("expected root manageTrust=true to survive, got %v", root.ManageTrust)
		}
	})

	t.Run("inherit=false member still flows manageTrust up (outside array branch)", func(t *testing.T) {
		root := &ClaudeConfig{}
		member := &ClaudeConfig{Inherit: boolPtr(false), ManageTrust: boolPtr(true)}
		root.Merge(member)
		if root.ManageTrust == nil || !*root.ManageTrust {
			t.Errorf("expected manageTrust to gap-fill regardless of inherit=false, got %v", root.ManageTrust)
		}
	})
}

// TestManageTrustNotInIsEmpty confirms a manageTrust-only profile is treated as
// empty for settings purposes: IsEmpty()==true and ShouldSeed()==false, so it
// never triggers a .claude/settings.local.json write. Trust is gated separately
// via ManagesTrust().
func TestManageTrustNotInIsEmpty(t *testing.T) {
	for _, v := range []*bool{boolPtr(true), boolPtr(false)} {
		c := &ClaudeConfig{ManageTrust: v}
		if !c.IsEmpty() {
			t.Errorf("expected a config with only manageTrust=%v to be IsEmpty()==true", *v)
		}
		if c.ShouldSeed() {
			t.Errorf("expected a config with only manageTrust=%v to be ShouldSeed()==false", *v)
		}
	}
}

// TestManagesTrust unit-tests the accessor across nil receiver / unset / false /
// true.
func TestManagesTrust(t *testing.T) {
	var nilCfg *ClaudeConfig
	if nilCfg.ManagesTrust() {
		t.Errorf("expected nil receiver ManagesTrust()==false")
	}
	if (&ClaudeConfig{}).ManagesTrust() {
		t.Errorf("expected unset manageTrust ManagesTrust()==false")
	}
	if (&ClaudeConfig{ManageTrust: boolPtr(false)}).ManagesTrust() {
		t.Errorf("expected manageTrust=false ManagesTrust()==false")
	}
	if !(&ClaudeConfig{ManageTrust: boolPtr(true)}).ManagesTrust() {
		t.Errorf("expected manageTrust=true ManagesTrust()==true")
	}
}

// TestDecode_ManageTrustRoundTrips confirms a raw map decodes `manageTrust` into
// the *bool field (mirrors UnmarshalExtension's mapstructure/yaml-tag decode).
func TestDecode_ManageTrustRoundTrips(t *testing.T) {
	raw := map[string]interface{}{
		"manageTrust": true,
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
	if cfg.ManageTrust == nil {
		t.Fatal("expected manageTrust decoded into *bool, got nil")
	}
	if !*cfg.ManageTrust {
		t.Errorf("expected manageTrust=true, got %v", *cfg.ManageTrust)
	}
	if !cfg.ManagesTrust() {
		t.Errorf("expected ManagesTrust()==true after decode")
	}
}

// TestDecode_ManageTrustAbsentLeavesNil confirms an absent manageTrust key
// leaves the pointer nil (unset), so ManagesTrust() is false (opt-in default).
func TestDecode_ManageTrustAbsentLeavesNil(t *testing.T) {
	raw := map[string]interface{}{
		"permissions": map[string]interface{}{"allow": []interface{}{"X(x:*)"}},
	}

	var cfg ClaudeConfig
	decoder, _ := mapstructure.NewDecoder(&mapstructure.DecoderConfig{Result: &cfg, TagName: "yaml"})
	if err := decoder.Decode(raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if cfg.ManageTrust != nil {
		t.Errorf("expected manageTrust nil when absent, got %v", *cfg.ManageTrust)
	}
	if cfg.ManagesTrust() {
		t.Errorf("expected ManagesTrust()==false when manageTrust absent")
	}
}
