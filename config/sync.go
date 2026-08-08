package config

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/invopop/jsonschema"
	"github.com/mitchellh/mapstructure"
	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"

	"github.com/grovetools/core/pkg/paths"
)

// Sync workspace subscription modes. A mode controls which subset of a
// workspace's documents a client subscribes to.
const (
	SyncModeFull       = "full"        // All documents in the workspace.
	SyncModePlansOnly  = "plans-only"  // Only the plans/ prefix.
	SyncModeSearchOnly = "search-only" // No local replica; server-side search only.
)

// SyncTokenEnvVar is the environment variable consulted first when resolving
// the sync bearer token (env > token_command > static token).
const SyncTokenEnvVar = "GROVE_SYNC_TOKEN"

// SyncProviderConfig is a single legacy per-notebook sync provider entry
// (e.g. the GitHub issues/PRs importer in nb). The historical config shape
// for `notebooks.definitions.<name>.sync` was a *list* of these entries;
// that shape is still accepted and decoded into SyncConfig.Providers.
type SyncProviderConfig struct {
	Provider   string `yaml:"provider" toml:"provider" jsonschema:"description=Provider name (e.g. github)"`
	IssuesType string `yaml:"issues_type,omitempty" toml:"issues_type,omitempty" jsonschema:"description=Note type used for imported issues"`
	PRsType    string `yaml:"prs_type,omitempty" toml:"prs_type,omitempty" jsonschema:"description=Note type used for imported pull requests"`
}

// Sync workspace roles (SyncWorkspace.Role) — what RELATIONSHIP this machine
// has with the peer holding the workspace. The role, not the pull flag, is
// what decides whether pulling is legitimate.
const (
	// SyncRoleSatellite: the peer is a disposable VM this machine provisions.
	// PUSH-ONLY, always: a pulling laptop would let the satellite overwrite
	// local notebooks, which is the invariant `grove satellite up` exists to
	// protect.
	SyncRoleSatellite = "satellite"
	// SyncRolePeer: another machine of the same operator, mirroring the same
	// notebook. Pulling is the whole point — a fresh machine materializing its
	// own workspace has to write locally.
	SyncRolePeer = "peer"
	// SyncRoleRegistry: the reserved machine-presence workspace. Pulling is
	// required to see other machines at all.
	SyncRoleRegistry = "registry"
)

// SyncWorkspace is a per-workspace sync subscription.
type SyncWorkspace struct {
	Name string `yaml:"name" toml:"name" jsonschema:"description=Workspace name to sync"`
	// Role names the relationship with the peer holding this workspace:
	// satellite, peer, or registry.
	//
	// EMPTY IS LOAD-BEARING. An entry with no role is a LEGACY entry, and
	// legacy means push-only: every guard that used to reject `pull = true`
	// outright still rejects it for role-less entries, byte for byte. Only an
	// explicitly-roled entry can opt into pulling, and only when its role
	// permits it. That is what lets peers and the registry pull without
	// weakening the satellite guarantee by a single line.
	Role string `yaml:"role,omitempty" toml:"role,omitempty" jsonschema:"description=Relationship with the peer holding this workspace,enum=satellite,enum=peer,enum=registry"`
	// Mode selects the subscription filter: full, plans-only, or search-only.
	Mode string `yaml:"mode,omitempty" toml:"mode,omitempty" jsonschema:"description=Subscription mode,enum=full,enum=plans-only,enum=search-only,default=full"`
	// Pull opts this machine into writing pulled changes to the local
	// notebook tree. Without it, sync is push-only (notebook-read-only).
	Pull bool `yaml:"pull,omitempty" toml:"pull,omitempty" jsonschema:"description=Allow pulled changes to be written to the local notebook tree,default=false"`
	// Excludes are additional path-prefix/glob exclusions applied on top of
	// the protocol's default exclusion manifest.
	Excludes []string `yaml:"excludes,omitempty" toml:"excludes,omitempty" jsonschema:"description=Additional exclusion globs for this workspace"`
	// MaxFileSize caps the byte size of documents synced from this workspace;
	// larger files are skipped by the client (0 = no limit). This is the
	// per-workspace overlay on the big-file policy, distinct from the server's
	// advertised blob ceiling.
	MaxFileSize int64 `yaml:"max_file_size,omitempty" toml:"max_file_size,omitempty" jsonschema:"description=Files larger than this many bytes are not synced (0 = no limit)"`
}

