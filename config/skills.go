package config

import (
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// DependencyConfig specifies how a particular skill should be resolved.
type DependencyConfig struct {
	// Source specifies where to resolve the skill from.
	// Valid values: "builtin", "user", "notebook", or empty for default precedence.
	Source string `toml:"source" yaml:"source"`

	// Name allows aliasing - use a different skill name for resolution.
	Name string `toml:"name" yaml:"name"`

	// Providers overrides the default providers for this skill.
	Providers []string `toml:"providers" yaml:"providers"`
}

// SkillsConfig represents the [skills] block in grove.toml.
type SkillsConfig struct {
	// Use lists the skills to be made available.
	Use []string `toml:"use" yaml:"use"`

	// Providers specifies the default agent providers to sync skills to.
	// Defaults to ["claude"] if not specified.
	Providers []string `toml:"providers" yaml:"providers"`

	// Dependencies provides explicit configuration for specific skills.
	Dependencies map[string]DependencyConfig `toml:"dependencies" yaml:"dependencies"`

	// Projects maps project names to user-scoped skill configurations.
	// Used in global config (~/.config/grove/grove.toml) to define
	// project-specific skills that live in dotfiles rather than repo config.
	Projects map[string]*SkillsConfig `toml:"projects" yaml:"projects"`

	// Ecosystems maps ecosystem names to user-scoped skill configurations.
	// Used in global config (~/.config/grove/grove.toml) to define
	// ecosystem-specific skills that live in dotfiles rather than repo config.
	Ecosystems map[string]*SkillsConfig `toml:"ecosystems" yaml:"ecosystems"`
}

// groveTomlSkills is used to extract the skills block from grove.toml
type groveTomlSkills struct {
	Skills *SkillsConfig `toml:"skills"`
}

// LoadSkillsFromGlobalConfig extracts [skills] from the core config's raw data.
// Uses UnmarshalExtension to safely decode nested projects/ecosystems maps.
func LoadSkillsFromGlobalConfig(cfg *Config) *SkillsConfig {
	if cfg == nil || cfg.Extensions == nil {
		return nil
	}

	var result SkillsConfig
	if err := cfg.UnmarshalExtension("skills", &result); err != nil {
		return nil
	}

	// Return nil if nothing was configured
	if len(result.Use) == 0 && len(result.Providers) == 0 &&
		len(result.Dependencies) == 0 && len(result.Projects) == 0 &&
		len(result.Ecosystems) == 0 {
		return nil
	}

	return &result
}

// LoadSkillsFromPath reads the [skills] block from grove.toml at the given path.
func LoadSkillsFromPath(dir string) (*SkillsConfig, error) {
	tomlPath := filepath.Join(dir, "grove.toml")
	data, err := os.ReadFile(tomlPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var parsed groveTomlSkills
	if err := toml.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}

	return parsed.Skills, nil
}

// MergeSkillsConfig merges ecosystem and project configs.
// Project config takes precedence for dependencies, but Use arrays are unioned.
func MergeSkillsConfig(ecosystem, project *SkillsConfig) *SkillsConfig {
	// If both are nil, return nil
	if ecosystem == nil && project == nil {
		return nil
	}

	// If only one exists, return a copy of it
	if ecosystem == nil {
		return CopySkillsConfig(project)
	}
	if project == nil {
		return CopySkillsConfig(ecosystem)
	}

	// Merge both configs
	merged := &SkillsConfig{
		// Union the Use arrays
		Use: unionSkillStrings(ecosystem.Use, project.Use),

		// Project providers override ecosystem providers if specified
		Providers: project.Providers,

		// Deep merge dependencies (project overrides ecosystem)
		Dependencies: make(map[string]DependencyConfig),
	}

	// If project didn't specify providers, use ecosystem's
	if len(merged.Providers) == 0 {
		merged.Providers = ecosystem.Providers
	}

	// Copy ecosystem dependencies first
	for k, v := range ecosystem.Dependencies {
		merged.Dependencies[k] = v
	}
	// Project dependencies override
	for k, v := range project.Dependencies {
		merged.Dependencies[k] = v
	}

	return merged
}

// CopySkillsConfig creates a deep copy of a SkillsConfig.
func CopySkillsConfig(cfg *SkillsConfig) *SkillsConfig {
	if cfg == nil {
		return nil
	}

	copied := &SkillsConfig{
		Use:          make([]string, len(cfg.Use)),
		Providers:    make([]string, len(cfg.Providers)),
		Dependencies: make(map[string]DependencyConfig),
	}

	copy(copied.Use, cfg.Use)
	copy(copied.Providers, cfg.Providers)

	for k, v := range cfg.Dependencies {
		copied.Dependencies[k] = v
	}

	return copied
}

// ApplySkillsDefaults applies default values to a SkillsConfig.
func ApplySkillsDefaults(cfg *SkillsConfig) *SkillsConfig {
	if cfg == nil {
		return nil
	}

	if len(cfg.Providers) == 0 {
		cfg.Providers = []string{"claude"}
	}
	cfg.Use = deduplicateSkillStrings(cfg.Use)

	return cfg
}

// unionSkillStrings returns the union of two string slices, preserving order.
func unionSkillStrings(a, b []string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, s := range a {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	for _, s := range b {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}

	return result
}

// deduplicateSkillStrings removes duplicates from a string slice while preserving order.
func deduplicateSkillStrings(input []string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, s := range input {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}

	return result
}
