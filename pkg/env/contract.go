package env

import (
	"github.com/grovetools/core/pkg/workspace"
)

// EnvRequest is the payload sent to providers (via daemon API or exec plugin stdin).
type EnvRequest struct {
	Action    string                   `json:"action"`              // "up", "down"
	Provider  string                   `json:"provider"`            // Provider name (e.g., native, docker, cloud)
	Workspace *workspace.WorkspaceNode `json:"workspace,omitempty"` // Full node context
	PlanDir   string                   `json:"plan_dir"`            // Notebook plan dir for ephemeral state
	Config    map[string]interface{}   `json:"config,omitempty"`    // Provider-specific config from grove.yml
}

// EnvResponse is the payload returned by providers (via daemon API or exec plugin stdout).
type EnvResponse struct {
	Status    string            `json:"status"`              // "running", "stopped", "failed"
	EnvVars   map[string]string `json:"env_vars,omitempty"`  // Written to .env.local in worktree
	Endpoints []string          `json:"endpoints,omitempty"` // Display to the user
	State     map[string]string `json:"state,omitempty"`     // Saved to .env_state.json in notebook
	Error     string            `json:"error,omitempty"`
}

// EnvStateFile represents the persistent state written to the notebook plan directory.
type EnvStateFile struct {
	Provider string            `json:"provider"`
	Command  string            `json:"command,omitempty"` // Binary path for exec plugins (empty = search PATH)
	State    map[string]string `json:"state"`
}
