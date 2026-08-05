package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func validWireGuardConfig() *WireGuardConfig {
	on := true
	return &WireGuardConfig{
		Enabled:      &on,
		Address:      "10.100.0.7/24",
		Endpoint:     "hub.example.com:51820",
		HubPublicKey: base64.StdEncoding.EncodeToString(make([]byte, 32)),
	}
}

func TestWireGuardConfigDefaultsAndEnabled(t *testing.T) {
	cfg := validWireGuardConfig()
	if !cfg.IsEnabled() {
		t.Fatal("complete enabled block reports disabled")
	}
	if got := cfg.AllowedIPsOrDefault(); len(got) != 1 || got[0] != "10.100.0.0/24" {
		t.Fatalf("AllowedIPsOrDefault = %v", got)
	}
	if got := cfg.EffectivePersistentKeepalive(); got != 25 {
		t.Fatalf("EffectivePersistentKeepalive = %d, want 25", got)
	}

	off := false
	cfg.Enabled = &off
	if cfg.IsEnabled() {
		t.Fatal("explicitly disabled block reports enabled")
	}
	cfg.Enabled = nil
	if cfg.IsEnabled() {
		t.Fatal("block without enabled=true reports enabled")
	}
}

func TestWireGuardConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*WireGuardConfig)
		wantErr string
	}{
		{"valid hostname endpoint", func(*WireGuardConfig) {}, ""},
		{"valid ipv4 endpoint", func(c *WireGuardConfig) { c.Endpoint = "192.0.2.1:51820" }, ""},
		{"valid ipv6 endpoint", func(c *WireGuardConfig) { c.Endpoint = "[2001:db8::1]:51820" }, ""},
		{"missing address", func(c *WireGuardConfig) { c.Address = "" }, "address is required"},
		{"bad address", func(c *WireGuardConfig) { c.Address = "10.0.0.1" }, "not a valid CIDR"},
		{"bad endpoint", func(c *WireGuardConfig) { c.Endpoint = "hub.example.com" }, "host:port"},
		{"bad endpoint port", func(c *WireGuardConfig) { c.Endpoint = "hub.example.com:nope" }, "invalid port"},
		{"endpoint newline", func(c *WireGuardConfig) { c.Endpoint = "hub.example.com:51820\nPostUp=x" }, "host:port"},
		{"bad dns", func(c *WireGuardConfig) { c.DNS = "resolver.example.com" }, "not an IP address"},
		{"bad key base64", func(c *WireGuardConfig) { c.HubPublicKey = "%%%" }, "valid base64"},
		{"short key", func(c *WireGuardConfig) { c.HubPublicKey = base64.StdEncoding.EncodeToString(make([]byte, 31)) }, "want 32"},
		{"bad allowed cidr", func(c *WireGuardConfig) { c.AllowedIPs = []string{"not-a-cidr"} }, "not a valid CIDR"},
		{"ipv4 full tunnel", func(c *WireGuardConfig) { c.AllowedIPs = []string{"0.0.0.0/0"} }, "whole internet"},
		{"ipv6 full tunnel", func(c *WireGuardConfig) { c.AllowedIPs = []string{"::/0"} }, "whole internet"},
		{"keepalive low", func(c *WireGuardConfig) { c.PersistentKeepalive = -1 }, "between 1 and 3600"},
		{"keepalive high", func(c *WireGuardConfig) { c.PersistentKeepalive = 3601 }, "between 1 and 3600"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validWireGuardConfig()
			tc.mutate(cfg)
			err := cfg.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate = %v, want error containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestWireGuardDisabledBlockIsParseOnly(t *testing.T) {
	off := false
	cfg := &WireGuardConfig{Enabled: &off, Address: "not-cidr"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("disabled block validation = %v", err)
	}

	src := `version = "1.0"
[forge.wireguard]
enabled = true
address = "not-cidr"
`
	loaded, err := LoadFromTOMLBytes([]byte(src))
	if err != nil {
		t.Fatalf("parse-only load rejected malformed wireguard block: %v", err)
	}
	forge, err := loaded.Forge()
	if err != nil {
		t.Fatal(err)
	}
	if err := forge.Validate(); err == nil {
		t.Fatal("explicit use-time validation accepted malformed enabled block")
	}
}
