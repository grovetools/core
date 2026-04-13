package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/grovetools/core/errors"
	"github.com/grovetools/core/pkg/paths"
	"github.com/pelletier/go-toml/v2"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// loadCacheEntry caches a resolved Config keyed by the absolute startDir passed
// to LoadFromWithLogger. The TTL is short enough to stay correct for users
// editing grove.toml interactively, and long enough to absorb thundering-herd
// callers (60fps TUI renders, 500ms fsnotify batches, long-lived watchers).
type loadCacheEntry struct {
	cfg    *Config
	stored time.Time
}

const loadCacheTTL = 2 * time.Second

var loadCache sync.Map // map[string]loadCacheEntry, keyed by absolute startDir

// ResetLoadCache clears the LoadFromWithLogger cache. Tests that mutate config
// files across sub-cases within the TTL window should call this between them;
// production code has no reason to touch it.
func ResetLoadCache() {
	loadCache.Range(func(key, _ any) bool {
		loadCache.Delete(key)
		return true
	})
}

var envVarRegex = regexp.MustCompile(`\$\{([^}]+)\}`)

// ConfigMeta holds metadata about the config file itself.
// This is parsed from the [_grove] section and stripped from the final config.
type ConfigMeta struct {
	Priority int `toml:"priority" yaml:"priority"` // Loading priority (higher loads later, default: 50)
}

// DefaultPriority is the default priority for config fragments.
const DefaultPriority = 50

// configFragment holds a config file path and its priority for sorting.
type configFragment struct {
	path     string
	priority int
}

// extractConfigMeta reads the [_grove] section from a config file to get metadata.
// Returns default values if the section doesn't exist.
func extractConfigMeta(data []byte, path string) ConfigMeta {
	meta := ConfigMeta{Priority: DefaultPriority}

	if strings.HasSuffix(path, ".toml") {
		var raw struct {
			Grove ConfigMeta `toml:"_grove"`
		}
		if err := toml.Unmarshal(data, &raw); err == nil && raw.Grove.Priority != 0 {
			meta.Priority = raw.Grove.Priority
		}
	} else {
		var raw struct {
			Grove ConfigMeta `yaml:"_grove"`
		}
		if err := yaml.Unmarshal(data, &raw); err == nil && raw.Grove.Priority != 0 {
			meta.Priority = raw.Grove.Priority
		}
	}

	return meta
}

// stripGroveMeta removes the [_grove] section from Extensions after loading.
func stripGroveMeta(cfg *Config) {
	if cfg.Extensions != nil {
		delete(cfg.Extensions, "_grove")
	}
}

// coreConfigKeys lists the known top-level keys that are part of the core Config struct.
// These are excluded from Extensions when loading TOML files.
var coreConfigKeys = map[string]bool{
	"name":              true,
	"version":           true,
	"workspaces":        true,
	"build_cmd":         true,
	"build_after":       true,
	"notebooks":         true,
	"tui":               true,
	"context":           true,
	"daemon":            true,
	"environment":       true,
	"environments":      true,
	"groves":            true,
	"search_paths":      true,
	"explicit_projects": true,
	"_grove":            true, // Meta section for config metadata (priority, etc.)
}

// unmarshalConfig parses config data based on file extension (TOML or YAML).
// For TOML files, it also captures extension fields into Extensions to emulate YAML inline behavior.
func unmarshalConfig(path string, data []byte) (*Config, error) {
	var cfg Config

	if strings.HasSuffix(path, ".toml") {
		if err := toml.Unmarshal(data, &cfg); err != nil {
			return nil, err
		}
		// Capture extension fields (non-core keys) into Extensions
		var raw map[string]interface{}
		if err := toml.Unmarshal(data, &raw); err == nil {
			extensions := make(map[string]interface{})
			for k, v := range raw {
				if !coreConfigKeys[k] {
					extensions[k] = v
				}
			}
			if len(extensions) > 0 {
				cfg.Extensions = extensions
			}
		}
		// Post-process TOML keybindings to handle simplified path format
		postProcessTOMLKeybindings(&cfg, data)
	} else {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, err
		}
	}

	return &cfg, nil
}

// postProcessTOMLKeybindings handles the simplified keybinding path format for TOML.
// It parses [tui.keybindings.package.tui] sections and populates TUIOverrides.
func postProcessTOMLKeybindings(cfg *Config, data []byte) {
	// Parse raw TOML to find keybinding sections
	var raw map[string]interface{}
	if err := toml.Unmarshal(data, &raw); err != nil {
		return
	}

	tuiRaw, ok := raw["tui"].(map[string]interface{})
	if !ok {
		return
	}

	kbRaw, ok := tuiRaw["keybindings"].(map[string]interface{})
	if !ok {
		return
	}

	// Known section names that apply globally (not package names)
	sectionNames := map[string]bool{
		"navigation": true, "selection": true, "actions": true,
		"search": true, "view": true, "fold": true, "system": true,
		"overrides": true, // Also skip the legacy overrides key
	}

	// Ensure TUI and Keybindings structs exist
	if cfg.TUI == nil {
		cfg.TUI = &TUIConfig{}
	}
	if cfg.TUI.Keybindings == nil {
		cfg.TUI.Keybindings = &KeybindingsConfig{}
	}

	// Process non-section keys as package names
	for pkgName, pkgValue := range kbRaw {
		if sectionNames[pkgName] {
			continue
		}

		// This should be a package name with TUI sub-keys
		pkgMap, ok := pkgValue.(map[string]interface{})
		if !ok {
			continue
		}

		for tuiName, tuiValue := range pkgMap {
			tuiMap, ok := tuiValue.(map[string]interface{})
			if !ok {
				continue
			}

			// Convert to KeybindingSectionConfig
			sectionConfig := make(KeybindingSectionConfig)
			for action, keys := range tuiMap {
				if arr, ok := keys.([]interface{}); ok {
					var strKeys []string
					for _, k := range arr {
						if s, ok := k.(string); ok {
							strKeys = append(strKeys, s)
						}
					}
					sectionConfig[action] = strKeys
				}
			}

			// Add to TUIOverrides
			if cfg.TUI.Keybindings.TUIOverrides == nil {
				cfg.TUI.Keybindings.TUIOverrides = make(map[string]map[string]KeybindingSectionConfig)
			}
			if cfg.TUI.Keybindings.TUIOverrides[pkgName] == nil {
				cfg.TUI.Keybindings.TUIOverrides[pkgName] = make(map[string]KeybindingSectionConfig)
			}
			cfg.TUI.Keybindings.TUIOverrides[pkgName][tuiName] = sectionConfig
		}
	}
}