// SyncConfig is the typed sync configuration. It is the schema for both
// ~/.config/grove/sync.toml (the canonical client config; see LoadSyncConfig)
// and the per-notebook `sync` key in grove config files.
//
// Sync is dark by default: when no sync configuration is present, nothing in
// the sync stack activates.
//
// For backward compatibility, the per-notebook `sync` key also accepts the
// legacy list-of-providers shape, decoded into Providers.
type SyncConfig struct {
	// Server is the base URL of the grove-syncd server (e.g.
	// "https://sync.example.com").
	Server string `yaml:"server,omitempty" toml:"server,omitempty" jsonschema:"description=Base URL of the sync server"`
	// Token is a static bearer token. Resolution order is
	// GROVE_SYNC_TOKEN env var > TokenCommand > Token (see ResolveToken).
	Token string `yaml:"token,omitempty" toml:"token,omitempty" jsonschema:"description=Static sync bearer token" jsonschema_extras:"x-sensitive=true,x-hint=Consider using token_command to fetch from a secrets manager"`
	// TokenCommand is a shell command whose trimmed stdout is the token.
	TokenCommand string `yaml:"token_command,omitempty" toml:"token_command,omitempty" jsonschema:"description=Shell command to retrieve the sync token (e.g. a secrets manager)"`
	// CACert is the path to a PEM file whose certificates are the ONLY roots
	// trusted for the sync server's TLS. It exists for self-signed
	// deployments (`grove forge`'s `tls_mode = "self-signed"` writes exactly
	// one such cert): pin the server's own certificate here and stock Go
	// verification — expiry, and the IP/DNS SAN match — still applies.
	//
	// This is a REPLACEMENT root pool, not an addition to the system pool,
	// and it is deliberately not an "insecure/skip-verify" switch: there is
	// no configuration that disables certificate verification.
	CACert string `yaml:"ca_cert,omitempty" toml:"ca_cert,omitempty" jsonschema:"description=Path to a PEM file of certificates to trust as the sole roots for the sync server (for self-signed deployments)"`
	// Workspaces lists the per-workspace sync subscriptions.
	Workspaces []SyncWorkspace `yaml:"workspaces,omitempty" toml:"workspaces,omitempty" jsonschema:"description=Per-workspace sync subscriptions"`
	// Providers holds legacy per-notebook sync provider entries (the old
	// list shape of the `sync` key). Consumed by nb's provider-based sync.
	Providers []SyncProviderConfig `yaml:"providers,omitempty" toml:"providers,omitempty" jsonschema:"description=Legacy per-notebook sync provider entries"`
}

// syncConfigAlias avoids UnmarshalYAML recursion when decoding the mapping shape.
type syncConfigAlias SyncConfig

// UnmarshalYAML accepts both the typed mapping shape and the legacy
// list-of-providers shape for the `sync` key.
func (s *SyncConfig) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.SequenceNode:
		// Legacy shape: sync is a list of provider entries.
		var providers []SyncProviderConfig
		if err := node.Decode(&providers); err != nil {
			return fmt.Errorf("failed to decode legacy sync provider list: %w", err)
		}
		*s = SyncConfig{Providers: providers}
		return nil
	case yaml.MappingNode:
		var tmp syncConfigAlias
		if err := node.Decode(&tmp); err != nil {
			return err
		}
		*s = SyncConfig(tmp)
		return nil
	default:
		// Null or scalar: leave zero value (no sync configured).
		return nil
	}
}

