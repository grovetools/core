package config

//go:generate sh -c "cd .. && go run ./tools/forge-schema-generator/"

import (
	"encoding/base64"
	"fmt"
	"net"
	"strconv"
	"strings"
)

const DefaultWireGuardPersistentKeepalive = 25

// WireGuardConfig describes one client of an owner-managed WireGuard hub.
// It intentionally contains no private key: keys are generated and retained on
// the client machine by the command that converges this configuration.
type WireGuardConfig struct {
	Enabled             *bool    `yaml:"enabled,omitempty" toml:"enabled,omitempty" jsonschema:"description=Join this machine to the owner-managed WireGuard mesh,default=false"`
	Address             string   `yaml:"address,omitempty" toml:"address,omitempty" jsonschema:"description=Owner-assigned WireGuard interface address in CIDR form"`
	Endpoint            string   `yaml:"endpoint,omitempty" toml:"endpoint,omitempty" jsonschema:"description=WireGuard hub endpoint as host:port"`
	HubPublicKey        string   `yaml:"hub_public_key,omitempty" toml:"hub_public_key,omitempty" jsonschema:"description=Base64 WireGuard public key of the hub"`
	AllowedIPs          []string `yaml:"allowed_ips,omitempty" toml:"allowed_ips,omitempty" jsonschema:"description=Routes sent through the hub (defaults to the subnet containing address)"`
	DNS                 string   `yaml:"dns,omitempty" toml:"dns,omitempty" jsonschema:"description=Optional DNS server for wg-quick (empty by default)"`
	PersistentKeepalive int      `yaml:"persistent_keepalive,omitempty" toml:"persistent_keepalive,omitempty" jsonschema:"description=WireGuard keepalive interval in seconds,default=25,minimum=1,maximum=3600"`
}

// IsEnabled reports whether the block explicitly opts in and contains the
// required public routing facts. Validate explains an incomplete enabled block.
func (w *WireGuardConfig) IsEnabled() bool {
	return w != nil && w.Enabled != nil && *w.Enabled &&
		strings.TrimSpace(w.Address) != "" &&
		strings.TrimSpace(w.Endpoint) != "" &&
		strings.TrimSpace(w.HubPublicKey) != ""
}

// EffectivePersistentKeepalive returns the safe client default when unset.
func (w *WireGuardConfig) EffectivePersistentKeepalive() int {
	if w == nil || w.PersistentKeepalive == 0 {
		return DefaultWireGuardPersistentKeepalive
	}
	return w.PersistentKeepalive
}

// AllowedIPsOrDefault returns configured routes or the network containing
// Address. Invalid addresses are left for Validate to report.
func (w *WireGuardConfig) AllowedIPsOrDefault() []string {
	if w == nil {
		return nil
	}
	if len(w.AllowedIPs) > 0 {
		return append([]string(nil), w.AllowedIPs...)
	}
	_, network, err := net.ParseCIDR(strings.TrimSpace(w.Address))
	if err != nil {
		return nil
	}
	return []string{network.String()}
}

// Validate checks an enabled WireGuard block at use time. Configuration loading
// remains parse-only, matching the rest of [forge].
func (w *WireGuardConfig) Validate() error {
	if w == nil || w.Enabled == nil || !*w.Enabled {
		return nil
	}
	address := strings.TrimSpace(w.Address)
	if address == "" {
		return fmt.Errorf("wireguard address is required when enabled")
	}
	if _, _, err := net.ParseCIDR(address); err != nil {
		return fmt.Errorf("wireguard address %q is not a valid CIDR: %w", w.Address, err)
	}
	endpoint := strings.TrimSpace(w.Endpoint)
	if strings.ContainsAny(endpoint, " \t\r\n") {
		return fmt.Errorf("wireguard endpoint %q is not a host:port address", w.Endpoint)
	}
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil || strings.TrimSpace(host) == "" || strings.TrimSpace(port) == "" {
		return fmt.Errorf("wireguard endpoint %q is not a host:port address", w.Endpoint)
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("wireguard endpoint %q has an invalid port", w.Endpoint)
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(w.HubPublicKey))
	if err != nil {
		return fmt.Errorf("wireguard hub_public_key is not valid base64: %w", err)
	}
	if len(key) != 32 {
		return fmt.Errorf("wireguard hub_public_key decodes to %d bytes, want 32", len(key))
	}
	if dns := strings.TrimSpace(w.DNS); dns != "" && net.ParseIP(dns) == nil {
		return fmt.Errorf("wireguard dns %q is not an IP address", w.DNS)
	}
	for _, cidr := range w.AllowedIPsOrDefault() {
		cidr = strings.TrimSpace(cidr)
		if cidr == "0.0.0.0/0" || cidr == "::/0" {
			return fmt.Errorf("wireguard allowed_ips %q is the whole internet — restrict it to the mesh subnet (e.g. 10.0.0.0/24)", cidr)
		}
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("wireguard allowed_ips %q is not a valid CIDR: %w", cidr, err)
		}
	}
	keepalive := w.EffectivePersistentKeepalive()
	if keepalive < 1 || keepalive > 3600 {
		return fmt.Errorf("wireguard persistent_keepalive %d must be between 1 and 3600 seconds", w.PersistentKeepalive)
	}
	return nil
}
