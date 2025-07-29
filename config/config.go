package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mattsolo1/grove-core/errors"
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