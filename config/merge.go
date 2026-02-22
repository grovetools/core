package config

import (
	"fmt"
	"os"
	"path/filepath"
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
		filepath.Join(dir, "grove.override.toml"),
		filepath.Join(dir, ".grove.override.yml"),
		filepath.Join(dir, ".grove.override.yaml"),
		filepath.Join(dir, ".grove.override.toml"),
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
			override, parseErr := unmarshalConfig(overrideFile, []byte(expanded))
			if parseErr != nil {
				return nil, fmt.Errorf("parse override %s: %w", overrideFile, parseErr)
			}

			config = mergeConfigs(config, override)
		}
	}

	return config, nil
}

// mergeKeybindingSection merges override keybindings into base.
// Override values replace base values for the same action key.
func mergeKeybindingSection(base, override KeybindingSectionConfig) KeybindingSectionConfig {
	if override == nil {
		return base
	}
	if base == nil {
		result := make(KeybindingSectionConfig)
		for k, v := range override {
			result[k] = v
		}
		return result
	}
	result := make(KeybindingSectionConfig)
	for k, v := range base {
		result[k] = v
	}
	for k, v := range override {
		result[k] = v
	}
	return result
}

// deepMergeMaps recursively merges two maps, with src values overriding dst values.
// When both dst and src have the same key pointing to maps, they are merged recursively.
func deepMergeMaps(dst, src map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{})
	for k, v := range dst {
		out[k] = v
	}
	for k, vSrc := range src {
		if vDst, ok := out[k]; ok {
			if mapDst, okDst := vDst.(map[string]interface{}); okDst {
				if mapSrc, okSrc := vSrc.(map[string]interface{}); okSrc {
					out[k] = deepMergeMaps(mapDst, mapSrc)
					continue
				}
			}
		}
		out[k] = vSrc
	}
	return out
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
	if override.BuildCmd != "" {
		result.BuildCmd = override.BuildCmd
	}

	// Merge slice fields (replace if present)
	if override.Workspaces != nil {
		result.Workspaces = override.Workspaces
	}
	if override.BuildAfter != nil {
		result.BuildAfter = override.BuildAfter
	}
	if override.ExplicitProjects != nil {
		result.ExplicitProjects = override.ExplicitProjects
	}

	// Merge TUI configuration
	if override.TUI != nil {
		if result.TUI == nil {
			result.TUI = &TUIConfig{}
		}
		if override.TUI.Icons != "" {
			result.TUI.Icons = override.TUI.Icons
		}
		if override.TUI.Theme != "" {
			result.TUI.Theme = override.TUI.Theme
		}
		if override.TUI.Preset != "" {
			result.TUI.Preset = override.TUI.Preset
		}
		if override.TUI.NvimEmbed != nil {
			result.TUI.NvimEmbed = override.TUI.NvimEmbed
		}

		// Merge Keybindings
		if override.TUI.Keybindings != nil {
			if result.TUI.Keybindings == nil {
				result.TUI.Keybindings = &KeybindingsConfig{}
			}

			// Merge standard sections
			result.TUI.Keybindings.Navigation = mergeKeybindingSection(result.TUI.Keybindings.Navigation, override.TUI.Keybindings.Navigation)
			result.TUI.Keybindings.Selection = mergeKeybindingSection(result.TUI.Keybindings.Selection, override.TUI.Keybindings.Selection)
			result.TUI.Keybindings.Actions = mergeKeybindingSection(result.TUI.Keybindings.Actions, override.TUI.Keybindings.Actions)
			result.TUI.Keybindings.Search = mergeKeybindingSection(result.TUI.Keybindings.Search, override.TUI.Keybindings.Search)
			result.TUI.Keybindings.View = mergeKeybindingSection(result.TUI.Keybindings.View, override.TUI.Keybindings.View)
			result.TUI.Keybindings.Fold = mergeKeybindingSection(result.TUI.Keybindings.Fold, override.TUI.Keybindings.Fold)
			result.TUI.Keybindings.System = mergeKeybindingSection(result.TUI.Keybindings.System, override.TUI.Keybindings.System)

			// Merge TUIOverrides (per-TUI overrides) - these have yaml:"-" toml:"-" tags
			// so they must be manually merged to preserve them across config merges
			if override.TUI.Keybindings.TUIOverrides != nil {
				if result.TUI.Keybindings.TUIOverrides == nil {
					result.TUI.Keybindings.TUIOverrides = make(map[string]map[string]KeybindingSectionConfig)
				}
				for pkgName, pkgOverrides := range override.TUI.Keybindings.TUIOverrides {
					if result.TUI.Keybindings.TUIOverrides[pkgName] == nil {
						result.TUI.Keybindings.TUIOverrides[pkgName] = make(map[string]KeybindingSectionConfig)
					}
					for tuiName, tuiOverrides := range pkgOverrides {
						result.TUI.Keybindings.TUIOverrides[pkgName][tuiName] = mergeKeybindingSection(
							result.TUI.Keybindings.TUIOverrides[pkgName][tuiName],
							tuiOverrides,
						)
					}
				}
			}

			// Merge legacy Overrides map for backward compatibility
			if override.TUI.Keybindings.Overrides != nil {
				if result.TUI.Keybindings.Overrides == nil {
					result.TUI.Keybindings.Overrides = make(map[string]map[string]KeybindingSectionConfig)
				}
				for pkgName, pkgOverrides := range override.TUI.Keybindings.Overrides {
					if result.TUI.Keybindings.Overrides[pkgName] == nil {
						result.TUI.Keybindings.Overrides[pkgName] = make(map[string]KeybindingSectionConfig)
					}
					for tuiName, tuiOverrides := range pkgOverrides {
						result.TUI.Keybindings.Overrides[pkgName][tuiName] = mergeKeybindingSection(
							result.TUI.Keybindings.Overrides[pkgName][tuiName],
							tuiOverrides,
						)
					}
				}
			}
		}
	}

	// Merge Notebooks configuration (now nested under NotebooksConfig)
	if override.Notebooks != nil {
		if result.Notebooks == nil {
			result.Notebooks = &NotebooksConfig{}
		}

		// Merge Definitions
		if override.Notebooks.Definitions != nil {
			if result.Notebooks.Definitions == nil {
				result.Notebooks.Definitions = make(map[string]*Notebook)
			}
			for k, v := range override.Notebooks.Definitions {
				if v != nil {
					// Deep merge notebook fields instead of replacing
					if existing, exists := result.Notebooks.Definitions[k]; exists && existing != nil {
						merged := *existing // Copy existing
						// Override non-empty fields
						if v.RootDir != "" {
							merged.RootDir = v.RootDir
						}
						if v.NotesPathTemplate != "" {
							merged.NotesPathTemplate = v.NotesPathTemplate
						}
						if v.PlansPathTemplate != "" {
							merged.PlansPathTemplate = v.PlansPathTemplate
						}
						if v.ChatsPathTemplate != "" {
							merged.ChatsPathTemplate = v.ChatsPathTemplate
						}
						if v.TemplatesPathTemplate != "" {
							merged.TemplatesPathTemplate = v.TemplatesPathTemplate
						}
						if v.RecipesPathTemplate != "" {
							merged.RecipesPathTemplate = v.RecipesPathTemplate
						}
						if v.Types != nil {
							if merged.Types == nil {
								merged.Types = make(map[string]*NoteTypeConfig)
							}
							for typeKey, typeVal := range v.Types {
								merged.Types[typeKey] = typeVal
							}
						}
						result.Notebooks.Definitions[k] = &merged
					} else {
						// No existing notebook, just use the override
						result.Notebooks.Definitions[k] = v
					}
				}
			}
		}

		// Merge Rules
		if override.Notebooks.Rules != nil {
			if result.Notebooks.Rules == nil {
				result.Notebooks.Rules = &NotebookRules{}
			}
			if override.Notebooks.Rules.Default != "" {
				result.Notebooks.Rules.Default = override.Notebooks.Rules.Default
			}
			if override.Notebooks.Rules.Global != nil && override.Notebooks.Rules.Global.RootDir != "" {
				if result.Notebooks.Rules.Global == nil {
					result.Notebooks.Rules.Global = &GlobalNotebookConfig{}
				}
				result.Notebooks.Rules.Global.RootDir = override.Notebooks.Rules.Global.RootDir
			}
		}
	}

	// Merge Groves map
	if override.Groves != nil {
		if result.Groves == nil {
			result.Groves = make(map[string]GroveSourceConfig)
		}
		for k, v := range override.Groves {
			result.Groves[k] = v
		}
	}

	// Merge SearchPaths map (legacy support)
	if override.SearchPaths != nil {
		if result.SearchPaths == nil {
			result.SearchPaths = make(map[string]SearchPathConfig)
		}
		for k, v := range override.SearchPaths {
			result.SearchPaths[k] = v
		}
	}

	// Merge extensions with recursive deep merge
	if override.Extensions != nil {
		if result.Extensions == nil {
			result.Extensions = make(map[string]interface{})
		}
		result.Extensions = deepMergeMaps(result.Extensions, override.Extensions)
	}

	return &result
}
