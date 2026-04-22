package env

import (
	"path/filepath"

	"github.com/grovetools/core/pkg/workspace"
)

// BuildSharedBackendConfig resolves the shared_env profile reference declared
// in a terraform environment profile's config and returns the
// shared_backend_config payload the daemon uses to fetch var.shared from the
// referenced profile's tfstate. Returns nil if the profile does not declare
// shared_env, or if state_backend/state_bucket are unset (the daemon only
// supports the gcs backend today).
//
// Inputs mirror what both grove/cmd/env.go and flow/cmd/plan_init.go have in
// hand at provisioning time: the resolved profile config map and the
// workspace node for the active worktree (used to derive the ecosystem name
// for the GCS prefix).
func BuildSharedBackendConfig(profileConfig map[string]interface{}, ws *workspace.WorkspaceNode) map[string]interface{} {
	sharedEnv, _ := profileConfig["shared_env"].(string)
	if sharedEnv == "" {
		return nil
	}
	sharedRef, _ := profileConfig["shared_ref"].(string)
	if sharedRef == "" {
		sharedRef = "main"
	}
	stateBackend, _ := profileConfig["state_backend"].(string)
	stateBucket, _ := profileConfig["state_bucket"].(string)
	if stateBackend != "gcs" || stateBucket == "" {
		return nil
	}

	ecosystem := ""
	if ws != nil {
		if ws.ParentEcosystemPath != "" {
			ecosystem = filepath.Base(ws.ParentEcosystemPath)
		} else if ws.RootEcosystemPath != "" {
			ecosystem = filepath.Base(ws.RootEcosystemPath)
		}
	}

	prefix := sharedEnv + "/" + sharedRef
	if ecosystem != "" {
		prefix = ecosystem + "/" + sharedEnv + "/" + sharedRef
	}

	return map[string]interface{}{
		"state_backend": stateBackend,
		"state_bucket":  stateBucket,
		"state_prefix":  prefix,
	}
}

// ApplySharedBackendConfig populates req.Config["shared_backend_config"]
// in-place when the resolved profile declares a shared_env. No-op when
// BuildSharedBackendConfig returns nil.
func ApplySharedBackendConfig(req *EnvRequest) {
	if req == nil {
		return
	}
	shared := BuildSharedBackendConfig(req.Config, req.Workspace)
	if shared == nil {
		return
	}
	if req.Config == nil {
		req.Config = make(map[string]interface{})
	}
	req.Config["shared_backend_config"] = shared
}
