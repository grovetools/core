package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/oklog/ulid/v2"
	"github.com/pelletier/go-toml/v2"

	"github.com/grovetools/core/pkg/coderoot"
	"github.com/grovetools/core/pkg/paths"
	"github.com/grovetools/core/pkg/subject"
)

// MachineConfig is the typed schema for ~/.config/grove/machine.toml.
// Code topology remains in roots.toml/notebooks.toml. This file records
// machine-local notes-plane routing facts; none may be inferred.
type MachineConfig struct {
	Machine   MachineSettings   `toml:"machine"`
	Sync      MachineSync       `toml:"sync,omitempty"`
	Primaries map[string]string `toml:"primaries,omitempty"`
	Subjects  map[string]string `toml:"subjects,omitempty"`
}

// MachineSync records relationships with sync infrastructure.
type MachineSync struct {
	Registry *SyncRegistry `toml:"registry,omitempty"`
}

// SyncRegistry is the machine-local registry notespace binding.
type SyncRegistry struct {
	Notebook    string `toml:"notebook"`
	NotespaceID string `toml:"notespace_id"`
}

// MachineSettings is the [machine] table.
type MachineSettings struct {
	Name string `toml:"name,omitempty" jsonschema:"description=Display name for this machine (defaults to the hostname)"`
}

func MachineConfigPath() string {
	configDir := paths.ConfigDir()
	if configDir == "" {
		return ""
	}
	return filepath.Join(configDir, "machine.toml")
}

const (
	MachineConfigFileName = "machine.toml"
	LegacyMachinesDirName = "machines"
)

func isExcludedGlobalFragment(baseName string) bool {
	switch baseName {
	case "grove.toml", "grove.yml", "grove.override.toml", "sync.toml", MachineConfigFileName,
		coderoot.RootsFileName, coderoot.NotebooksFileName:
		return true
	}
	return false
}

// LegacyMachinesDir returns the dead ~/.config/grove/machines/ directory when
// it exists. Its contents are migration input only and are never loaded.
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

func LoadMachineConfig() (*MachineConfig, error) {
	path := MachineConfigPath()
	if path == "" {
		return nil, nil
	}
	return LoadMachineConfigFrom(path)
}

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

// ParseMachineConfigContent rejects legacy topology explicitly. A typed TOML
// decode would otherwise ignore the removed fields and silently lose intent.
func ParseMachineConfigContent(path, content string) (*MachineConfig, error) {
	expanded := expandEnvVars(content)
	var raw map[string]interface{}
	if err := toml.Unmarshal([]byte(expanded), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse machine config %s: %w", path, err)
	}
	if machine, ok := raw["machine"].(map[string]interface{}); ok {
		for _, table := range []string{"ecosystems", "roots"} {
			if _, exists := machine[table]; exists {
				if _, statErr := os.Stat(coderoot.RootsPath()); statErr == nil {
					return nil, fmt.Errorf("forbidden mixed state: %s contains legacy [machine.%s] while %s exists; run 'grove migrate'", path, table, coderoot.RootsFileName)
				}
				return nil, fmt.Errorf("legacy config %s contains [machine.%s] and %s is absent; run 'grove migrate'", path, table, coderoot.RootsFileName)
			}
		}
	}
	var cfg MachineConfig
	if err := toml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse machine config %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid machine config %s: %w", path, err)
	}
	return &cfg, nil
}

// validateCanonicalMachineConfig makes the canonical machine.toml part of a
// normal config load without projecting its display-only fields into Config.
// A nil trace is used by uncached loaders; hierarchical loads pass their trace
// so both presence and absence participate in cache freshness.
func validateCanonicalMachineConfig(trace *loadFromTrace) error {
	path := MachineConfigPath()
	if path == "" {
		return nil
	}
	var (
		data []byte
		err  error
	)
	if trace != nil {
		data, err = trace.readFile(path)
	} else {
		data, err = os.ReadFile(path)
	}
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to read machine config %s: %w", path, err)
	}
	_, err = ParseMachineConfigContent(path, string(data))
	return err
}

func (m *MachineConfig) Validate() error {
	if strings.TrimSpace(m.Machine.Name) != m.Machine.Name {
		return fmt.Errorf("machine name %q has leading or trailing whitespace", m.Machine.Name)
	}
	if strings.ContainsAny(m.Machine.Name, "\n\r\t") {
		return fmt.Errorf("machine name %q contains control characters", m.Machine.Name)
	}
	if m.Sync.Registry != nil {
		if err := validateMachineText("sync.registry notebook", m.Sync.Registry.Notebook); err != nil {
			return err
		}
		if _, err := ulid.ParseStrict(m.Sync.Registry.NotespaceID); err != nil {
			return fmt.Errorf("sync.registry notespace_id %q is not a ULID: %w", m.Sync.Registry.NotespaceID, err)
		}
	}
	for value, id := range m.Primaries {
		if err := subject.Validate(value); err != nil {
			return fmt.Errorf("invalid [primaries] subject %q: %w", value, err)
		}
		if _, err := ulid.ParseStrict(id); err != nil {
			return fmt.Errorf("[primaries] %q id %q is not a ULID: %w", value, id, err)
		}
	}
	for path, value := range m.Subjects {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return fmt.Errorf("[subjects] path %q is not canonical absolute path", path)
		}
		if !strings.HasPrefix(value, subject.LocalPrefix) {
			return fmt.Errorf("[subjects] %q must record a local: subject, got %q", path, value)
		}
		if err := subject.Validate(value); err != nil {
			return fmt.Errorf("invalid [subjects] value for %q: %w", path, err)
		}
	}
	return nil
}

func validateMachineText(field, value string) error {
	if value == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\n\r\t") {
		return fmt.Errorf("%s is empty or contains surrounding/control whitespace", field)
	}
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func ResolveMachineName() string {
	if cfg, err := LoadMachineConfig(); err == nil && cfg != nil && cfg.Machine.Name != "" {
		return cfg.Machine.Name
	}
	return DefaultMachineName()
}

func DefaultMachineName() string {
	if host, err := os.Hostname(); err == nil && host != "" {
		return host
	}
	return "unknown"
}
