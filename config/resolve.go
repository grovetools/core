package config

import (
	"fmt"
	"path/filepath"
)

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

// ResolveEnvironmentWithProvenance resolves a named environment profile
// across all raw config layers and also returns per-key provenance and a map
// of keys dropped via `_delete = true`.
//
// Provenance keys are dotted paths:
//   - "provider", "command" — peer scalars on EnvironmentConfig
//   - "commands.<name>" — entries in the commands map
//   - "config.<...>" — entries in the nested Config map (leaves only; arrays
//     are treated as scalars because deepMergeMaps replaces them wholesale)
//
// Values look like `<layer> (<block>)`, e.g. `"project (environments.hybrid-api)"`,
// so a reader can pinpoint the exact config block that produced a setting.
//
// This resolver walks the raw layers in order (global, fragments, global
// override, ecosystem, project-notebook, project, overrides). For each layer
// it first applies the default `[environment]` block, then — in a second pass
// — overlays the named profile block. That matches how the normal merge path
// produces a "fully-merged named profile wins over fully-merged default".
//
// Existing callers of ResolveEnvironment are untouched; this is additive.
func ResolveEnvironmentWithProvenance(layered *LayeredConfig, profileName string) (*EnvironmentConfig, map[string]string, map[string]string, error) {
	prov := make(map[string]string)
	deleted := make(map[string]string)
	resolved := &EnvironmentConfig{
		Config:   make(map[string]interface{}),
		Commands: make(map[string]string),
	}

	if layered == nil {
		if profileName != "" {
			return nil, nil, nil, fmt.Errorf("environment profile %q not found", profileName)
		}
		return resolved, prov, deleted, nil
	}

	type layerEntry struct {
		cfg    *Config
		source string
	}
	var layers []layerEntry
	if layered.Global != nil {
		layers = append(layers, layerEntry{layered.Global, string(SourceGlobal)})
	}
	for _, f := range layered.GlobalFragments {
		layers = append(layers, layerEntry{f.Config, fmt.Sprintf("%s (%s)", SourceGlobalFragment, filepath.Base(f.Path))})
	}
	if layered.GlobalOverride != nil {
		layers = append(layers, layerEntry{layered.GlobalOverride.Config, string(SourceGlobalOverride)})
	}
	if layered.Ecosystem != nil {
		layers = append(layers, layerEntry{layered.Ecosystem, string(SourceEcosystem)})
	}
	if layered.ProjectNotebook != nil {
		layers = append(layers, layerEntry{layered.ProjectNotebook, string(SourceProjectNotebook)})
	}
	if layered.Project != nil {
		layers = append(layers, layerEntry{layered.Project, string(SourceProject)})
	}
	for _, o := range layered.Overrides {
		layers = append(layers, layerEntry{o.Config, fmt.Sprintf("%s (%s)", SourceOverride, filepath.Base(o.Path))})
	}

	// Step A: base default env from all layers in order.
	for _, layer := range layers {
		if layer.cfg == nil || layer.cfg.Environment == nil {
			continue
		}
		applyEnvWithProvenance(resolved, layer.cfg.Environment, layer.source+" (environment)", prov, deleted)
	}

	if profileName == "" {
		return resolved, prov, deleted, nil
	}

	// Step B: named profile overlay across all layers.
	found := false
	for _, layer := range layers {
		if layer.cfg == nil || layer.cfg.Environments == nil {
			continue
		}
		namedEnv, ok := layer.cfg.Environments[profileName]
		if !ok || namedEnv == nil {
			continue
		}
		found = true
		applyEnvWithProvenance(resolved, namedEnv, fmt.Sprintf("%s (environments.%s)", layer.source, profileName), prov, deleted)
	}
	if !found {
		return nil, nil, nil, fmt.Errorf("environment profile %q not found", profileName)
	}

	return resolved, prov, deleted, nil
}

// applyEnvWithProvenance merges a single EnvironmentConfig layer into resolved
// and records provenance for every key it touches. `sourceLabel` is the fully
// qualified origin string (e.g. "project (environments.hybrid-api)").
func applyEnvWithProvenance(resolved, env *EnvironmentConfig, sourceLabel string, prov, deleted map[string]string) {
	if env == nil {
		return
	}
	if env.Provider != "" {
		resolved.Provider = env.Provider
		prov["provider"] = sourceLabel
	}
	if env.Command != "" {
		resolved.Command = env.Command
		prov["command"] = sourceLabel
	}
	if env.Config != nil {
		resolved.Config = deepMergeMapsWithProvenance(resolved.Config, env.Config, sourceLabel, "config", prov, deleted)
	}
	for k, v := range env.Commands {
		resolved.Commands[k] = v
		prov["commands."+k] = sourceLabel
	}
}