// Load reads and parses a Grove configuration file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.ConfigNotFound(path)
		}
		return nil, errors.Wrap(err, errors.ErrCodeConfigInvalid, "failed to read config file").
			WithDetail("path", path)
	}

	if strings.HasSuffix(path, ".toml") {
		return LoadFromTOMLBytes(data)
	}

	return LoadFromBytes(data)
}

// LoadDefault finds and loads the configuration with hierarchical merging:
// 1. Global config (~/.config/grove/grove.yml) - base layer
// 2. Project config (grove.yml) - overrides global
// 3. Local override (grove.override.yml) - overrides all
func LoadDefault() (*Config, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeConfigInvalid, "failed to get current directory")
	}
	
	return LoadFrom(cwd)
}

// LoadFrom loads configuration with hierarchical merging starting from the given directory
func LoadFrom(startDir string) (*Config, error) {
	return LoadFromWithLogger(startDir, logrus.New())
}

// LoadFromWithLogger loads configuration with hierarchical merging and logging
func LoadFromWithLogger(startDir string, logger *logrus.Logger) (*Config, error) {
	// Short-lived cache keyed by absolute startDir. The full load path stats
	// and parses ~10 different hierarchical files plus compiles the JSONSchema
	// validator (~15ms), which shows up as a dominant hot path in long-lived
	// processes — cx TUI rendering, groved fsnotify handlers, nav ticker
	// loops, etc. 2s is long enough to absorb bursts from those callers and
	// short enough that interactive edits to grove.toml feel instant.
	cacheKey, _ := filepath.Abs(startDir)
	if cacheKey == "" {
		cacheKey = startDir
	}
	if raw, ok := loadCache.Load(cacheKey); ok {
		if entry, ok := raw.(loadCacheEntry); ok && time.Since(entry.stored) < loadCacheTTL {
			return entry.cfg, nil
		}
	}

	// Find project config file first
	projectPath, err := FindConfigFile(startDir)
	if err != nil {
		// If it's any error other than not found, we fail.
		if !errors.Is(err, errors.ErrCodeConfigNotFound) {
			return nil, err
		}
		projectPath = "" // No project file found, proceed without it.
	}

	// Start with an empty config
	var finalConfig *Config

	// 1. Load global config if it exists (optional)
	globalPath := getXDGConfigPath()
	if globalPath != "" {
		if _, err := os.Stat(globalPath); err == nil {
			logger.WithField("path", globalPath).Debug("Loading global configuration")
			// Load global config without validation/defaults (raw load)
			globalData, err := os.ReadFile(globalPath)
			if err == nil {
				expanded := expandEnvVars(string(globalData))
				globalConfig, parseErr := unmarshalConfig(globalPath, []byte(expanded))
				if parseErr == nil {
					finalConfig = globalConfig
				} else {
					logger.WithError(parseErr).Warn("Failed to parse global configuration, continuing without it")
				}
			} else {
				logger.WithError(err).Warn("Failed to read global configuration, continuing without it")
			}
		}

		// Glob and merge additional modular TOML files from config directory
		// Files are sorted by priority ([_grove].priority), then alphabetically within same priority
		globalDir := filepath.Dir(globalPath)
		pattern := filepath.Join(globalDir, "*.toml")
		if files, err := filepath.Glob(pattern); err == nil {
			// First pass: collect fragments with their priorities
			var fragments []configFragment
			for _, file := range files {
				baseName := filepath.Base(file)
				// Skip main config and override files
				if baseName == "grove.toml" || baseName == "grove.yml" || baseName == "grove.override.toml" {
					continue
				}

				fragmentData, err := os.ReadFile(file)
				if err != nil {
					logger.WithError(err).Warnf("Failed to read config fragment %s, skipping", baseName)
					continue
				}

				meta := extractConfigMeta(fragmentData, file)
				fragments = append(fragments, configFragment{path: file, priority: meta.Priority})
			}

			// Sort by priority (stable sort maintains alphabetical order within same priority)
			sort.SliceStable(fragments, func(i, j int) bool {
				return fragments[i].priority < fragments[j].priority
			})

			// Second pass: merge in priority order
			for _, frag := range fragments {
				baseName := filepath.Base(frag.path)
				logger.WithFields(logrus.Fields{
					"path":     frag.path,
					"priority": frag.priority,
				}).Debug("Loading global config fragment")

				fragmentData, err := os.ReadFile(frag.path)
				if err != nil {
					logger.WithError(err).Warnf("Failed to read config fragment %s, skipping", baseName)
					continue
				}

				expanded := expandEnvVars(string(fragmentData))
				fragmentConfig, parseErr := unmarshalConfig(frag.path, []byte(expanded))
				if parseErr != nil {
					logger.WithError(parseErr).Warnf("Failed to parse config fragment %s, skipping", baseName)
					continue
				}

				// Strip _grove meta section
				stripGroveMeta(fragmentConfig)

				if finalConfig == nil {
					finalConfig = fragmentConfig
				} else {
					finalConfig = mergeConfigs(finalConfig, fragmentConfig)
				}
			}
		}

		// Also glob ~/.config/grove/plugins/*.toml for per-user plugin manifests
		pluginPattern := filepath.Join(globalDir, "plugins", "*.toml")
		if pluginFiles, err := filepath.Glob(pluginPattern); err == nil {
			for _, file := range pluginFiles {
				baseName := filepath.Base(file)
				logger.WithField("path", file).Debug("Loading plugin config fragment")

				fragmentData, err := os.ReadFile(file)
				if err != nil {
					logger.WithError(err).Warnf("Failed to read plugin config %s, skipping", baseName)
					continue
				}

				expanded := expandEnvVars(string(fragmentData))
				fragmentConfig, parseErr := unmarshalConfig(file, []byte(expanded))
				if parseErr != nil {
					logger.WithError(parseErr).Warnf("Failed to parse plugin config %s, skipping", baseName)
					continue
				}

				stripGroveMeta(fragmentConfig)

				if finalConfig == nil {
					finalConfig = fragmentConfig
				} else {
					finalConfig = mergeConfigs(finalConfig, fragmentConfig)
				}
			}
		}
	}

	// Load global override if it exists
	if globalPath != "" {
		globalDir := filepath.Dir(globalPath)
		overrideFiles := []string{
			filepath.Join(globalDir, "grove.override.yml"),
			filepath.Join(globalDir, "grove.override.yaml"),
			filepath.Join(globalDir, "grove.override.toml"),
		}

		for _, overridePath := range overrideFiles {
			if _, err := os.Stat(overridePath); err == nil {
				logger.WithField("path", overridePath).Debug("Loading global override configuration")
				overrideData, err := os.ReadFile(overridePath)
				if err != nil {
					logger.WithError(err).Warn("Failed to read global override file, skipping")
					continue
				}
				expanded := expandEnvVars(string(overrideData))
				overrideConfig, parseErr := unmarshalConfig(overridePath, []byte(expanded))
				if parseErr != nil {
					logger.WithError(parseErr).Warn("Failed to parse global override file, skipping")
					continue
				}
				if finalConfig == nil {
					finalConfig = overrideConfig
				} else {
					finalConfig = mergeConfigs(finalConfig, overrideConfig)
				}
				break // Only load one
			}
		}
	}

	// Load GROVE_CONFIG_OVERLAY if set (for demo/testing environments)
	// Any field present in the overlay replaces the corresponding field in base config.
	if overlayPath := os.Getenv("GROVE_CONFIG_OVERLAY"); overlayPath != "" {
		overlayPath = expandPath(overlayPath)
		if _, err := os.Stat(overlayPath); err == nil {
			logger.WithField("path", overlayPath).Debug("Loading config overlay from GROVE_CONFIG_OVERLAY")
			overlayData, err := os.ReadFile(overlayPath)
			if err != nil {
				return nil, errors.Wrap(err, errors.ErrCodeConfigInvalid, "failed to read config overlay").
					WithDetail("path", overlayPath)
			}
			expanded := expandEnvVars(string(overlayData))
			overlayConfig, parseErr := unmarshalConfig(overlayPath, []byte(expanded))
			if parseErr != nil {
				return nil, errors.Wrap(parseErr, errors.ErrCodeConfigInvalid, "failed to parse config overlay").
					WithDetail("path", overlayPath)
			}
			if finalConfig == nil {
				finalConfig = overlayConfig
			} else {
				// Replace any non-zero field from overlay
				applyOverlay(finalConfig, overlayConfig)
			}
		} else if os.IsNotExist(err) {
			// If GROVE_CONFIG_OVERLAY is set but file doesn't exist, that's an error
			return nil, errors.ConfigNotFound(overlayPath).
				WithDetail("reason", "GROVE_CONFIG_OVERLAY path does not exist")
		} else {
			return nil, errors.Wrap(err, errors.ErrCodeConfigInvalid, "failed to access config overlay").
				WithDetail("path", overlayPath)
		}
	}

	// Detect when FindConfigFile fell through to the global config (no local project config).
	// In this case, skip loading it again as the "project" layer — it's already loaded as global.
	isGlobalFallback := projectPath != "" && globalPath != "" && projectPath == globalPath

	if projectPath != "" {
		logger.WithField("path", projectPath).Debug("Loading project configuration")
		// 2. Load and merge project config - also without defaults/validation
		projectData, err := os.ReadFile(projectPath)
		if err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeConfigInvalid, "failed to read project config").
				WithDetail("path", projectPath)
		}

		expanded := expandEnvVars(string(projectData))
		projectConfig, parseErr := unmarshalConfig(projectPath, []byte(expanded))
		if parseErr != nil {
			return nil, errors.Wrap(parseErr, errors.ErrCodeConfigInvalid, "failed to parse project config").
				WithDetail("path", projectPath)
		}

		// Check if this is a workspace config (has no workspaces field) and look for ecosystem config
		ecosystemPath := ""
		if !isGlobalFallback && len(projectConfig.Workspaces) == 0 {
			// This appears to be a workspace config, look for ecosystem config
			ecosystemPath = FindEcosystemConfig(filepath.Dir(projectPath))
			if ecosystemPath != "" {
				logger.WithField("path", ecosystemPath).Debug("Loading ecosystem configuration")
				ecosystemData, err := os.ReadFile(ecosystemPath)
				if err == nil {
					expandedEco := expandEnvVars(string(ecosystemData))
					ecosystemConfig, ecoParseErr := unmarshalConfig(ecosystemPath, []byte(expandedEco))
					if ecoParseErr == nil {
						// Merge ecosystem config after global but before project
						if finalConfig == nil {
							finalConfig = ecosystemConfig
						} else {
							logger.Debug("Merging ecosystem configuration over global configuration")
							finalConfig = mergeConfigs(finalConfig, ecosystemConfig)
						}
					} else {
						logger.WithError(ecoParseErr).Warn("Failed to parse ecosystem configuration, continuing without it")
					}
				} else {
					logger.WithError(err).Warn("Failed to read ecosystem configuration, continuing without it")
				}
			}
		}

		// Load notebook config (after ecosystem, before project local)
		//
		// Determine the correct project root for notebook resolution.
		// When FindConfigFile traversed up to the global/XDG config (no local
		// grove.toml exists), or found an ecosystem config with workspaces,
		// use startDir's git root as the project root. The notebook config is
		// keyed by the actual project directory name, not the config file's parent.
		projectRoot := filepath.Dir(projectPath)
		absStart, _ := filepath.Abs(startDir)
		absProjectRoot, _ := filepath.Abs(projectRoot)
		if absStart != absProjectRoot {
			// projectPath is not in startDir — it's an ancestor config (global/ecosystem).
			// Use git root or startDir as the actual project root for notebook lookup.
			if gitRoot, gitErr := getGitRoot(startDir); gitErr == nil && gitRoot != "" {
				projectRoot = gitRoot
			} else {
				projectRoot = startDir
			}
		}
		notebookConfigPath := findNotebookConfigPath(projectRoot, finalConfig)
		if notebookConfigPath != "" {
			logger.WithField("path", notebookConfigPath).Debug("Loading project notebook configuration")
			nbData, err := os.ReadFile(notebookConfigPath)
			if err == nil {
				expandedNb := expandEnvVars(string(nbData))
				nbConfig, parseErr := unmarshalConfig(notebookConfigPath, []byte(expandedNb))
				if parseErr == nil {
					stripGroveMeta(nbConfig)
					if finalConfig == nil {
						finalConfig = nbConfig
					} else {
						finalConfig = mergeConfigs(finalConfig, nbConfig)
					}
				} else {
					logger.WithError(parseErr).Warn("Failed to parse project notebook config, skipping")
				}
			}
		}

		if !isGlobalFallback {
			if finalConfig == nil {
				finalConfig = projectConfig
			} else {
				logger.Debug("Merging project configuration over global/ecosystem/notebook configuration")
				finalConfig = mergeConfigs(finalConfig, projectConfig)
			}
		}

		// 3. Load and merge override files if they exist (optional)
		projectDir := filepath.Dir(projectPath)
		overrideFiles := []string{
			filepath.Join(projectDir, "grove.override.yml"),
			filepath.Join(projectDir, "grove.override.yaml"),
			filepath.Join(projectDir, "grove.override.toml"),
			filepath.Join(projectDir, ".grove.override.yml"),
			filepath.Join(projectDir, ".grove.override.yaml"),
			filepath.Join(projectDir, ".grove.override.toml"),
		}

		for _, overridePath := range overrideFiles {
			if _, err := os.Stat(overridePath); err == nil {
				logger.WithField("path", overridePath).Debug("Loading local override configuration")

				overrideData, err := os.ReadFile(overridePath)
				if err != nil {
					logger.WithError(err).Warn("Failed to read override file, skipping")
					continue
				}

				// Expand environment variables
				expanded := expandEnvVars(string(overrideData))
				overrideConfig, parseErr := unmarshalConfig(overridePath, []byte(expanded))
				if parseErr != nil {
					logger.WithError(parseErr).Warn("Failed to parse override file, skipping")
					continue
				}

				finalConfig = mergeConfigs(finalConfig, overrideConfig)
			}
		}
	}

	// If no project config was found, still try loading notebook config
	if projectPath == "" && finalConfig != nil {
		projectRoot := startDir
		if gitRoot, err := getGitRoot(startDir); err == nil && gitRoot != "" {
			projectRoot = gitRoot
		}
		notebookConfigPath := findNotebookConfigPath(projectRoot, finalConfig)
		if notebookConfigPath != "" {
			logger.WithField("path", notebookConfigPath).Debug("Loading project notebook configuration (no local project config)")
			nbData, err := os.ReadFile(notebookConfigPath)
			if err == nil {
				expandedNb := expandEnvVars(string(nbData))
				nbConfig, parseErr := unmarshalConfig(notebookConfigPath, []byte(expandedNb))
				if parseErr == nil {
					stripGroveMeta(nbConfig)
					finalConfig = mergeConfigs(finalConfig, nbConfig)
				} else {
					logger.WithError(parseErr).Warn("Failed to parse project notebook config, skipping")
				}
			} else {
				logger.WithError(err).Warn("Failed to read project notebook config, skipping")
			}
		}
	}

	// If no configs were found at all, create an empty one to avoid nil pointers
	if finalConfig == nil {
		finalConfig = &Config{}
	}

	// Set defaults
	finalConfig.SetDefaults()

	logger.Debug("Configuration loaded and validated successfully")

	// Log the merged config at debug level
	if logger.IsLevelEnabled(logrus.DebugLevel) {
		configData, err := yaml.Marshal(finalConfig)
		if err == nil {
			logger.Debugf("Merged configuration:\n%s", string(configData))
		}
	}

	// Populate the short-lived cache for subsequent callers. Callers are
	// expected to treat the returned *Config as read-only; mutating it would
	// leak into other callers within the TTL window.
	loadCache.Store(cacheKey, loadCacheEntry{cfg: finalConfig, stored: time.Now()})

	return finalConfig, nil
}

