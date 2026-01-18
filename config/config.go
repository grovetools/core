package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/grovetools/core/errors"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

var envVarRegex = regexp.MustCompile(`\$\{([^}]+)\}`)

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
				var globalConfig Config
				if err := yaml.Unmarshal([]byte(expanded), &globalConfig); err == nil {
					finalConfig = &globalConfig
				} else {
					logger.WithError(err).Warn("Failed to parse global configuration, continuing without it")
				}
			} else {
				logger.WithError(err).Warn("Failed to read global configuration, continuing without it")
			}
		}
	}

	// Load global override if it exists
	if globalPath != "" {
		globalDir := filepath.Dir(globalPath)
		overrideFiles := []string{
			filepath.Join(globalDir, "grove.override.yml"),
			filepath.Join(globalDir, "grove.override.yaml"),
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
				var overrideConfig Config
				if err := yaml.Unmarshal([]byte(expanded), &overrideConfig); err != nil {
					logger.WithError(err).Warn("Failed to parse global override file, skipping")
					continue
				}
				if finalConfig == nil {
					finalConfig = &overrideConfig
				} else {
					finalConfig = mergeConfigs(finalConfig, &overrideConfig)
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
			var overlayConfig Config
			if err := yaml.Unmarshal([]byte(expanded), &overlayConfig); err != nil {
				return nil, errors.Wrap(err, errors.ErrCodeConfigInvalid, "failed to parse config overlay").
					WithDetail("path", overlayPath)
			}
			if finalConfig == nil {
				finalConfig = &overlayConfig
			} else {
				// Replace any non-zero field from overlay
				applyOverlay(finalConfig, &overlayConfig)
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

	if projectPath != "" {
		logger.WithField("path", projectPath).Debug("Loading project configuration")
		// 2. Load and merge project config - also without defaults/validation
		projectData, err := os.ReadFile(projectPath)
		if err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeConfigInvalid, "failed to read project config").
				WithDetail("path", projectPath)
		}

		expanded := expandEnvVars(string(projectData))
		var projectConfig Config
		if err := yaml.Unmarshal([]byte(expanded), &projectConfig); err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeConfigInvalid, "failed to parse project config").
				WithDetail("path", projectPath)
		}

		// Check if this is a workspace config (has no workspaces field) and look for ecosystem config
		ecosystemPath := ""
		if len(projectConfig.Workspaces) == 0 {
			// This appears to be a workspace config, look for ecosystem config
			ecosystemPath = FindEcosystemConfig(filepath.Dir(projectPath))
			if ecosystemPath != "" {
				logger.WithField("path", ecosystemPath).Debug("Loading ecosystem configuration")
				ecosystemData, err := os.ReadFile(ecosystemPath)
				if err == nil {
					expandedEco := expandEnvVars(string(ecosystemData))
					var ecosystemConfig Config
					if err := yaml.Unmarshal([]byte(expandedEco), &ecosystemConfig); err == nil {
						// Merge ecosystem config after global but before project
						if finalConfig == nil {
							finalConfig = &ecosystemConfig
						} else {
							logger.Debug("Merging ecosystem configuration over global configuration")
							finalConfig = mergeConfigs(finalConfig, &ecosystemConfig)
						}
					} else {
						logger.WithError(err).Warn("Failed to parse ecosystem configuration, continuing without it")
					}
				} else {
					logger.WithError(err).Warn("Failed to read ecosystem configuration, continuing without it")
				}
			}
		}

		if finalConfig == nil {
			finalConfig = &projectConfig
		} else {
			logger.Debug("Merging project configuration over global/ecosystem configuration")
			finalConfig = mergeConfigs(finalConfig, &projectConfig)
		}

		// 3. Load and merge override files if they exist (optional)
		projectDir := filepath.Dir(projectPath)
		overrideFiles := []string{
			filepath.Join(projectDir, "grove.override.yml"),
			filepath.Join(projectDir, "grove.override.yaml"),
			filepath.Join(projectDir, ".grove.override.yml"),
			filepath.Join(projectDir, ".grove.override.yaml"),
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

				var overrideConfig Config
				if err := yaml.Unmarshal([]byte(expanded), &overrideConfig); err != nil {
					logger.WithError(err).Warn("Failed to parse override file, skipping")
					continue
				}

				finalConfig = mergeConfigs(finalConfig, &overrideConfig)
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

// FindConfigFile searches for grove configuration files with the following precedence:
// 1. Current directory up to filesystem root
// 2. Git repository root (if in a git repo)  
// 3. XDG config directory (~/.config/grove/grove.yml)
func FindConfigFile(startDir string) (string, error) {
	configNames := []string{
		"grove.yml",
		"grove.yaml",
		".grove.yml",
		".grove.yaml",
		"docker-compose.grove.yml",
		"docker-compose.grove.yaml",
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
	// Check XDG_CONFIG_HOME first
	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		return filepath.Join(xdgConfig, "grove", "grove.yml")
	}

	// Fall back to ~/.config
	if homeDir, err := os.UserHomeDir(); err == nil {
		return filepath.Join(homeDir, ".config", "grove", "grove.yml")
	}

	return ""
}

// FindEcosystemConfig searches upward from the given directory for a grove.yml
// that has a 'workspaces' field (indicating it's an ecosystem config)
func FindEcosystemConfig(startDir string) string {
	configNames := []string{
		"grove.yml",
		"grove.yaml",
		".grove.yml",
		".grove.yaml",
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
					if err := yaml.Unmarshal([]byte(expanded), &cfg); err == nil {
						// An ecosystem config is identified by having a non-empty 'workspaces' field.
						if len(cfg.Workspaces) > 0 {
							return path
						}
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
				var globalConfig Config
				if err := yaml.Unmarshal([]byte(expanded), &globalConfig); err == nil {
					layeredConfig.Global = &globalConfig
					layeredConfig.FilePaths[SourceGlobal] = globalPath
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
		}
		for _, overridePath := range overrideFiles {
			if _, err := os.Stat(overridePath); err == nil {
				overrideData, err := os.ReadFile(overridePath)
				if err == nil {
					expanded := expandEnvVars(string(overrideData))
					var overrideConfig Config
					if err := yaml.Unmarshal([]byte(expanded), &overrideConfig); err == nil {
						layeredConfig.GlobalOverride = &OverrideSource{
							Path:   overridePath,
							Config: &overrideConfig,
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
				var overlayConfig Config
				if err := yaml.Unmarshal([]byte(expanded), &overlayConfig); err == nil {
					layeredConfig.EnvOverlay = &OverrideSource{
						Path:   overlayPath,
						Config: &overlayConfig,
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
		var projectConfig Config
		if err := yaml.Unmarshal([]byte(expandedProject), &projectConfig); err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeConfigInvalid, "failed to parse project config").WithDetail("path", projectPath)
		}
		layeredConfig.Project = &projectConfig
		layeredConfig.FilePaths[SourceProject] = projectPath

		// 3.5. Load Ecosystem layer (optional, only if this is a workspace config)
		if len(projectConfig.Workspaces) == 0 {
			ecosystemPath := FindEcosystemConfig(filepath.Dir(projectPath))
			if ecosystemPath != "" {
				ecosystemData, err := os.ReadFile(ecosystemPath)
				if err == nil {
					expandedEco := expandEnvVars(string(ecosystemData))
					var ecosystemConfig Config
					if err := yaml.Unmarshal([]byte(expandedEco), &ecosystemConfig); err == nil {
						layeredConfig.Ecosystem = &ecosystemConfig
						layeredConfig.FilePaths[SourceEcosystem] = ecosystemPath
					}
				}
			}
		}
	}

	// 4. Load Override layers (optional)
	if projectPath != "" {
		projectDir := filepath.Dir(projectPath)
		overrideFiles := []string{
			filepath.Join(projectDir, "grove.override.yml"),
			filepath.Join(projectDir, "grove.override.yaml"),
			filepath.Join(projectDir, ".grove.override.yml"),
			filepath.Join(projectDir, ".grove.override.yaml"),
			// This also includes the previously named .grove-work.yml/.yaml
			filepath.Join(projectDir, ".grove-work.yml"),
			filepath.Join(projectDir, ".grove-work.yaml"),
		}
		for _, overridePath := range overrideFiles {
			if _, err := os.Stat(overridePath); err == nil {
				overrideData, err := os.ReadFile(overridePath)
				if err != nil {
					continue // Skip unreadable override files
				}
				expandedOverride := expandEnvVars(string(overrideData))
				var overrideConfig Config
				if err := yaml.Unmarshal([]byte(expandedOverride), &overrideConfig); err == nil {
					layeredConfig.Overrides = append(layeredConfig.Overrides, OverrideSource{
						Path:   overridePath,
						Config: &overrideConfig,
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

	// Merge global override
	if layeredConfig.GlobalOverride != nil {
		finalConfig = mergeConfigs(finalConfig, layeredConfig.GlobalOverride.Config)
	}

	// Apply env overlay (GROVE_CONFIG_OVERLAY) - REPLACES groves/workspaces for isolation
	if layeredConfig.EnvOverlay != nil {
		applyOverlay(finalConfig, layeredConfig.EnvOverlay.Config)
	}

	// Merge ecosystem config (after global, before project)
	if layeredConfig.Ecosystem != nil {
		finalConfig = mergeConfigs(finalConfig, layeredConfig.Ecosystem)
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