package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/grovetools/core/pkg/paths"
)

// MachineConfig is the typed schema for ~/.config/grove/machine.toml — this
// machine's *intent*, as opposed to its identity (a ULID in
// $XDG_STATE_HOME/grove/machine.json; see core/pkg/machine).
//
// It is deliberately loaded by a standalone typed loader rather than picked
// up by the global config fragment glob, mirroring sync.toml: a stray table
// in machine.toml must not leak into the config cascade. LoadFromWithLogger
// and LoadLayered both exclude machine.toml from the fragment glob for that
// reason.
//
// machine.toml IS dotfiles-portable on purpose — a restored config plus a
// freshly minted state ID is "a new machine with the same intent", the
// supported fast path.
//
// The full shape is name + subscriptions + bare roots. Both subscription
// kinds compile into the legacy cfg.Groves map at load time
// (compileMachineGroves), so the ~15 existing Groves consumers change nothing
// while machine.toml becomes the place intent is authored.
type MachineConfig struct {
	Machine MachineSettings `toml:"machine"`
}

// MachineSettings is the [machine] table.
type MachineSettings struct {
	// Name is this machine's display name. It is display-level only and can
	// collide across machines restored from one dotfiles repo, which is why
	// every surface renders "name (short id)" — never the name alone.
	// Empty means "use the hostname" (see ResolveMachineName).
	Name string `toml:"name,omitempty" jsonschema:"description=Display name for this machine (defaults to the hostname)"`

	// Ecosystems are this machine's ecosystem subscriptions:
	// [machine.ecosystems.<name>]. A subscription is a standing statement of
	// intent ("this machine wants grovetools at ~/code/grovetools"), which is
	// what makes "declared but missing" computable — the diff between these
	// entries and what is actually on disk is exactly the materialization
	// verb's input.
	Ecosystems map[string]MachineEcosystem `toml:"ecosystems,omitempty" jsonschema:"description=Ecosystem subscriptions for this machine"`

	// Roots are bare scan roots: [machine.roots.<name>]. A root is a directory
	// of repos that is NOT an ecosystem and never gains an identity card —
	// ~/code/chickens stays a first-class citizen. Roots are never
	// declared-but-missing candidates: nothing can materialize them.
	Roots map[string]MachineRoot `toml:"roots,omitempty" jsonschema:"description=Bare scan roots (directories of repos that are not ecosystems)"`
}

// MachineEcosystem is one [machine.ecosystems.<name>] entry.
type MachineEcosystem struct {
	// Path is where this machine keeps the ecosystem. It is machine-local by
	// nature (the same ecosystem lives at different paths on different hosts),
	// which is why it is config and not part of the repo-side card.
	Path string `toml:"path" jsonschema:"description=Path to the ecosystem on this machine"`
	// Notebook OVERRIDES the card's default notebook binding for this machine.
	// Empty means "use whatever the ecosystem's own card says" — the normal
	// case, and the reason the field is optional.
	Notebook string `toml:"notebook,omitempty" jsonschema:"description=Override the ecosystem card's default notebook binding on this machine"`
	// Enabled disables a subscription without deleting it. Nil = enabled,
	// matching GroveSourceConfig.Enabled, which this compiles into.
	Enabled *bool `toml:"enabled,omitempty" jsonschema:"description=Whether this subscription is active (default: true)"`
	// Description is a human label carried through to the compiled grove entry.
	Description string `toml:"description,omitempty" jsonschema:"description=Human-readable description"`
	// Repos narrows this subscription to the named member repositories. Empty
	// means every member. It is subscriber-local intent, not ecosystem-card
	// metadata, so two machines may materialize different subsets of one card.
	Repos []string `toml:"repos,omitempty" jsonschema:"description=Member repositories to materialize (empty means all)"`
	// Exclude omits named member repositories when Repos is empty. Repos and
	// Exclude are mutually exclusive so intent has one unambiguous shape.
	Exclude []string `toml:"exclude,omitempty" jsonschema:"description=Member repositories to omit"`
}

