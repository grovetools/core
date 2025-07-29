package cli

import (
	"fmt"
	"os"

	"github.com/mattsolo1/grove-core/errors"
)

// ErrorHandler provides user-friendly error messages
type ErrorHandler struct {
	Verbose bool
}

// NewErrorHandler creates a new error handler
func NewErrorHandler(verbose bool) *ErrorHandler {
	return &ErrorHandler{
		Verbose: verbose,
	}
}

// Handle provides user-friendly error messages based on error type
func (h *ErrorHandler) Handle(err error) error {
	// Check for specific error codes
	switch errors.GetCode(err) {
	case errors.ErrCodeConfigNotFound:
		fmt.Fprintf(os.Stderr, "❌ Configuration not found. Run 'grove init' to create a new configuration.\n")
		return err

	case errors.ErrCodeServiceNotFound:
		if groveErr, ok := err.(*errors.GroveError); ok {
			fmt.Fprintf(os.Stderr, "❌ Service '%s' not found in grove.yml\n", groveErr.Details["service"])
			fmt.Fprintf(os.Stderr, "Run 'grove services' to see available services.\n")
		}
		return err

	case errors.ErrCodePortConflict:
		if groveErr, ok := err.(*errors.GroveError); ok {
			fmt.Fprintf(os.Stderr, "❌ Port %d is already in use\n", groveErr.Details["port"])
			fmt.Fprintf(os.Stderr, "Stop the conflicting service or change the port in grove.yml\n")
		}
		return err

	case errors.ErrCodeCommandNotFound:
		fmt.Fprintf(os.Stderr, "❌ Required command not found. Make sure Docker and docker-compose are installed.\n")
		return err

	case errors.ErrCodeContainerTimeout:
		if groveErr, ok := err.(*errors.GroveError); ok {
			fmt.Fprintf(os.Stderr, "❌ Service '%s' failed to start within %s\n",
				groveErr.Details["service"], groveErr.Details["timeout"])
			fmt.Fprintf(os.Stderr, "Check the service logs with 'grove logs %s'\n", groveErr.Details["service"])
		}
		return err

	default:
		// Generic error handling
		fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)

		// If verbose mode, show full error details
		if h.Verbose {
			if groveErr, ok := err.(*errors.GroveError); ok {
				fmt.Fprintf(os.Stderr, "\nError details:\n%s\n", groveErr.ToJSON())
			}
		}
		return err
	}
}