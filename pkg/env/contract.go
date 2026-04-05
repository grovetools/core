package env

import (
	"github.com/grovetools/core/pkg/workspace"
)

// EnvRequest is the payload sent to providers (via daemon API or exec plugin stdin).
type EnvRequest struct {
	Action    string                   `json:"action"`              // "up", "down", "status", "restart"
	Provider  string                   `json:"provider"`            // Provider name (e.g., native, docker, cloud)
	Workspace *workspace.WorkspaceNode `json:"workspace,omitempty"` // Full node context
	PlanDir   string                   `json:"plan_dir"`            // Notebook plan dir for ephemeral state (legacy, prefer StateDir)
	StateDir  string                   `json:"state_dir"`           // Directory for persistent env state (.grove/env/)
	Config    map[string]interface{}   `json:"config,omitempty"`    // Provider-specific config from grove.yml
	ManagedBy string                   `json:"managed_by,omitempty"` // Who owns this env: "plan:<slug>", "user", or empty
	Force     bool                     `json:"force,omitempty"`     // Force teardown even if not the owner
}

// EnvResponse is the payload returned by providers (via daemon API or exec plugin stdout).
type EnvResponse struct {
	Status    string            `json:"status"`              // "running", "stopped", "failed"
	EnvVars   map[string]string `json:"env_vars,omitempty"`  // Written to .env.local in worktree
	Endpoints []string          `json:"endpoints,omitempty"` // Display to the user
	State     map[string]string `json:"state,omitempty"`     // Saved to state.json
	Error     string            `json:"error,omitempty"`
}

// ServiceState tracks the runtime state of an individual service within an environment.
type ServiceState struct {
	Name   string `json:"name"`
	Port   int    `json:"port,omitempty"`
	Status string `json:"status"` // "running", "stopped", "error"
}

// EnvStateFile represents the persistent state written to .grove/env/state.json.
type EnvStateFile struct {
	Provider        string            `json:"provider"`
	Command         string            `json:"command,omitempty"`          // Binary path for exec plugins (empty = search PATH)
	Environment     string            `json:"environment,omitempty"`      // Named environment profile used for this plan
	ManagedBy       string            `json:"managed_by,omitempty"`       // "plan:<slug>", "user", or empty
	Ports           map[string]int    `json:"ports,omitempty"`            // Service name -> allocated port
	Services        []ServiceState    `json:"services,omitempty"`         // Per-service runtime state
	ServiceCommands map[string]string `json:"service_commands,omitempty"` // Service name -> shell command (for native restart)
	EnvVars         map[string]string `json:"env_vars,omitempty"`         // Env vars produced by the provider
	State           map[string]string `json:"state"`                      // Opaque provider state
}

// EffectiveStateDir returns the state directory to use, preferring StateDir over PlanDir.
func (r *EnvRequest) EffectiveStateDir() string {
	if r.StateDir != "" {
		return r.StateDir
	}
	return r.PlanDir
}
