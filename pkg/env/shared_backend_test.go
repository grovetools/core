package env

import (
	"reflect"
	"testing"

	"github.com/grovetools/core/pkg/workspace"
)

func TestBuildSharedBackendConfig_NoSharedEnv(t *testing.T) {
	got := BuildSharedBackendConfig(map[string]interface{}{
		"state_backend": "gcs",
		"state_bucket":  "b",
	}, nil)
	if got != nil {
		t.Fatalf("expected nil for missing shared_env, got %v", got)
	}
}

func TestBuildSharedBackendConfig_DefaultsSharedRefToMain(t *testing.T) {
	ws := &workspace.WorkspaceNode{ParentEcosystemPath: "/tmp/kitchen-env"}
	cfg := map[string]interface{}{
		"shared_env":    "kitchen-infra",
		"state_backend": "gcs",
		"state_bucket":  "kitchen-env-grove-state",
	}
	got := BuildSharedBackendConfig(cfg, ws)
	want := map[string]interface{}{
		"state_backend": "gcs",
		"state_bucket":  "kitchen-env-grove-state",
		"state_prefix":  "kitchen-env/kitchen-infra/main",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestBuildSharedBackendConfig_ExplicitSharedRef(t *testing.T) {
	ws := &workspace.WorkspaceNode{ParentEcosystemPath: "/tmp/kitchen-env"}
	cfg := map[string]interface{}{
		"shared_env":    "kitchen-infra",
		"shared_ref":    "tier1-a",
		"state_backend": "gcs",
		"state_bucket":  "kitchen-env-grove-state",
	}
	got := BuildSharedBackendConfig(cfg, ws)
	if got["state_prefix"] != "kitchen-env/kitchen-infra/tier1-a" {
		t.Fatalf("unexpected prefix: %v", got["state_prefix"])
	}
}

func TestBuildSharedBackendConfig_NonGCSBackend(t *testing.T) {
	cfg := map[string]interface{}{
		"shared_env":    "kitchen-infra",
		"state_backend": "local",
		"state_bucket":  "",
	}
	if got := BuildSharedBackendConfig(cfg, nil); got != nil {
		t.Fatalf("expected nil when state_backend != gcs, got %v", got)
	}
}

func TestBuildSharedBackendConfig_MissingBucket(t *testing.T) {
	cfg := map[string]interface{}{
		"shared_env":    "kitchen-infra",
		"state_backend": "gcs",
	}
	if got := BuildSharedBackendConfig(cfg, nil); got != nil {
		t.Fatalf("expected nil when state_bucket missing, got %v", got)
	}
}

func TestBuildSharedBackendConfig_FallsBackToRootEcosystem(t *testing.T) {
	ws := &workspace.WorkspaceNode{RootEcosystemPath: "/tmp/kitchen-env"}
	cfg := map[string]interface{}{
		"shared_env":    "kitchen-infra",
		"state_backend": "gcs",
		"state_bucket":  "b",
	}
	got := BuildSharedBackendConfig(cfg, ws)
	if got["state_prefix"] != "kitchen-env/kitchen-infra/main" {
		t.Fatalf("unexpected prefix: %v", got["state_prefix"])
	}
}

func TestBuildSharedBackendConfig_NilWorkspace(t *testing.T) {
	cfg := map[string]interface{}{
		"shared_env":    "kitchen-infra",
		"state_backend": "gcs",
		"state_bucket":  "b",
	}
	got := BuildSharedBackendConfig(cfg, nil)
	if got["state_prefix"] != "kitchen-infra/main" {
		t.Fatalf("unexpected prefix with nil workspace: %v", got["state_prefix"])
	}
}

func TestApplySharedBackendConfig_PopulatesReqConfig(t *testing.T) {
	req := &EnvRequest{
		Config: map[string]interface{}{
			"shared_env":    "kitchen-infra",
			"state_backend": "gcs",
			"state_bucket":  "b",
		},
		Workspace: &workspace.WorkspaceNode{ParentEcosystemPath: "/tmp/kitchen-env"},
	}
	ApplySharedBackendConfig(req)
	shared, ok := req.Config["shared_backend_config"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected shared_backend_config to be set")
	}
	if shared["state_prefix"] != "kitchen-env/kitchen-infra/main" {
		t.Fatalf("unexpected prefix: %v", shared["state_prefix"])
	}
}

func TestApplySharedBackendConfig_NoopWhenAbsent(t *testing.T) {
	req := &EnvRequest{Config: map[string]interface{}{"state_backend": "gcs", "state_bucket": "b"}}
	ApplySharedBackendConfig(req)
	if _, ok := req.Config["shared_backend_config"]; ok {
		t.Fatalf("shared_backend_config should not be set without shared_env")
	}
}