// IncludesRepo reports whether a card member belongs to this subscription.
// The member name is the flat-card remote name or superrepo submodule path.
func (e MachineEcosystem) IncludesRepo(name string) bool {
	if len(e.Repos) > 0 {
		return containsString(e.Repos, name)
	}
	return !containsString(e.Exclude, name)
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// MachineRoot is one [machine.roots.<name>] entry.
type MachineRoot struct {
	// Path is the directory to scan.
	Path string `toml:"path" jsonschema:"description=Path to the bare scan root"`
	// Notebook binds the root's projects to a notebook. Unlike an ecosystem
	// (which can fall back to its card), a bare root has no other source for
	// this, so declaring it here is the only way to bind one.
	Notebook string `toml:"notebook,omitempty" jsonschema:"description=Notebook used for projects under this root"`
	// Enabled disables a root without deleting it. Nil = enabled.
	Enabled *bool `toml:"enabled,omitempty" jsonschema:"description=Whether this root is scanned (default: true)"`
	// Description is a human label carried through to the compiled grove entry.
	Description string `toml:"description,omitempty" jsonschema:"description=Human-readable description"`
}

// MachineConfigPath returns the path of the canonical machine config, or ""
// when no config directory can be resolved.
func MachineConfigPath() string {
	configDir := paths.ConfigDir()
	if configDir == "" {
		return ""
	}
	return filepath.Join(configDir, "machine.toml")
}

// MachineConfigFileName is the basename excluded from the global config
// fragment glob (see LoadFromWithLogger / LoadLayered).
const MachineConfigFileName = "machine.toml"

// LegacyMachinesDirName is the dead per-machine config directory. It is never
// loaded; its presence is only reported, by the surfaces an operator asks
// (`grove machine`, `grove doctor`), pointing at `grove machine migrate`.
const LegacyMachinesDirName = "machines"

// isExcludedGlobalFragment reports whether a basename in ~/.config/grove is
// owned by a standalone typed loader and must therefore be skipped by the
// `*.toml` fragment glob in LoadFromWithLogger and LoadLayered. Without the
// machine.toml entry, a `[machine]` table would land in Config.Extensions and
// leak into the whole cascade — the pathology the typed loader exists to
// contain.
func isExcludedGlobalFragment(baseName string) bool {
	switch baseName {
	case "grove.toml", "grove.yml", "grove.override.toml", "sync.toml", MachineConfigFileName:
		return true
	}
	return false
}

// LegacyMachinesDir returns the dead ~/.config/grove/machines/ directory when
// it exists, or "" when it does not. Its contents are never loaded — machine
// intent lives in machine.toml now — so the only correct response is to tell
// the operator how to migrate.
//
// Deliberately a QUERY, not a warning raised on the config-load path. The dead
// directory is a standing CONDITION, not an event: it outlives every process,
// while grove spawns processes constantly (hooks, status polls, daemons, TUI
// refreshes). A load-path warning is therefore O(processes), not O(conditions)
// — per-process dedupe still writes one line per invocation, which is how this
// nag came back as hundreds of identical WARNING lines in the workspace logs
// after it was moved off the console. Standing conditions belong to the
// surfaces an operator interrogates: `grove machine` prints it, `grove doctor`
// checks it (doctorchecks.legacyMachinesDirCheck), and both report the current
// state of the machine rather than appending to an event stream.
func LegacyMachinesDir() string {
	configDir := paths.ConfigDir()
	if configDir == "" {
		return ""
	}
	dir := filepath.Join(configDir, LegacyMachinesDirName)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return ""
	}
	return dir
}

