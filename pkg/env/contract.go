package env

import (
	"github.com/grovetools/core/pkg/workspace"
)

// EnvRequest is the payload sent to providers (via daemon API or exec plugin stdin).
type EnvRequest struct {
	Action    string                   `json:"action"`               // "up", "down", "status", "restart"
	Provider  string                   `json:"provider"`             // Provider name (e.g., native, docker, cloud)
	Profile   string                   `json:"profile,omitempty"`    // Named environment profile (empty = default)
	Workspace *workspace.WorkspaceNode `json:"workspace,omitempty"`  // Full node context
	PlanDir   string                   `json:"plan_dir"`             // Notebook plan dir for ephemeral state (legacy, prefer StateDir)
	StateDir  string                   `json:"state_dir"`            // Directory for persistent env state (.grove/env/)
	Config    map[string]interface{}   `json:"config,omitempty"`     // Provider-specific config from grove.yml
	ManagedBy string                   `json:"managed_by,omitempty"` // Who owns this env: "plan:<slug>", "user", or empty
	Force     bool                     `json:"force,omitempty"`      // Force teardown even if not the owner
	Clean     bool                     `json:"clean,omitempty"`      // Remove all volumes including persistent ones
	// ForceDestroy instructs the terraform provider on Down to bypass the
	// `skip_destroy` profile flag and actually run `terraform destroy`.
	// Intended for plan-finish at worktree retirement, where preserving
	// cloud resources across iteration no longer applies.
	ForceDestroy bool     `json:"force_destroy,omitempty"`
	Rebuild      []string `json:"rebuild,omitempty"` // Image services to force-rebuild. "all" = every image; named entries match service keys.
}

// ForceRebuild reports whether the service with the given name should be
// force-rebuilt based on r.Rebuild. nil/empty means no force.
func (r *EnvRequest) ForceRebuild(svc string) bool {
	for _, s := range r.Rebuild {
		if s == "all" || s == svc {
			return true
		}
	}
	return false
}

// VolumeState tracks a volume's path and persistence setting for teardown.
type VolumeState struct {
	Path          string `json:"path"`                     // Host path (relative to workspace root)
	Persist       bool   `json:"persist,omitempty"`        // If true, survives env down (but not --clean)
	ContainerPath string `json:"container_path,omitempty"` // Mount target inside container (docker provider)
}

// EnvResponse is the payload returned by providers (via daemon API or exec plugin stdout).
type EnvResponse struct {
	Status       string            `json:"status"`                  // "running", "stopped", "failed"
	EnvVars      map[string]string `json:"env_vars,omitempty"`      // Written to .env.local in worktree
	Endpoints    []string          `json:"endpoints,omitempty"`     // Display to the user
	State        map[string]string `json:"state,omitempty"`         // Saved to state.json
	CleanupPaths []string          `json:"cleanup_paths,omitempty"` // Deprecated: use Volumes instead
	Volumes      []VolumeState     `json:"volumes,omitempty"`       // Volume state for teardown
	ProxyRoutes  map[string]int    `json:"proxy_routes,omitempty"`  // Route name -> host port, for global-daemon proxy registration
	Error        string            `json:"error,omitempty"`
}

// ServiceState tracks the runtime state of an individual service within an environment.
type ServiceState struct {
	Name   string `json:"name"`
	Port   int    `json:"port,omitempty"`
	Status string `json:"status"` // "running", "stopped", "error"
}

// EnvStateFile represents the persistent state written to .grove/env/state.json.
type EnvStateFile struct {
	Provider         string            `json:"provider"`
	Command          string            `json:"command,omitempty"`           // Binary path for exec plugins (empty = search PATH)
	Environment      string            `json:"environment,omitempty"`       // Named environment profile used for this plan
	ManagedBy        string            `json:"managed_by,omitempty"`        // "plan:<slug>", "user", or empty
	WorkspaceName    string            `json:"workspace_name,omitempty"`    // workspace.WorkspaceNode.Name at Up time
	WorkspacePath    string            `json:"workspace_path,omitempty"`    // workspace.WorkspaceNode.Path at Up time
	Ports            map[string]int    `json:"ports,omitempty"`             // Service name -> allocated port
	Services         []ServiceState    `json:"services,omitempty"`          // Per-service runtime state
	ServiceCommands  map[string]string `json:"service_commands,omitempty"`  // Service name -> shell command (for native restart)
	NativePGIDs      map[string]int    `json:"native_pgids,omitempty"`      // Service/tunnel name -> process group id (for cross-restart teardown)
	DockerContainers map[string]string `json:"docker_containers,omitempty"` // Service name -> container name (for native-docker services)
	LastProfile      string            `json:"last_profile,omitempty"`      // Profile name from the most recent Up; preserved in a sidecar after Down so `grove env cmd <name>` can fall back to the last-used profile instead of silently resolving to `default`.
	EnvVars          map[string]string `json:"env_vars,omitempty"`          // Env vars produced by the provider
	Endpoints        []string          `json:"endpoints,omitempty"`         // URLs the provider surfaced to users
	CleanupPaths     []string          `json:"cleanup_paths,omitempty"`     // Deprecated: use Volumes instead
	Volumes          []VolumeState     `json:"volumes,omitempty"`           // Volume state for teardown
	ProxyRoutes      map[string]int    `json:"proxy_routes,omitempty"`      // Route name -> host port; global daemon rebuilds proxy table from this on restart
	State            map[string]string `json:"state"`                       // Opaque provider state
}

// EffectiveVolumes returns the volumes to consider for teardown, migrating
// legacy CleanupPaths entries as non-persistent volumes when no Volumes are set.
func (s *EnvStateFile) EffectiveVolumes() []VolumeState {
	if len(s.Volumes) > 0 {
		return s.Volumes
	}
	// Backward compat: treat old cleanup_paths as non-persistent volumes
	vols := make([]VolumeState, 0, len(s.CleanupPaths))
	for _, p := range s.CleanupPaths {
		vols = append(vols, VolumeState{Path: p, Persist: false})
	}
	return vols
}

// EffectiveStateDir returns the state directory to use, preferring StateDir over PlanDir.
func (r *EnvRequest) EffectiveStateDir() string {
	if r.StateDir != "" {
		return r.StateDir
	}
	return r.PlanDir
}

// ProxyRouteRequest registers a single host-based route with the global
// daemon's proxy. Sent by scoped daemons over RPC whenever an env brings
// up a service that declared `route = ...` in grove.toml.
type ProxyRouteRequest struct {
	Worktree string `json:"worktree"`
	Route    string `json:"route"`
	Port     int    `json:"port"`
}

// ProxyUnregisterRequest drops every route keyed by Worktree.
type ProxyUnregisterRequest struct {
	Worktree string `json:"worktree"`
}
