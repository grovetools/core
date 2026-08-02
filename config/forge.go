package config

import (
	"fmt"
	"net/url"
	"strings"
	"time"
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
// The self-hosted-instance fields (URL, RemoteName, TokenCommand) remain
// parse-only: no code contacts the URL, enrolls the remote, or executes
// TokenCommand. Resolving TokenCommand is the daemon's job — core/pkg/forge
// accepts an already-resolved token and never shells out for one — so this
// struct carries the string and nothing more.
//
// The one field that IS acted on is Poll, the daemon forge poller's opt-in.
// It sits here because it configures forge integration, not because it
// configures the instance above it: the poller reads whatever forge a repo's
// origin resolves to (GitHub today) and works fine with no [forge] URL at all.
type ForgeConfig struct {
	// URL is the base URL of the self-hosted forge instance.
	URL string `yaml:"url,omitempty" toml:"url,omitempty" jsonschema:"description=Base URL of the self-hosted forge instance"`
	// RemoteName is the git remote the forge is enrolled under. Empty means
	// DefaultForgeRemoteName; see EffectiveRemoteName.
	RemoteName string `yaml:"remote_name,omitempty" toml:"remote_name,omitempty" jsonschema:"description=Git remote name for the forge,default=forge"`
	// TokenCommand is a shell command whose trimmed stdout is the forge API
	// token. It is stored, never executed here.
	TokenCommand string `yaml:"token_command,omitempty" toml:"token_command,omitempty" jsonschema:"description=Shell command that prints the forge API token" jsonschema_extras:"x-sensitive=true,x-hint=Prefer token_command over embedding a token in config"`
	// Poll is the [forge.poll] sub-block configuring the daemon's read-only
	// forge poller. Absent means the poller stays off.
	Poll *ForgePollConfig `yaml:"poll,omitempty" toml:"poll,omitempty" jsonschema:"description=Daemon read-only forge poller (PR + checks state)"`
}

// Poller cadence bounds. The floor exists so a typo cannot turn the daemon into
// a request generator against someone else's API; the defaults are what a
// laptop watching its own ecosystem wants.
const (
	// DefaultForgePollInterval is the gap between poll sweeps when
	// [forge.poll] does not say otherwise.
	DefaultForgePollInterval = 5 * time.Minute
	// MinForgePollInterval is the floor a configured interval is clamped to.
	MinForgePollInterval = 1 * time.Minute
	// DefaultForgeStaleAfter is how long a successful fetch stays "fresh"
	// before the poller degrades it to stale of its own accord.
	DefaultForgeStaleAfter = 15 * time.Minute
	// MinForgeStaleAfter is the floor for StaleAfter. A window shorter than the
	// poll interval would mark every entry stale between sweeps.
	MinForgeStaleAfter = 2 * time.Minute
)

// ForgePollConfig is the typed [forge.poll] block:
//
//	[forge.poll]
//	enabled = true
//	interval = "5m"
//	stale_after = "15m"
//
// It is OFF by default and stays off unless Enabled is explicitly true. The
// poller additionally refuses to run when its provider reports the transport
// unavailable, so enabling this on a machine with no `gh` is a no-op with a
// log line rather than an error.
type ForgePollConfig struct {
	// Enabled is the explicit opt-in. False (and absent) means no poller, no
	// goroutine, no network reads.
	Enabled bool `yaml:"enabled,omitempty" toml:"enabled,omitempty" jsonschema:"description=Enable the daemon forge poller,default=false"`
	// Interval is a Go duration string ("5m") for the gap between sweeps.
	// Empty means DefaultForgePollInterval; anything below
	// MinForgePollInterval is clamped up. See EffectiveInterval.
	Interval string `yaml:"interval,omitempty" toml:"interval,omitempty" jsonschema:"description=Gap between poll sweeps (Go duration),default=5m"`
	// StaleAfter is a Go duration string for how long fetched data stays
	// fresh. See EffectiveStaleAfter.
	StaleAfter string `yaml:"stale_after,omitempty" toml:"stale_after,omitempty" jsonschema:"description=How long fetched forge data stays fresh (Go duration),default=15m"`
}

// PollEnabled reports whether the daemon forge poller is opted in. Nil-safe at
// both levels, because "no [forge] block at all" is the normal state.
func (f *ForgeConfig) PollEnabled() bool {
	return f != nil && f.Poll != nil && f.Poll.Enabled
}

// EffectiveInterval resolves Interval against the default and the floor. An
// unparseable duration falls back to the default rather than failing: the
// poller is an optional read-only convenience and must never be a reason a
// daemon does not boot.
func (p *ForgePollConfig) EffectiveInterval() time.Duration {
	return effectiveForgeDuration(p.rawInterval(), DefaultForgePollInterval, MinForgePollInterval)
}

// EffectiveStaleAfter resolves StaleAfter against the default and the floor,
// then guarantees it is at least one interval long — a freshness window
// narrower than the sweep gap would report every entry stale between sweeps.
func (p *ForgePollConfig) EffectiveStaleAfter() time.Duration {
	stale := effectiveForgeDuration(p.rawStaleAfter(), DefaultForgeStaleAfter, MinForgeStaleAfter)
	if interval := p.EffectiveInterval(); stale < interval {
		return interval
	}
	return stale
}

func (p *ForgePollConfig) rawInterval() string {
	if p == nil {
		return ""
	}
	return p.Interval
}

func (p *ForgePollConfig) rawStaleAfter() string {
	if p == nil {
		return ""
	}
	return p.StaleAfter
}

func effectiveForgeDuration(raw string, def, floor time.Duration) time.Duration {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return def
	}
	d, err := time.ParseDuration(trimmed)
	if err != nil || d <= 0 {
		return def
	}
	if d < floor {
		return floor
	}
	return d
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
	if f.Poll != nil {
		// Report a typo'd duration to whoever asked for validation. The runtime
		// path never consults this: EffectiveInterval/EffectiveStaleAfter fall
		// back to the defaults, because an unparseable poll cadence must not be
		// a reason the daemon fails to boot.
		for _, d := range []struct{ field, raw string }{
			{"interval", f.Poll.Interval},
			{"stale_after", f.Poll.StaleAfter},
		} {
			raw := strings.TrimSpace(d.raw)
			if raw == "" {
				continue
			}
			parsed, err := time.ParseDuration(raw)
			if err != nil {
				return fmt.Errorf("forge poll %s %q is not a valid duration: %w", d.field, d.raw, err)
			}
			if parsed <= 0 {
				return fmt.Errorf("forge poll %s %q must be positive", d.field, d.raw)
			}
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