// LoadFromBytes parses configuration from byte array
func LoadFromBytes(data []byte) (*Config, error) {
	// Expand environment variables
	expanded := expandEnvVars(string(data))

	var config Config
	if err := yaml.Unmarshal([]byte(expanded), &config); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeConfigInvalid, "failed to parse YAML configuration")
	}

	// Validate against schema
	validator, err := NewSchemaValidator()
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeConfigInvalid, "failed to create validator")
	}

	if err := validator.Validate(&config); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeConfigInvalid, "schema validation failed")
	}

	// Set defaults
	config.SetDefaults()

	return &config, nil
}

// LoadFromTOMLBytes parses configuration from TOML byte array
func LoadFromTOMLBytes(data []byte) (*Config, error) {
	// Expand environment variables
	expanded := expandEnvVars(string(data))

	var config Config
	if err := toml.Unmarshal([]byte(expanded), &config); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeConfigInvalid, "failed to parse TOML configuration")
	}

	// Capture extension fields (non-core keys) into Extensions
	// TOML doesn't support inline like YAML, so we unmarshal again to a raw map
	var raw map[string]interface{}
	if err := toml.Unmarshal([]byte(expanded), &raw); err == nil {
		extensions := make(map[string]interface{})
		for k, v := range raw {
			if !coreConfigKeys[k] {
				extensions[k] = v
			}
		}
		if len(extensions) > 0 {
			config.Extensions = extensions
		}
	}

	// Validate against schema
	validator, err := NewSchemaValidator()
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeConfigInvalid, "failed to create validator")
	}

	if err := validator.Validate(&config); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeConfigInvalid, "schema validation failed")
	}

	// Set defaults
	config.SetDefaults()

	return &config, nil
}

