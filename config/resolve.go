package config

import "fmt"

// ResolveEnvironment merges a named environment profile over the default environment.
// If profileName is empty, it returns the default environment.
// If profileName is set but not found in Environments, it returns an error.
func ResolveEnvironment(cfg *Config, profileName string) (*EnvironmentConfig, error) {
	// Start with a clone of the default environment
	resolved := &EnvironmentConfig{
		Config:   make(map[string]interface{}),
		Commands: make(map[string]string),
	}

	if cfg.Environment != nil {
		resolved.Provider = cfg.Environment.Provider
		resolved.Command = cfg.Environment.Command
		if cfg.Environment.Config != nil {
			resolved.Config = deepMergeMaps(nil, cfg.Environment.Config)
		}
		for k, v := range cfg.Environment.Commands {
			resolved.Commands[k] = v
		}
	}

	// If no profile requested, return the default
	if profileName == "" {
		return resolved, nil
	}

	// Overlay the named profile
	namedEnv, exists := cfg.Environments[profileName]
	if !exists || namedEnv == nil {
		return nil, fmt.Errorf("environment profile %q not found", profileName)
	}

	if namedEnv.Provider != "" {
		resolved.Provider = namedEnv.Provider
	}
	if namedEnv.Command != "" {
		resolved.Command = namedEnv.Command
	}
	if namedEnv.Config != nil {
		resolved.Config = deepMergeMaps(resolved.Config, namedEnv.Config)
	}
	for k, v := range namedEnv.Commands {
		resolved.Commands[k] = v
	}

	return resolved, nil
}
