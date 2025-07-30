package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mattsolo1/grove-core/errors"
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
	// Find project config file first (it's required)
	projectPath, err := FindConfigFile(startDir)
	if err != nil {
		return nil, err
	}

	logger.WithField("path", projectPath).Debug("Loading project configuration")

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

	// 2. Load and merge project config (required) - also without defaults/validation
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
	
	if finalConfig == nil {
		finalConfig = &projectConfig
	} else {
		logger.Debug("Merging project configuration over global configuration")
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

	// Set defaults and validate
	finalConfig.SetDefaults()

	// Apply smart inference (enabled by default)
	if finalConfig.Settings.AutoInference == nil || *finalConfig.Settings.AutoInference {
		finalConfig.InferDefaults()
	}

	// Validate configuration
	if err := finalConfig.Validate(); err != nil {
		return nil, err
	}

	// Additional semantic validation
	if err := finalConfig.ValidateSemantics(); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeConfigInvalid, "semantic validation failed")
	}

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

	// Apply smart inference (enabled by default)
	if config.Settings.AutoInference == nil || *config.Settings.AutoInference {
		config.InferDefaults()
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, err // Already returns structured error from validation
	}

	// Additional semantic validation
	if err := config.ValidateSemantics(); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeConfigInvalid, "semantic validation failed")
	}

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