// FindConfigFile searches for grove configuration files with the following precedence:
// 1. Current directory up to filesystem root
// 2. Git repository root (if in a git repo)
// 3. XDG config directory (~/.config/grove/grove.yml)
func FindConfigFile(startDir string) (string, error) {
	configNames := []string{
		"grove.yml",
		"grove.yaml",
		"grove.toml",
		".grove.yml",
		".grove.yaml",
		".grove.toml",
		"docker-compose.grove.yml",
		"docker-compose.grove.yaml",
		"docker-compose.grove.toml",
	}

	// 1. Search from current directory up to filesystem root
	dir := startDir
	for {
		// Check each possible config name
		for _, name := range configNames {
			path := filepath.Join(dir, name)
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				return path, nil
			}
		}

		// Move to parent directory
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// 2. Check git repository root if we're in a git repo
	if gitRoot, err := getGitRoot(startDir); err == nil && gitRoot != "" {
		for _, name := range configNames {
			path := filepath.Join(gitRoot, name)
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				return path, nil
			}
		}
	}

	// 3. Check XDG config directory
	if xdgConfigPath := getXDGConfigPath(); xdgConfigPath != "" {
		if info, err := os.Stat(xdgConfigPath); err == nil && !info.IsDir() {
			return xdgConfigPath, nil
		}
	}

	return "", errors.ConfigNotFound(startDir).WithDetail("searchPath", startDir)
}

