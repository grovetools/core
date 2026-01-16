package starship

import (
	"github.com/grovetools/core/state"
)

// StatusProvider generates a status string for a tool based on the current state.
// Providers should return an empty string if they have nothing to display.
type StatusProvider func(s state.State) (string, error)

// providers holds all registered status providers.
var providers []StatusProvider

// RegisterProvider registers a status provider to be called by the status command.
// This allows Grove tools to add their status generation logic to the starship prompt.
func RegisterProvider(p StatusProvider) {
	providers = append(providers, p)
}

// GetProviders returns all registered status providers.
// This is primarily used for testing.
func GetProviders() []StatusProvider {
	return providers
}

// ClearProviders removes all registered providers.
// This is primarily used for testing.
func ClearProviders() {
	providers = nil
}
