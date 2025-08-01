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

	// Validate the final merged config
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("validate merged config: %w", err)
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

	// Merge networks
	if override.Networks != nil {
		if result.Networks == nil {
			result.Networks = make(map[string]NetworkConfig)
		}
		for name, network := range override.Networks {
			result.Networks[name] = network
		}
	}

	// Merge services
	if override.Services != nil {
		if result.Services == nil {
			result.Services = make(map[string]ServiceConfig)
		}
		for name, service := range override.Services {
			result.Services[name] = mergeService(result.Services[name], service)
		}
	}

	// Merge volumes
	if override.Volumes != nil {
		if result.Volumes == nil {
			result.Volumes = make(map[string]VolumeConfig)
		}
		for name, volume := range override.Volumes {
			result.Volumes[name] = volume
		}
	}

	// Merge profiles
	if override.Profiles != nil {
		if result.Profiles == nil {
			result.Profiles = make(map[string]ProfileConfig)
		}
		for name, profile := range override.Profiles {
			result.Profiles[name] = profile
		}
	}

	// Merge agent
	result.Agent = mergeAgent(result.Agent, override.Agent)

	// Merge settings
	result.Settings = mergeSettings(result.Settings, override.Settings)

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

func mergeService(base, override ServiceConfig) ServiceConfig {
	result := base

	if override.Build != nil {
		result.Build = override.Build
	}
	if override.Image != "" {
		result.Image = override.Image
	}
	if len(override.Ports) > 0 {
		result.Ports = override.Ports
	}
	if len(override.Environment) > 0 {
		result.Environment = override.Environment
	}
	if len(override.Volumes) > 0 {
		result.Volumes = override.Volumes
	}
	if override.Command != nil {
		result.Command = override.Command
	}
	if len(override.Labels) > 0 {
		if result.Labels == nil {
			result.Labels = make(map[string]string)
		}
		for k, v := range override.Labels {
			result.Labels[k] = v
		}
	}

	return result
}

func mergeSettings(base, override Settings) Settings {
	result := base

	if override.ProjectName != "" {
		result.ProjectName = override.ProjectName
	}
	if override.DefaultProfile != "" {
		result.DefaultProfile = override.DefaultProfile
	}
	if override.TraefikEnabled != nil {
		result.TraefikEnabled = override.TraefikEnabled
	}
	if override.NetworkName != "" {
		result.NetworkName = override.NetworkName
	}
	if override.DomainSuffix != "" {
		result.DomainSuffix = override.DomainSuffix
	}
	if override.AutoInference != nil {
		result.AutoInference = override.AutoInference
	}
	if override.Concurrency > 0 {
		result.Concurrency = override.Concurrency
	}
	if override.McpPort > 0 {
		result.McpPort = override.McpPort
	}

	return result
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

	return result
}