// expandPath expands ~ to home directory and environment variables in a path
func expandPath(path string) string {
	// First expand environment variables
	path = os.ExpandEnv(path)

	// Then handle ~ expansion
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[2:])
		}
	}

	return path
}

// applyOverlay replaces fields in base with non-zero fields from overlay.
func applyOverlay(base, overlay *Config) {
	if len(overlay.Groves) > 0 {
		base.Groves = overlay.Groves
	}
	if overlay.Notebooks != nil && overlay.Notebooks.Definitions != nil {
		if base.Notebooks == nil {
			base.Notebooks = &NotebooksConfig{}
		}
		base.Notebooks.Definitions = overlay.Notebooks.Definitions
	}
	if len(overlay.Extensions) > 0 {
		base.Extensions = overlay.Extensions
	}
}

// expandEnvVars replaces ${VAR} with environment variable values
func expandEnvVars(content string) string {
	return envVarRegex.ReplaceAllStringFunc(content, func(match string) string {
		varName := envVarRegex.FindStringSubmatch(match)[1]

		// Handle default values: ${VAR:-default}
		parts := strings.SplitN(varName, ":-", 2)
		varName = parts[0]
		defaultValue := ""
		if len(parts) > 1 {
			defaultValue = parts[1]
		}

		if value := os.Getenv(varName); value != "" {
			return value
		}

		return defaultValue
	})
}

