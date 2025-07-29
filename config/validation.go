package config

import (
	"fmt"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/mattsolo1/grove-core/errors"
)

var (
	serviceNameRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)
	portRegex        = regexp.MustCompile(`^\d+:?\d*$`)
)

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Version now has a default, so no need to check

	// Validate services
	for name, service := range c.Services {
		if err := validateServiceName(name); err != nil {
			return errors.Wrap(err, errors.ErrCodeConfigValidation, fmt.Sprintf("invalid service name '%s'", name)).
				WithDetail("service", name)
		}

		svc := service
		if err := validateService(&svc); err != nil {
			return errors.Wrap(err, errors.ErrCodeConfigValidation, fmt.Sprintf("invalid service configuration for '%s'", name)).
				WithDetail("service", name)
		}
	}

	// Validate profiles
	for name, profile := range c.Profiles {
		prof := profile
		if err := validateProfile(name, &prof, c.Services); err != nil {
			return errors.Wrap(err, errors.ErrCodeConfigValidation, fmt.Sprintf("invalid profile '%s'", name)).
				WithDetail("profile", name)
		}
	}

	// Validate settings
	if err := validateSettings(&c.Settings); err != nil {
		return errors.Wrap(err, errors.ErrCodeConfigValidation, "invalid settings configuration")
	}

	// Validate agent configuration
	if c.Agent.Enabled {
		if c.Agent.Image == "" {
			return errors.New(errors.ErrCodeConfigValidation, "agent.image cannot be empty when agent is enabled")
		}
		// Validate that paths are not absolute Windows paths on Unix or vice versa
		if err := validatePath("agent.logs_path", c.Agent.LogsPath); err != nil {
			return err
		}
		// Validate extra volumes format
		for _, vol := range c.Agent.ExtraVolumes {
			if !strings.Contains(vol, ":") {
				return errors.New(errors.ErrCodeConfigValidation, fmt.Sprintf("invalid volume format: %s (must be host:container)", vol)).
					WithDetail("volume", vol)
			}
		}
	}

	return nil
}

func validateServiceName(name string) error {
	if !serviceNameRegex.MatchString(name) {
		return errors.New(errors.ErrCodeInvalidInput, "service name must start with letter and contain only letters, numbers, underscores, and hyphens").
			WithDetail("name", name)
	}
	return nil
}

func validateService(service *ServiceConfig) error {
	// Must have either build or image
	if service.Build == nil && service.Image == "" {
		return errors.New(errors.ErrCodeConfigValidation, "service must specify either 'build' or 'image'")
	}

	// Validate ports
	for _, port := range service.Ports {
		if !portRegex.MatchString(port) {
			return errors.New(errors.ErrCodeConfigValidation, fmt.Sprintf("invalid port format: %s", port)).
				WithDetail("port", port)
		}
	}

	// Validate dependencies exist
	for _, dep := range service.DependsOn {
		// Dependencies will be validated against service list later
		if dep == "" {
			return errors.New(errors.ErrCodeConfigValidation, "service dependency cannot be empty")
		}
	}

	return nil
}

func validateProfile(name string, profile *ProfileConfig, services map[string]ServiceConfig) error {
	if len(profile.Services) == 0 {
		return errors.New(errors.ErrCodeConfigValidation, "profile must specify at least one service")
	}

	// Check all services exist
	for _, serviceName := range profile.Services {
		if _, exists := services[serviceName]; !exists {
			return errors.ServiceNotFound(serviceName).
				WithDetail("profile", name)
		}
	}

	return nil
}

func validateSettings(settings *Settings) error {
	if settings.NetworkName == "" {
		return errors.New(errors.ErrCodeConfigValidation, "network name cannot be empty")
	}

	if settings.DomainSuffix == "" {
		return errors.New(errors.ErrCodeConfigValidation, "domain suffix cannot be empty")
	}

	// Validate domain suffix format
	if strings.Contains(settings.DomainSuffix, " ") {
		return errors.New(errors.ErrCodeConfigValidation, "domain suffix cannot contain spaces").
			WithDetail("domainSuffix", settings.DomainSuffix)
	}

	return nil
}

// validatePath validates that a path is appropriate for the current OS
func validatePath(fieldName, path string) error {
	if path == "" {
		return nil
	}

	// Check for Windows absolute paths on Unix systems
	if runtime.GOOS != "windows" && filepath.IsAbs(path) && strings.Contains(path, "\\") {
		return errors.New(errors.ErrCodeConfigValidation, fmt.Sprintf("%s contains Windows-style path on Unix system", fieldName)).
			WithDetail("path", path)
	}

	// Check for Unix absolute paths on Windows systems
	if runtime.GOOS == "windows" && strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "//") {
		return errors.New(errors.ErrCodeConfigValidation, fmt.Sprintf("%s contains Unix-style path on Windows system", fieldName)).
			WithDetail("path", path)
	}

	return nil
}
