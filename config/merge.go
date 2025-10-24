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

	// Merge simple string fields
	if override.Name != "" {
		result.Name = override.Name
	}
	if override.Version != "" {
		result.Version = override.Version
	}

	// Merge slice fields (replace if present)
	if override.Workspaces != nil {
		result.Workspaces = override.Workspaces
	}
	if override.ExplicitProjects != nil {
		result.ExplicitProjects = override.ExplicitProjects
	}

	// Merge pointer fields (replace if present)
	if override.Notebook != nil {
		result.Notebook = override.Notebook
	}

	// Merge SearchPaths map
	if override.SearchPaths != nil {
		if result.SearchPaths == nil {
			result.SearchPaths = make(map[string]SearchPathConfig)
		}
		for k, v := range override.SearchPaths {
			result.SearchPaths[k] = v
		}
	}

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