// JSONSchema keeps the generated schema permissive for the `sync` key so the
// legacy list shape continues to validate alongside the typed object shape.
func (SyncConfig) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Description: "Sync configuration: typed object (server/token/workspaces) or legacy provider list",
	}
}

// CACertPath returns CACert with ~ and environment variables expanded, or
// "" when no CA file is configured.
func (s *SyncConfig) CACertPath() string {
	if s.CACert == "" {
		return ""
	}
	return expandPath(s.CACert)
}

// TLSClientConfig builds the *tls.Config the sync HTTP client should use.
// It returns (nil, nil) when no CA file is configured, which leaves the
// system trust store in play — the correct behavior for a publicly-trusted
// server.
//
// When ca_cert IS set, the returned config's RootCAs contains ONLY those
// certificates. Everything else about verification is stock: the hostname or
// IP still has to match a SAN, and expiry still applies.
func (s *SyncConfig) TLSClientConfig() (*tls.Config, error) {
	path := s.CACertPath()
	if path == "" {
		return nil, nil
	}
	pem, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read sync ca_cert %s: %w", path, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("sync ca_cert %s contains no PEM certificates", path)
	}
	return &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}, nil
}

// Validate checks the structural validity of a SyncConfig. It is not called
// on the config load path (sync is dark in Phase 0); consumers that activate
// sync should call it explicitly.
func (s *SyncConfig) Validate() error {
	if s.CACert != "" {
		if _, err := s.TLSClientConfig(); err != nil {
			return err
		}
	}
	for i, ws := range s.Workspaces {
		if ws.Name == "" {
			return fmt.Errorf("sync workspace entry %d missing 'name'", i)
		}
		switch ws.Mode {
		case "", SyncModeFull, SyncModePlansOnly, SyncModeSearchOnly:
		default:
			return fmt.Errorf("sync workspace %q has invalid mode %q (expected %s, %s, or %s)",
				ws.Name, ws.Mode, SyncModeFull, SyncModePlansOnly, SyncModeSearchOnly)
		}
		switch ws.Role {
		case "", SyncRoleSatellite, SyncRolePeer, SyncRoleRegistry:
		default:
			return fmt.Errorf("sync workspace %q has invalid role %q (expected %s, %s, or %s; empty means legacy push-only)",
				ws.Name, ws.Role, SyncRoleSatellite, SyncRolePeer, SyncRoleRegistry)
		}
		if ws.MaxFileSize < 0 {
			return fmt.Errorf("sync workspace %q has negative max_file_size %d", ws.Name, ws.MaxFileSize)
		}
	}
	for i, p := range s.Providers {
		if p.Provider == "" {
			return fmt.Errorf("sync provider entry %d missing 'provider'", i)
		}
	}
	return nil
}

// RolePushOnly reports whether a role forbids `pull = true`.
//
// The rule is deliberately narrow: only the SATELLITE relationship and LEGACY
// (role-less) entries are push-only. The invariant it enforces is "a
// disposable VM must never be able to write this machine's notebooks" — it was
// never "this machine may not pull", and reading it that way is what kept a
// workstation from mirroring its own notebook.
func RolePushOnly(role string) bool {
	return role == "" || role == SyncRoleSatellite
}

// ResolveToken resolves the sync bearer token using three-tier resolution,
// mirroring the API-key pattern used by the LLM providers:
//  1. GROVE_SYNC_TOKEN environment variable
//  2. token_command output (trimmed)
//  3. static token value
//
// Returns an empty string with no error when nothing is configured; callers
// gate activation on configuration presence, not on token availability.
func (s *SyncConfig) ResolveToken() (string, error) {
	if token := os.Getenv(SyncTokenEnvVar); token != "" {
		return token, nil
	}

	if s.TokenCommand != "" {
		cmd := exec.Command("sh", "-c", s.TokenCommand) //nolint:gosec // command comes from user's sync.toml config
		output, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("failed to execute sync token_command: %w", err)
		}
		token := strings.TrimSpace(string(output))
		if token == "" {
			return "", fmt.Errorf("sync token_command returned empty output")
		}
		return token, nil
	}

	return s.Token, nil
}

