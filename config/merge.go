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

	// Merge services
	if override.Services != nil {
		if result.Services == nil {
			result.Services = make(map[string]ServiceConfig)
		}
		for name, service := range override.Services {
			result.Services[name] = mergeService(result.Services[name], service)
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

	// Merge settings
	result.Settings = mergeSettings(result.Settings, override.Settings)

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
	if override.NetworkName != "" {
		result.NetworkName = override.NetworkName
	}
	if override.DomainSuffix != "" {
		result.DomainSuffix = override.DomainSuffix
	}

	return result
}
