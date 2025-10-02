package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// LoadWithOverrides loads configuration with override files
func LoadWithOverrides(baseFile string) (*Config, error) {
	// Load base configuration
	config, err := Load(baseFile)
	if err != nil {
		return nil, err
	}

	// Look for override files
	dir := filepath.Dir(baseFile)
	overrides := []string{
		filepath.Join(dir, "grove.override.yml"),
		filepath.Join(dir, "grove.override.yaml"),
		filepath.Join(dir, ".grove.override.yml"),
		filepath.Join(dir, ".grove.override.yaml"),
	}

	for _, overrideFile := range overrides {
		if _, err := os.Stat(overrideFile); err == nil {
			// Load override without validation
			data, err := os.ReadFile(overrideFile)
			if err != nil {
				return nil, fmt.Errorf("read override %s: %w", overrideFile, err)
			}

			// Expand environment variables
			expanded := expandEnvVars(string(data))

			var override Config
			if err := yaml.Unmarshal([]byte(expanded), &override); err != nil {
				return nil, fmt.Errorf("parse override %s: %w", overrideFile, err)
			}

			config = mergeConfigs(config, &override)
		}
	}

	return config, nil
}

// mergeConfigs merges override configuration into base
func mergeConfigs(base, override *Config) *Config {
	result := *base

	// Merge version
	if override.Version != "" {
		result.Version = override.Version
	}

	// Merge agent
	result.Agent = mergeAgent(result.Agent, override.Agent)

	// Merge extensions
	if override.Extensions != nil {
		if result.Extensions == nil {
			result.Extensions = make(map[string]interface{})
		}
		for key, value := range override.Extensions {
			// If both base and override have the same extension key, merge them
			if baseValue, exists := result.Extensions[key]; exists {
				if baseMap, baseOk := baseValue.(map[string]interface{}); baseOk {
					if overrideMap, overrideOk := value.(map[string]interface{}); overrideOk {
						// Merge the maps
						mergedMap := make(map[string]interface{})
						for k, v := range baseMap {
							mergedMap[k] = v
						}
						for k, v := range overrideMap {
							mergedMap[k] = v
						}
						result.Extensions[key] = mergedMap
						continue
					}
				}
			}
			// Otherwise just replace
			result.Extensions[key] = value
		}
	}

	return &result
}

func mergeAgent(base, override AgentConfig) AgentConfig {
	result := base

	if override.Enabled {
		result.Enabled = override.Enabled
	}
	if override.Image != "" {
		result.Image = override.Image
	}
	if override.LogsPath != "" {
		result.LogsPath = override.LogsPath
	}
	if len(override.ExtraVolumes) > 0 {
		result.ExtraVolumes = override.ExtraVolumes
	}
	if override.NotesDir != "" {
		result.NotesDir = override.NotesDir
	}
	if len(override.Args) > 0 {
		result.Args = override.Args
	}
	if override.OutputFormat != "" {
		result.OutputFormat = override.OutputFormat
	}
	if override.MountWorkspaceAtHostPath {
		result.MountWorkspaceAtHostPath = override.MountWorkspaceAtHostPath
	}
	if override.UseSuperprojectRoot {
		result.UseSuperprojectRoot = override.UseSuperprojectRoot
	}

	return result
}