// getGitRoot attempts to find the git repository root
func getGitRoot(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// getXDGConfigPath returns the XDG config path for Grove
func getXDGConfigPath() string {
	configDir := paths.ConfigDir()
	if configDir == "" {
		return ""
	}

	// Check YAML first
	yamlPath := filepath.Join(configDir, "grove.yml")
	if _, err := os.Stat(yamlPath); err == nil {
		return yamlPath
	}

	// Check TOML second
	tomlPath := filepath.Join(configDir, "grove.toml")
	if _, err := os.Stat(tomlPath); err == nil {
		return tomlPath
	}

	// Default to YAML if neither exists (for callers that might create it)
	return yamlPath
}

// FindEcosystemConfig searches upward from the given directory for a grove.yml
// that has a 'workspaces' field (indicating it's an ecosystem config)
func FindEcosystemConfig(startDir string) string {
	configNames := []string{
		"grove.yml",
		"grove.yaml",
		"grove.toml",
		".grove.yml",
		".grove.yaml",
		".grove.toml",
	}

	dir := startDir // Start from the given directory itself
	for {
		// Check for grove.yml with workspaces in this directory
		// Note: We check even inside .grove-worktrees because ecosystem worktrees
		// contain a full copy of the ecosystem including grove.yml with workspaces
		for _, name := range configNames {
			path := filepath.Join(dir, name)
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				// Check if this config has workspaces field
				data, err := os.ReadFile(path)
				if err == nil {
					expanded := expandEnvVars(string(data))
					var cfg Config
					if strings.HasSuffix(name, ".toml") {
						if err := toml.Unmarshal([]byte(expanded), &cfg); err != nil {
							continue
						}
					} else {
						if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
							continue
						}
					}
					// An ecosystem config is identified by having a non-empty 'workspaces' field.
					if len(cfg.Workspaces) > 0 {
						return path
					}
				}
			}
		}

		// Move to parent directory
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return ""
}

// notebookContext holds the resolved notebook workspace information for a project.
type notebookContext struct {
	notebookRootDir string
	workspaceName   string
}

// resolveNotebookContext finds the notebook workspace directory and name for a
// project root, using the global config to match against groves and resolve
// notebook definitions. Returns nil if the project is not in a grove or has no
// notebook configured.
func resolveNotebookContext(projectRoot string, cfg *Config) *notebookContext {
	if cfg == nil || len(cfg.Groves) == 0 {
		return nil
	}

	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil
	}

	var bestMatchGrove string
	var bestMatchNotebook string

	// Find the grove this project belongs to (longest prefix match)
	for _, grove := range cfg.Groves {
		if grove.Enabled != nil && !*grove.Enabled {
			continue
		}
		expandedGrove := expandPath(grove.Path)
		absGrove, err := filepath.Abs(expandedGrove)
		if err != nil {
			continue
		}

		if absRoot == absGrove || strings.HasPrefix(absRoot, absGrove+string(filepath.Separator)) {
			if len(absGrove) > len(bestMatchGrove) {
				bestMatchGrove = absGrove
				bestMatchNotebook = grove.Notebook
			}
		}
	}

	if bestMatchGrove == "" {
		return nil
	}

	// Extract workspace name as relative path from grove root
	wsName, err := filepath.Rel(bestMatchGrove, absRoot)
	if err != nil || wsName == "." {
		return nil
	}

	// Resolve the notebook name
	notebookName := bestMatchNotebook
	if notebookName == "" && cfg.Notebooks != nil && cfg.Notebooks.Rules != nil {
		notebookName = cfg.Notebooks.Rules.Default
	}
	if notebookName == "" {
		notebookName = "nb"
	}

	// Resolve the notebook root directory
	var notebookRootDir string
	if cfg.Notebooks != nil && cfg.Notebooks.Definitions != nil {
		if nb, ok := cfg.Notebooks.Definitions[notebookName]; ok && nb != nil {
			notebookRootDir = expandPath(nb.RootDir)
		}
	}

	if notebookRootDir == "" {
		return nil
	}

	return &notebookContext{
		notebookRootDir: notebookRootDir,
		workspaceName:   wsName,
	}
}

// findNotebookConfigPath resolves the path to a project's configuration file
// stored in its notebook directory. It uses the global config to find the grove
// the project belongs to, determine the notebook name, and construct the path.
func findNotebookConfigPath(projectRoot string, globalCfg *Config) string {
	ctx := resolveNotebookContext(projectRoot, globalCfg)
	if ctx == nil {
		return ""
	}

	configNames := []string{"grove.toml", "grove.yml", "grove.yaml"}
	for _, name := range configNames {
		configPath := filepath.Join(ctx.notebookRootDir, "workspaces", ctx.workspaceName, name)
		if info, err := os.Stat(configPath); err == nil && !info.IsDir() {
			return configPath
		}
	}

	return ""
}

// ResolveNotebookDir returns the notebook workspace directory for a project.
// Unlike findNotebookConfigPath (which checks for existing files), this returns
// the directory where a notebook config *should* be placed, even if it doesn't
// exist yet. Returns the directory path and the workspace name, or empty strings
// if the project is not in a grove or has no notebook configured.
func ResolveNotebookDir(projectRoot string) (dir string, workspaceName string, err error) {
	cfg, loadErr := LoadDefault()
	if loadErr != nil {
		return "", "", fmt.Errorf("failed to load config: %w", loadErr)
	}
	return resolveNotebookDirWithConfig(projectRoot, cfg)
}

func resolveNotebookDirWithConfig(projectRoot string, cfg *Config) (string, string, error) {
	ctx := resolveNotebookContext(projectRoot, cfg)
	if ctx == nil {
		if cfg == nil || len(cfg.Groves) == 0 {
			return "", "", fmt.Errorf("no groves configured")
		}
		return "", "", fmt.Errorf("project is not inside a configured grove or has no notebook configured")
	}

	return filepath.Join(ctx.notebookRootDir, "workspaces", ctx.workspaceName), ctx.workspaceName, nil
}