// SyncConfigPath returns the canonical location of the sync client config
// (~/.config/grove/sync.toml, honoring GROVE_HOME/XDG overrides). Returns an
// empty string when no config directory can be determined.
func SyncConfigPath() string {
	configDir := paths.ConfigDir()
	if configDir == "" {
		return ""
	}
	return filepath.Join(configDir, "sync.toml")
}

// LoadSyncConfig loads ~/.config/grove/sync.toml. A missing file is not an
// error: it returns (nil, nil), which is the "sync is disabled" state — the
// config-gate that keeps the entire sync stack dark.
func LoadSyncConfig() (*SyncConfig, error) {
	path := SyncConfigPath()
	if path == "" {
		return nil, nil
	}
	return LoadSyncConfigFrom(path)
}

// LoadSyncConfigFrom loads a sync config from an explicit path. A missing
// file returns (nil, nil).
func LoadSyncConfigFrom(path string) (*SyncConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read sync config %s: %w", path, err)
	}

	expanded := expandEnvVars(string(data))
	var cfg SyncConfig
	if err := toml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse sync config %s: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid sync config %s: %w", path, err)
	}

	return &cfg, nil
}

// decodeRawSyncValue converts a raw decoded TOML value for the `sync` key
// into a typed SyncConfig, accepting both the typed table shape and the
// legacy array-of-tables shape.
func decodeRawSyncValue(value interface{}) (*SyncConfig, error) {
	decode := func(input, target interface{}) error {
		decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
			Result:  target,
			TagName: "toml",
		})
		if err != nil {
			return err
		}
		return decoder.Decode(input)
	}

	switch v := value.(type) {
	case []interface{}:
		// Legacy shape: list of provider entries.
		var providers []SyncProviderConfig
		if err := decode(v, &providers); err != nil {
			return nil, fmt.Errorf("failed to decode legacy sync provider list: %w", err)
		}
		return &SyncConfig{Providers: providers}, nil
	case map[string]interface{}:
		var cfg SyncConfig
		if err := decode(v, &cfg); err != nil {
			return nil, fmt.Errorf("failed to decode sync config: %w", err)
		}
		return &cfg, nil
	default:
		return nil, fmt.Errorf("sync config must be a table or a provider list, got %T", value)
	}
}

// postProcessTOMLNotebookSync decodes `notebooks.definitions.<name>.sync`
// from raw TOML data into the typed Notebook.Sync field. The field is tagged
// `toml:"-"` because go-toml cannot decode the legacy array shape into a
// struct; this post-processing step (the same pattern as
// postProcessTOMLKeybindings) handles both shapes.
func postProcessTOMLNotebookSync(cfg *Config, data []byte) {
	if cfg.Notebooks == nil || cfg.Notebooks.Definitions == nil {
		return
	}

	var raw map[string]interface{}
	if err := toml.Unmarshal(data, &raw); err != nil {
		return
	}

	notebooksRaw, ok := raw["notebooks"].(map[string]interface{})
	if !ok {
		return
	}
	definitionsRaw, ok := notebooksRaw["definitions"].(map[string]interface{})
	if !ok {
		return
	}

	for name, defRaw := range definitionsRaw {
		defMap, ok := defRaw.(map[string]interface{})
		if !ok {
			continue
		}
		syncRaw, ok := defMap["sync"]
		if !ok {
			continue
		}
		notebook, ok := cfg.Notebooks.Definitions[name]
		if !ok || notebook == nil {
			continue
		}
		syncCfg, err := decodeRawSyncValue(syncRaw)
		if err != nil {
			// Tolerate malformed sync blocks: config loading must not start
			// failing on a key nothing consumes yet (dark-build rule).
			continue
		}
		notebook.Sync = syncCfg
	}
}
