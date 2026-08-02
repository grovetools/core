package config

import (
	"fmt"
	"net/url"
	"strings"
)

// ForgeExtensionKey is the top-level config key holding the self-hosted forge
// block:
//
//	[forge]
//	url = "https://forge.example.com"
//	remote_name = "forge"
//	token_command = "op read op://grove/forge/token"
//
// It is an extension namespace rather than a core Config field, so it parses
// through the existing Extensions path with no change to the config loader,
// the schema, or the serialization round-trip.
const ForgeExtensionKey = "forge"

// DefaultForgeRemoteName is the git remote name a forge is enrolled under when
// [forge] does not say otherwise. It is deliberately not "origin": a forge
// remote sits alongside the upstream, it does not replace it.
const DefaultForgeRemoteName = "forge"

// ForgeConfig is the typed [forge] block.
//
// This is parse-only support. Nothing in this wave acts on it: no code
// contacts the URL, enrolls the remote, or executes TokenCommand. In
// particular, resolving TokenCommand is the daemon's job — core/pkg/forge
// accepts an already-resolved token and never shells out for one — so this
// struct carries the string and nothing more.
type ForgeConfig struct {
	// URL is the base URL of the self-hosted forge instance.
	URL string `yaml:"url,omitempty" toml:"url,omitempty" jsonschema:"description=Base URL of the self-hosted forge instance"`
	// RemoteName is the git remote the forge is enrolled under. Empty means
	// DefaultForgeRemoteName; see EffectiveRemoteName.
	RemoteName string `yaml:"remote_name,omitempty" toml:"remote_name,omitempty" jsonschema:"description=Git remote name for the forge,default=forge"`
	// TokenCommand is a shell command whose trimmed stdout is the forge API
	// token. It is stored, never executed here.
	TokenCommand string `yaml:"token_command,omitempty" toml:"token_command,omitempty" jsonschema:"description=Shell command that prints the forge API token" jsonschema_extras:"x-sensitive=true,x-hint=Prefer token_command over embedding a token in config"`
}

// EffectiveRemoteName resolves RemoteName against the default.
func (f *ForgeConfig) EffectiveRemoteName() string {
	if f == nil || strings.TrimSpace(f.RemoteName) == "" {
		return DefaultForgeRemoteName
	}
	return strings.TrimSpace(f.RemoteName)
}

// IsConfigured reports whether a usable forge is declared. An empty or
// missing block means "no forge", which is the normal state for every
// worktree today.
func (f *ForgeConfig) IsConfigured() bool {
	return f != nil && strings.TrimSpace(f.URL) != ""
}

// Validate checks structural validity. Like SyncConfig.Validate it is NOT
// called on the config load path: a key nothing consumes yet must never start
// failing config loads. Consumers that activate the forge call it explicitly.
func (f *ForgeConfig) Validate() error {
	if f == nil {
		return nil
	}
	if raw := strings.TrimSpace(f.URL); raw != "" {
		u, err := url.Parse(raw)
		if err != nil {
			return fmt.Errorf("forge url %q is not a valid URL: %w", f.URL, err)
		}
		switch strings.ToLower(u.Scheme) {
		case "http", "https":
		default:
			return fmt.Errorf("forge url %q must use http or https, got scheme %q", f.URL, u.Scheme)
		}
		if u.Host == "" {
			return fmt.Errorf("forge url %q has no host", f.URL)
		}
	}
	if name := strings.TrimSpace(f.RemoteName); name != "" {
		if err := validateRemoteName(name); err != nil {
			return err
		}
	}
	return nil
}

// validateRemoteName rejects names git itself would reject, and names that
// would be read as flags by the git commands that eventually carry them.
func validateRemoteName(name string) error {
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("forge remote_name %q may not start with '-'", name)
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.', r == '_', r == '-':
		default:
			return fmt.Errorf("forge remote_name %q contains illegal character %q", name, string(r))
		}
	}
	return nil
}

// Forge decodes the [forge] block from a loaded Config.
//
// It returns (nil, nil) when no [forge] key is present — the overwhelmingly
// common case — so callers gate on presence rather than on a zero struct.
func (c *Config) Forge() (*ForgeConfig, error) {
	if c == nil {
		return nil, nil
	}
	if _, ok := c.Extensions[ForgeExtensionKey]; !ok {
		return nil, nil
	}
	var cfg ForgeConfig
	if err := c.UnmarshalExtension(ForgeExtensionKey, &cfg); err != nil {
		return nil, fmt.Errorf("failed to decode [forge] config: %w", err)
	}
	return &cfg, nil
}