// LoadLayered finds and loads all configuration layers (global, project, overrides)
// without merging them, for analysis purposes. It also computes the final merged config.
func LoadLayered(startDir string) (*LayeredConfig, error) {
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel) // Suppress debug logs for this loader

	layeredConfig := &LayeredConfig{
		Overrides: make([]OverrideSource, 0),
		FilePaths: make(map[ConfigSource]string),
	}

	// 1. Determine Default layer
	defaultCfg := &Config{}
	defaultCfg.SetDefaults()
	// We don't run InferDefaults here as it depends on project structure which we haven't analyzed yet.
	// It will be part of the final merged config.
	layeredConfig.Default = defaultCfg

	// 2. Load Global layer (optional)
	globalPath := getXDGConfigPath()
	if globalPath != "" {
		if _, err := os.Stat(globalPath); err == nil {
			globalData, err := os.ReadFile(globalPath)
			if err == nil {
				expanded := expandEnvVars(string(globalData))
				globalConfig, parseErr := unmarshalConfig(globalPath, []byte(expanded))
				if parseErr == nil {
					layeredConfig.Global = globalConfig
					layeredConfig.FilePaths[SourceGlobal] = globalPath
				}
			}
		}

		// 2.25. Load Global Fragment layers (modular *.toml files)
		// Files are sorted by priority ([_grove].priority), then alphabetically within same priority
		globalDir := filepath.Dir(globalPath)
		pattern := filepath.Join(globalDir, "*.toml")
		if files, err := filepath.Glob(pattern); err == nil {
			// First pass: collect fragments with their priorities
			var fragments []configFragment
			for _, file := range files {
				baseName := filepath.Base(file)
				// Skip main config and override files
				if baseName == "grove.toml" || baseName == "grove.yml" || baseName == "grove.override.toml" {
					continue
				}

				fragmentData, err := os.ReadFile(file)
				if err != nil {
					continue
				}

				meta := extractConfigMeta(fragmentData, file)
				fragments = append(fragments, configFragment{path: file, priority: meta.Priority})
			}

			// Sort by priority (stable sort maintains alphabetical order within same priority)
			sort.SliceStable(fragments, func(i, j int) bool {
				return fragments[i].priority < fragments[j].priority
			})

			// Second pass: load in priority order
			for _, frag := range fragments {
				fragmentData, err := os.ReadFile(frag.path)
				if err != nil {
					continue
				}

				expanded := expandEnvVars(string(fragmentData))
				fragmentConfig, parseErr := unmarshalConfig(frag.path, []byte(expanded))
				if parseErr == nil {
					stripGroveMeta(fragmentConfig)
					layeredConfig.GlobalFragments = append(layeredConfig.GlobalFragments, OverrideSource{
						Path:   frag.path,
						Config: fragmentConfig,
					})
				}
			}
		}
	}

	// 2.5. Load Global Override layer (optional)
	if globalPath != "" {
		globalDir := filepath.Dir(globalPath)
		overrideFiles := []string{
			filepath.Join(globalDir, "grove.override.yml"),
			filepath.Join(globalDir, "grove.override.yaml"),
			filepath.Join(globalDir, "grove.override.toml"),
		}
		for _, overridePath := range overrideFiles {
			if _, err := os.Stat(overridePath); err == nil {
				overrideData, err := os.ReadFile(overridePath)
				if err == nil {
					expanded := expandEnvVars(string(overrideData))
					overrideConfig, parseErr := unmarshalConfig(overridePath, []byte(expanded))
					if parseErr == nil {
						layeredConfig.GlobalOverride = &OverrideSource{
							Path:   overridePath,
							Config: overrideConfig,
						}
						layeredConfig.FilePaths[SourceGlobalOverride] = overridePath
						break // Only load the first one found
					}
				}
			}
		}
	}

	// 2.75. Load GROVE_CONFIG_OVERLAY layer (optional)
	if overlayPath := os.Getenv("GROVE_CONFIG_OVERLAY"); overlayPath != "" {
		overlayPath = expandPath(overlayPath)
		if _, err := os.Stat(overlayPath); err == nil {
			overlayData, err := os.ReadFile(overlayPath)
			if err == nil {
				expanded := expandEnvVars(string(overlayData))
				overlayConfig, parseErr := unmarshalConfig(overlayPath, []byte(expanded))
				if parseErr == nil {
					layeredConfig.EnvOverlay = &OverrideSource{
						Path:   overlayPath,
						Config: overlayConfig,
					}
					layeredConfig.FilePaths[SourceEnvOverlay] = overlayPath
				}
			}
		}
	}

	// 3. Load Project layer (optional)
	projectPath, err := FindConfigFile(startDir)
	if err != nil {
		// If config not found, it's not a fatal error. We can proceed with just global/defaults.
		if !errors.Is(err, errors.ErrCodeConfigNotFound) {
			return nil, errors.Wrap(err, errors.ErrCodeConfigInvalid, "error while finding project config file")
		}
		projectPath = "" // No project file found, proceed without it.
	}

	if projectPath != "" {
		projectData, err := os.ReadFile(projectPath)
		if err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeConfigInvalid, "failed to read project config").WithDetail("path", projectPath)
		}
		expandedProject := expandEnvVars(string(projectData))
		projectConfig, parseErr := unmarshalConfig(projectPath, []byte(expandedProject))
		if parseErr != nil {
			return nil, errors.Wrap(parseErr, errors.ErrCodeConfigInvalid, "failed to parse project config").WithDetail("path", projectPath)
		}
		layeredConfig.Project = projectConfig
		layeredConfig.FilePaths[SourceProject] = projectPath

		// 3.5. Load Ecosystem layer (optional, only if this is a workspace config)
		if len(projectConfig.Workspaces) == 0 {
			ecosystemPath := FindEcosystemConfig(filepath.Dir(projectPath))
			if ecosystemPath != "" {
				ecosystemData, err := os.ReadFile(ecosystemPath)
				if err == nil {
					expandedEco := expandEnvVars(string(ecosystemData))
					ecosystemConfig, ecoParseErr := unmarshalConfig(ecosystemPath, []byte(expandedEco))
					if ecoParseErr == nil {
						layeredConfig.Ecosystem = ecosystemConfig
						layeredConfig.FilePaths[SourceEcosystem] = ecosystemPath
					}
				}
			}
		}
	}

	// 3.75. Load Project Notebook layer (optional)
	// Build a lookup config from global + ecosystem layers to resolve notebook paths.
	// Ecosystem config is included because notebooks may be defined there.
	lookupConfig := &Config{}
	if layeredConfig.Global != nil {
		lookupConfig = layeredConfig.Global
	}
	for _, fragment := range layeredConfig.GlobalFragments {
		lookupConfig = mergeConfigs(lookupConfig, fragment.Config)
	}
	if layeredConfig.GlobalOverride != nil {
		lookupConfig = mergeConfigs(lookupConfig, layeredConfig.GlobalOverride.Config)
	}
	if layeredConfig.Ecosystem != nil {
		lookupConfig = mergeConfigs(lookupConfig, layeredConfig.Ecosystem)
	}
	if layeredConfig.Project != nil {
		lookupConfig = mergeConfigs(lookupConfig, layeredConfig.Project)
	}

	projectRoot := startDir
	if projectPath != "" {
		projectRoot = filepath.Dir(projectPath)
		// When the found config is not in startDir (global/ecosystem ancestor),
		// use startDir's git root as the actual project root for notebook lookup.
		absStart, _ := filepath.Abs(startDir)
		absProjectRoot, _ := filepath.Abs(projectRoot)
		if absStart != absProjectRoot {
			if gitRoot, gitErr := getGitRoot(startDir); gitErr == nil && gitRoot != "" {
				projectRoot = gitRoot
			} else {
				projectRoot = startDir
			}
		}
	} else if gitRoot, err := getGitRoot(startDir); err == nil && gitRoot != "" {
		projectRoot = gitRoot
	}
	notebookConfigPath := findNotebookConfigPath(projectRoot, lookupConfig)
	if notebookConfigPath != "" {
		nbData, err := os.ReadFile(notebookConfigPath)
		if err == nil {
			expandedNb := expandEnvVars(string(nbData))
			nbConfig, parseErr := unmarshalConfig(notebookConfigPath, []byte(expandedNb))
			if parseErr == nil {
				stripGroveMeta(nbConfig)
				layeredConfig.ProjectNotebook = nbConfig
				layeredConfig.FilePaths[SourceProjectNotebook] = notebookConfigPath
			}
		}
	}

	// 4. Load Override layers (optional)
	if projectPath != "" {
		projectDir := filepath.Dir(projectPath)
		overrideFiles := []string{
			filepath.Join(projectDir, "grove.override.yml"),
			filepath.Join(projectDir, "grove.override.yaml"),
			filepath.Join(projectDir, "grove.override.toml"),
			filepath.Join(projectDir, ".grove.override.yml"),
			filepath.Join(projectDir, ".grove.override.yaml"),
			filepath.Join(projectDir, ".grove.override.toml"),
			// This also includes the previously named .grove-work.yml/.yaml/.toml
			filepath.Join(projectDir, ".grove-work.yml"),
			filepath.Join(projectDir, ".grove-work.yaml"),
			filepath.Join(projectDir, ".grove-work.toml"),
		}
		for _, overridePath := range overrideFiles {
			if _, err := os.Stat(overridePath); err == nil {
				overrideData, err := os.ReadFile(overridePath)
				if err != nil {
					continue // Skip unreadable override files
				}
				expandedOverride := expandEnvVars(string(overrideData))
				overrideConfig, parseErr := unmarshalConfig(overridePath, []byte(expandedOverride))
				if parseErr == nil {
					layeredConfig.Overrides = append(layeredConfig.Overrides, OverrideSource{
						Path:   overridePath,
						Config: overrideConfig,
					})
				}
			}
		}
	}

	// 5. Compute Final merged config
	// This logic is duplicated from LoadFrom, but necessary to build the final config for analysis.
	finalConfig := &Config{}

	// Start with global if it exists
	if layeredConfig.Global != nil {
		finalConfig = layeredConfig.Global
	}

	// Merge global fragments (modular *.toml files)
	for _, fragment := range layeredConfig.GlobalFragments {
		finalConfig = mergeConfigs(finalConfig, fragment.Config)
	}

	// Merge global override
	if layeredConfig.GlobalOverride != nil {
		finalConfig = mergeConfigs(finalConfig, layeredConfig.GlobalOverride.Config)
	}

	// Apply env overlay (GROVE_CONFIG_OVERLAY) - REPLACES groves/workspaces for isolation
	if layeredConfig.EnvOverlay != nil {
		applyOverlay(finalConfig, layeredConfig.EnvOverlay.Config)
	}

	// Merge ecosystem config (after global, before notebook)
	if layeredConfig.Ecosystem != nil {
		finalConfig = mergeConfigs(finalConfig, layeredConfig.Ecosystem)
	}

	// Merge project notebook config (after ecosystem, before project local)
	if layeredConfig.ProjectNotebook != nil {
		finalConfig = mergeConfigs(finalConfig, layeredConfig.ProjectNotebook)
	}

	// Merge project config
	if layeredConfig.Project != nil {
		finalConfig = mergeConfigs(finalConfig, layeredConfig.Project)
	}

	// Merge overrides (skip when overlay is active for full isolation)
	if layeredConfig.EnvOverlay == nil {
		for _, override := range layeredConfig.Overrides {
			finalConfig = mergeConfigs(finalConfig, override.Config)
		}
	}

	// Set defaults for the final merged config
	finalConfig.SetDefaults()

	layeredConfig.Final = finalConfig

	return layeredConfig, nil
}