// LoadMachineConfig loads ~/.config/grove/machine.toml. A missing file is not
// an error: it returns (nil, nil), which means "no declared intent" — the
// name then falls back to the hostname.
func LoadMachineConfig() (*MachineConfig, error) {
	path := MachineConfigPath()
	if path == "" {
		return nil, nil
	}
	return LoadMachineConfigFrom(path)
}

// LoadMachineConfigFrom loads a machine config from an explicit path. A
// missing file returns (nil, nil).
func LoadMachineConfigFrom(path string) (*MachineConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read machine config %s: %w", path, err)
	}

	return ParseMachineConfigContent(path, string(data))
}

// ParseMachineConfigContent decodes machine.toml content through exactly the
// path LoadMachineConfigFrom uses — env expansion, TOML decode, Validate — so
// what a writer verifies before persisting is what the loader will accept.
// `path` is used only in error messages.
func ParseMachineConfigContent(path, content string) (*MachineConfig, error) {
	expanded := expandEnvVars(content)
	var cfg MachineConfig
	if err := toml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse machine config %s: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid machine config %s: %w", path, err)
	}

	return &cfg, nil
}

// Validate checks the machine config for self-consistency.
func (m *MachineConfig) Validate() error {
	if strings.TrimSpace(m.Machine.Name) != m.Machine.Name {
		return fmt.Errorf("machine name %q has leading or trailing whitespace", m.Machine.Name)
	}
	if strings.ContainsAny(m.Machine.Name, "\n\r\t") {
		return fmt.Errorf("machine name %q contains control characters", m.Machine.Name)
	}
	for _, name := range sortedKeys(m.Machine.Ecosystems) {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("[machine.ecosystems] has an entry with an empty name")
		}
		eco := m.Machine.Ecosystems[name]
		if strings.TrimSpace(eco.Path) == "" {
			return fmt.Errorf("[machine.ecosystems.%s] has no path", name)
		}
		if len(eco.Repos) > 0 && len(eco.Exclude) > 0 {
			return fmt.Errorf("[machine.ecosystems.%s] cannot set both repos and exclude", name)
		}
		for field, values := range map[string][]string{"repos": eco.Repos, "exclude": eco.Exclude} {
			seen := make(map[string]bool, len(values))
			for _, repo := range values {
				if strings.TrimSpace(repo) == "" || strings.TrimSpace(repo) != repo {
					return fmt.Errorf("[machine.ecosystems.%s] %s contains an empty or whitespace-padded repository name", name, field)
				}
				if seen[repo] {
					return fmt.Errorf("[machine.ecosystems.%s] %s contains duplicate repository %q", name, field, repo)
				}
				seen[repo] = true
			}
		}
	}
	for _, name := range sortedKeys(m.Machine.Roots) {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("[machine.roots] has an entry with an empty name")
		}
		if strings.TrimSpace(m.Machine.Roots[name].Path) == "" {
			return fmt.Errorf("[machine.roots.%s] has no path", name)
		}
		// Both kinds compile into the same cfg.Groves keyspace, so a name
		// used twice would silently resolve to one of them. Reject it here,
		// where the file is being read, rather than at compile time where the
		// loser is invisible.
		if _, dup := m.Machine.Ecosystems[name]; dup {
			return fmt.Errorf("%q is declared as both [machine.ecosystems.%s] and [machine.roots.%s]; a name may be one or the other", name, name, name)
		}
	}
	return nil
}

