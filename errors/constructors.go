package errors

import (
	"fmt"
	"os/exec"
)

// ConfigNotFound creates a configuration not found error
func ConfigNotFound(path string) *GroveError {
	return New(ErrCodeConfigNotFound, fmt.Sprintf("configuration file not found: %s", path)).
		WithDetail("path", path)
}

// ConfigInvalid creates an invalid configuration error
func ConfigInvalid(reason string) *GroveError {
	return New(ErrCodeConfigInvalid, fmt.Sprintf("invalid configuration: %s", reason))
}

// ServiceNotFound creates a service not found error
func ServiceNotFound(service string) *GroveError {
	return New(ErrCodeServiceNotFound, fmt.Sprintf("service '%s' not found", service)).
		WithDetail("service", service)
}

// ContainerTimeout creates a container timeout error
func ContainerTimeout(service string, timeout string) *GroveError {
	return New(ErrCodeContainerTimeout,
		fmt.Sprintf("container '%s' failed to become ready within %s", service, timeout)).
		WithDetail("service", service).
		WithDetail("timeout", timeout)
}

// CommandFailed creates a command execution failure error
func CommandFailed(cmd string, err error) *GroveError {
	groveErr := Wrap(err, ErrCodeCommandFailed, fmt.Sprintf("command failed: %s", cmd)).
		WithDetail("command", cmd)

	// Extract exit code if available
	if exitErr, ok := err.(*exec.ExitError); ok {
		groveErr = groveErr.WithDetail("exitCode", exitErr.ExitCode())
	}

	return groveErr
}

// PortConflict creates a port conflict error
func PortConflict(port int, service string) *GroveError {
	return New(ErrCodePortConflict,
		fmt.Sprintf("port %d is already in use by another service", port)).
		WithDetail("port", port).
		WithDetail("conflictingService", service)
}