package errors

import (
	"encoding/json"
	"fmt"
)

// ErrorCode represents a specific error condition
type ErrorCode string

const (
	// Configuration errors
	ErrCodeConfigNotFound   ErrorCode = "CONFIG_NOT_FOUND"
	ErrCodeConfigInvalid    ErrorCode = "CONFIG_INVALID"
	ErrCodeConfigValidation ErrorCode = "CONFIG_VALIDATION"

	// Container runtime errors
	ErrCodeContainerNotRunning ErrorCode = "CONTAINER_NOT_RUNNING"
	ErrCodeContainerTimeout    ErrorCode = "CONTAINER_TIMEOUT"
	ErrCodeComposeNotFound     ErrorCode = "COMPOSE_NOT_FOUND"
	ErrCodeServiceNotFound     ErrorCode = "SERVICE_NOT_FOUND"
	ErrCodeServiceNotRunning   ErrorCode = "SERVICE_NOT_RUNNING"

	// Network errors
	ErrCodePortConflict  ErrorCode = "PORT_CONFLICT"
	ErrCodeNetworkCreate ErrorCode = "NETWORK_CREATE"

	// Command execution errors
	ErrCodeCommandTimeout  ErrorCode = "COMMAND_TIMEOUT"
	ErrCodeCommandNotFound ErrorCode = "COMMAND_NOT_FOUND"
	ErrCodeCommandFailed   ErrorCode = "COMMAND_FAILED"

	// Git errors
	ErrCodeGitNotInstalled ErrorCode = "GIT_NOT_INSTALLED"
	ErrCodeGitCloneFailed  ErrorCode = "GIT_CLONE_FAILED"
	ErrCodeGitDirty        ErrorCode = "GIT_DIRTY"

	// General errors
	ErrCodeInternal         ErrorCode = "INTERNAL_ERROR"
	ErrCodeInvalidInput     ErrorCode = "INVALID_INPUT"
	ErrCodePermissionDenied ErrorCode = "PERMISSION_DENIED"
)

// GroveError represents a structured error with context
type GroveError struct {
	Code    ErrorCode              `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
	Cause   error                  `json:"-"`
}

// Error implements the error interface
func (e *GroveError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (caused by: %v)", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap implements the errors.Unwrap interface
func (e *GroveError) Unwrap() error {
	return e.Cause
}

// WithDetail adds a detail to the error
func (e *GroveError) WithDetail(key string, value interface{}) *GroveError {
	if e.Details == nil {
		e.Details = make(map[string]interface{})
	}
	e.Details[key] = value
	return e
}

// ToJSON converts the error to JSON
func (e *GroveError) ToJSON() string {
	data, _ := json.MarshalIndent(e, "", "  ")
	return string(data)
}

// New creates a new GroveError
func New(code ErrorCode, message string) *GroveError {
	return &GroveError{
		Code:    code,
		Message: message,
	}
}

// Wrap wraps an existing error with a GroveError
func Wrap(err error, code ErrorCode, message string) *GroveError {
	return &GroveError{
		Code:    code,
		Message: message,
		Cause:   err,
	}
}

// Is checks if an error is a specific GroveError code
func Is(err error, code ErrorCode) bool {
	if err == nil {
		return false
	}

	groveErr, ok := err.(*GroveError)
	if !ok {
		// Try to unwrap
		if unwrapper, ok := err.(interface{ Unwrap() error }); ok {
			return Is(unwrapper.Unwrap(), code)
		}
		return false
	}

	return groveErr.Code == code
}

// GetCode extracts the error code from an error
func GetCode(err error) ErrorCode {
	if err == nil {
		return ""
	}

	groveErr, ok := err.(*GroveError)
	if !ok {
		// Try to unwrap
		if unwrapper, ok := err.(interface{ Unwrap() error }); ok {
			return GetCode(unwrapper.Unwrap())
		}
		return ""
	}

	return groveErr.Code
}