// sortedKeys returns a map's keys in deterministic order. Every machine-config
// surface (validation errors, compilation, rendering) iterates through it so
// repeated runs produce identical output.
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// compileMachineGroves projects machine.toml's subscriptions and bare roots
// into the legacy cfg.Groves map, which is what every discovery consumer
// already reads (DiscoveryService.DiscoverAll, notebookWorkspaceContext,
// SyncHandler.syntheticNodeFor, ...). Compiling instead of migrating those
// consumers is the whole point: intent moves to machine.toml without a
// fifteen-callsite refactor.
//
// It is an EXPLICIT post-merge step rather than an unmarshal hook. The
// search_paths→groves shim in Config.UnmarshalYAML is YAML-only (there is no
// Config.UnmarshalTOML; the TOML branch of unmarshalConfig is a plain
// toml.Unmarshal plus two post-processors), so an unmarshal-time compilation
// would silently not happen for TOML — the dominant dialect here.
//
// Merge rule: fill ABSENT keys only. An explicit [groves.<name>] anywhere in
// the cascade wins, which is what makes the migration window safe — a user can
// declare a subscription in machine.toml and keep the old entry until they are
// ready to delete it, with no change in behavior.
//
// It returns the config to use rather than mutating in place. LoadLayered's
// merged views can still BE a layer (`finalConfig = layeredConfig.Global` when
// no fragments exist), and compiled entries appearing inside a *layer* would
// misattribute them to a file that never declared them. When there is nothing
// to add — the overwhelmingly common case of no machine.toml — the original
// pointer comes straight back and nothing is copied.
func compileMachineGroves(cfg *Config, machineCfg *MachineConfig) *Config {
	if cfg == nil || machineCfg == nil {
		return cfg
	}
	compiled := machineCfg.CompiledGroves()
	if len(compiled) == 0 {
		return cfg
	}
	missing := false
	for name := range compiled {
		if _, exists := cfg.Groves[name]; !exists {
			missing = true
			break
		}
	}
	if !missing {
		return cfg
	}

	out := *cfg
	merged := make(map[string]GroveSourceConfig, len(cfg.Groves)+len(compiled))
	for name, entry := range compiled {
		merged[name] = entry
	}
	for name, entry := range cfg.Groves {
		merged[name] = entry // explicit [groves.*] wins during migration
	}
	out.Groves = merged
	return &out
}

// CompiledGroves renders this machine config's subscriptions and roots as
// grove source entries, without consulting any Config. Exported because the
// reconciliation surfaces (`grove machine status`, doctor) need to talk about
// the compiled view directly.
func (m *MachineConfig) CompiledGroves() map[string]GroveSourceConfig {
	if m == nil {
		return nil
	}
	out := make(map[string]GroveSourceConfig, len(m.Machine.Ecosystems)+len(m.Machine.Roots))
	for name, eco := range m.Machine.Ecosystems {
		out[name] = GroveSourceConfig{
			Path:        eco.Path,
			Enabled:     eco.Enabled,
			Notebook:    eco.Notebook,
			Description: eco.Description,
		}
	}
	for name, root := range m.Machine.Roots {
		// Validate rejects a name used by both, so a root can only land on a
		// free key here.
		out[name] = GroveSourceConfig{
			Path:        root.Path,
			Enabled:     root.Enabled,
			Notebook:    root.Notebook,
			Description: root.Description,
		}
	}
	return out
}

// loadMachineConfigForCompile reads the canonical machine.toml for the
// compilation step. Loading failures are swallowed on purpose: a malformed
// machine.toml must not take the whole config cascade down with it (the same
// stance every other optional layer takes). The operator sees the real error
// from `grove machine status` and the doctor's config-layer check.
func loadMachineConfigForCompile() *MachineConfig {
	cfg, err := LoadMachineConfig()
	if err != nil {
		return nil
	}
	return cfg
}

// ResolveMachineName returns the configured [machine] name, falling back to
// the hostname and finally to "unknown". It never fails: an unreadable
// machine.toml degrades to the hostname rather than blocking a surface that
// only wanted a label.
func ResolveMachineName() string {
	if cfg, err := LoadMachineConfig(); err == nil && cfg != nil && cfg.Machine.Name != "" {
		return cfg.Machine.Name
	}
	return DefaultMachineName()
}

// DefaultMachineName is the name used when machine.toml declares none.
func DefaultMachineName() string {
	if host, err := os.Hostname(); err == nil && host != "" {
		return host
	}
	return "unknown"
}
