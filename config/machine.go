package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/grovetools/core/pkg/coderoot"
	"github.com/grovetools/core/pkg/paths"
)

// MachineConfig is the typed schema for ~/.config/grove/machine.toml.
// Topology is recorded in roots.toml and notebooks.toml; machine.toml retains
// only display identity metadata.
type MachineConfig struct {
	Machine MachineSettings `toml:"machine"`
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
				return nil, fmt.Errorf("legacy config %s contains [machine.%s]; run 'grove migrate'", path, table)
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

func (m *MachineConfig) Validate() error {
	if strings.TrimSpace(m.Machine.Name) != m.Machine.Name {
		return fmt.Errorf("machine name %q has leading or trailing whitespace", m.Machine.Name)
	}
	if strings.ContainsAny(m.Machine.Name, "\n\r\t") {
		return fmt.Errorf("machine name %q contains control characters", m.Machine.Name)
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
