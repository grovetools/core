// Package envtf provides shared Terraform setup primitives used by both the
// daemon's `terraform` provider (for `grove env up`/`down`) and the grove CLI
// (for `grove env drift`). It encapsulates the grove-to-TF contract: the
// auto-injected `grove_*` variables, the GCS backend override, the init
// argument layout, and the shared-infrastructure output lookup.
package envtf

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	coreenv "github.com/grovetools/core/pkg/env"
	"github.com/grovetools/core/pkg/workspace"
)

// GroveContext is the set of standard variables injected into Terraform as
// auto.tfvars.json. Mirrors the fields the TF module reads via
// `variable "grove_*"` declarations.
type GroveContext struct {
	GroveEcosystem string                       `json:"grove_ecosystem"`
	GroveProject   string                       `json:"grove_project"`
	GroveWorktree  string                       `json:"grove_worktree"`
	GroveBranch    string                       `json:"grove_branch,omitempty"`
	GrovePlanDir   string                       `json:"grove_plan_dir"`
	GroveVolumes   map[string]GroveVolumeConfig `json:"grove_volumes,omitempty"`
	EnvName        string                       `json:"env_name"`
	GroveEnvId     int                          `json:"grove_env_id"`
}

// GroveVolumeConfig represents a volume's configuration passed to Terraform modules.
type GroveVolumeConfig struct {
	Service     string `json:"service"`
	HostPath    string `json:"host_path"`
	Persist     bool   `json:"persist,omitempty"`
	SnapshotURL string `json:"snapshot_url,omitempty"`
	RestoreCmd  string `json:"restore_command,omitempty"`
}

// BackendConfig holds resolved Terraform backend configuration.
type BackendConfig struct {
	Type   string // "gcs" or "local"
	Bucket string // GCS bucket name
	Prefix string // GCS state prefix
}

// TfOutput represents a single Terraform output value as emitted by
// `terraform output -json`.
type TfOutput struct {
	Value     interface{} `json:"value"`
	Type      interface{} `json:"type"`
	Sensitive bool        `json:"sensitive"`
}

// GenerateEnvId produces a deterministic environment ID (1-255) from a worktree name.
func GenerateEnvId(worktreeName string) int {
	h := fnv.New32a()
	h.Write([]byte(worktreeName))
	return int(h.Sum32()%255) + 1
}

// CurrentGitBranch returns the current git branch for the workspace, falling
// back to the workspace name if git metadata is unavailable.
func CurrentGitBranch(ws *workspace.WorkspaceNode) string {
	if ws == nil {
		return "default"
	}
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = ws.Path
	out, err := cmd.Output()
	if err == nil {
		branch := strings.TrimSpace(string(out))
		if branch != "" && branch != "HEAD" {
			return branch
		}
	}
	return ws.Name
}

// ResolveBackend determines the Terraform backend configuration from the
// request config. The GCS prefix follows the pattern
// `<ecosystem>/<project>/<ref>` where:
//   - project = first path component of config["path"] (e.g., "kitchen-infra")
//   - ref     = current git branch or worktree name
func ResolveBackend(req coreenv.EnvRequest) BackendConfig {
	stateBackend, _ := req.Config["state_backend"].(string)
	stateBucket, _ := req.Config["state_bucket"].(string)

	if stateBackend == "gcs" && stateBucket != "" {
		ecosystem := ""
		if req.Workspace != nil && req.Workspace.ParentEcosystemPath != "" {
			ecosystem = filepath.Base(req.Workspace.ParentEcosystemPath)
		}
		if ecosystem == "" && req.Workspace != nil && req.Workspace.RootEcosystemPath != "" {
			ecosystem = filepath.Base(req.Workspace.RootEcosystemPath)
		}

		project := "default"
		if configPath, ok := req.Config["path"].(string); ok && configPath != "" {
			parts := strings.SplitN(filepath.Clean(configPath), string(filepath.Separator), 2)
			if len(parts) > 0 && parts[0] != "." {
				project = parts[0]
			}
		}

		ref := ""
		if branch, ok := req.Config["branch"].(string); ok && branch != "" {
			ref = branch
		} else {
			ref = CurrentGitBranch(req.Workspace)
		}

		prefix := project + "/" + ref
		if ecosystem != "" {
			prefix = ecosystem + "/" + project + "/" + ref
		}
		return BackendConfig{Type: "gcs", Bucket: stateBucket, Prefix: prefix}
	}

	return BackendConfig{Type: "local"}
}

// BuildTfVarsPayload creates the combined variables map from grove context,
// user vars, shared outputs, and image URIs. `imageVars` and `sharedOutputs`
// may be nil.
func BuildTfVarsPayload(req coreenv.EnvRequest, imageVars map[string]string, sharedOutputs map[string]interface{}) (map[string]interface{}, error) {
	if req.Workspace == nil {
		return nil, fmt.Errorf("envtf: workspace required to build tfvars payload")
	}
	worktree := req.Workspace.Name
	stateDir := req.EffectiveStateDir()

	gctx := GroveContext{
		GroveProject:  req.Workspace.Name,
		GroveWorktree: worktree,
		GrovePlanDir:  stateDir,
		EnvName:       worktree,
		GroveEnvId:    GenerateEnvId(worktree),
	}
	if req.Workspace.ParentEcosystemPath != "" {
		gctx.GroveEcosystem = filepath.Base(req.Workspace.ParentEcosystemPath)
	}
	if branch, ok := req.Config["branch"].(string); ok {
		gctx.GroveBranch = branch
	}

	if services, ok := req.Config["services"].(map[string]interface{}); ok {
		for svcName, svcConfigRaw := range services {
			svcConfig, ok := svcConfigRaw.(map[string]interface{})
			if !ok {
				continue
			}
			volumes, ok := svcConfig["volumes"].(map[string]interface{})
			if !ok {
				continue
			}
			for volName, volCfgRaw := range volumes {
				volCfg, ok := volCfgRaw.(map[string]interface{})
				if !ok {
					continue
				}
				hostPath, _ := volCfg["host_path"].(string)
				persist, _ := volCfg["persist"].(bool)
				vc := GroveVolumeConfig{
					Service:  svcName,
					HostPath: hostPath,
					Persist:  persist,
				}
				if restoreCfg, ok := volCfg["restore"].(map[string]interface{}); ok {
					vc.RestoreCmd, _ = restoreCfg["command"].(string)
					vc.SnapshotURL, _ = restoreCfg["snapshot_url"].(string)
				}
				if gctx.GroveVolumes == nil {
					gctx.GroveVolumes = make(map[string]GroveVolumeConfig)
				}
				gctx.GroveVolumes[svcName+"/"+volName] = vc
			}
		}
	}

	gctxBytes, err := json.Marshal(gctx)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal grove context: %w", err)
	}
	payload := make(map[string]interface{})
	if err := json.Unmarshal(gctxBytes, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal grove context to map: %w", err)
	}

	if vars, ok := req.Config["vars"].(map[string]interface{}); ok {
		for k, v := range vars {
			payload[k] = v
		}
	}

	for k, v := range imageVars {
		payload[k] = v
	}

	if len(sharedOutputs) > 0 {
		payload["shared"] = sharedOutputs
	}

	return payload, nil
}

// WriteTfVars writes the combined variables payload to
// `<stateDir>/grove_context.auto.tfvars.json` and returns the path.
func WriteTfVars(stateDir string, payload map[string]interface{}) (string, error) {
	varsPath := filepath.Join(stateDir, "grove_context.auto.tfvars.json")
	varsBytes, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal tfvars: %w", err)
	}
	if err := os.WriteFile(varsPath, varsBytes, 0o644); err != nil { //nolint:gosec // terraform vars are not sensitive
		return "", fmt.Errorf("failed to write tfvars: %w", err)
	}
	return varsPath, nil
}

// WriteBackendOverride writes `_grove_backend_override.tf.json` to the module
// directory for GCS backend configuration. Returns the file path (for cleanup)
// or empty string for local backend.
func WriteBackendOverride(moduleDir string, bc BackendConfig) (string, error) {
	if bc.Type != "gcs" {
		return "", nil
	}
	overridePath := filepath.Join(moduleDir, "_grove_backend_override.tf.json")
	content := map[string]interface{}{
		"terraform": map[string]interface{}{
			"backend": map[string]interface{}{
				"gcs": map[string]interface{}{},
			},
		},
	}
	data, err := json.MarshalIndent(content, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal backend override: %w", err)
	}
	if err := os.WriteFile(overridePath, data, 0o644); err != nil { //nolint:gosec // terraform override is not sensitive
		return "", fmt.Errorf("failed to write backend override: %w", err)
	}
	return overridePath, nil
}

// BuildInitArgs returns the terraform init arguments based on backend config.
func BuildInitArgs(bc BackendConfig) []string {
	args := []string{"init", "-input=false", "-reconfigure"}
	if bc.Type == "gcs" {
		args = append(args,
			"-backend-config=bucket="+bc.Bucket,
			"-backend-config=prefix="+bc.Prefix,
		)
	}
	return args
}

// FetchSharedOutputs reads Terraform outputs from a shared infrastructure
// project's remote state. It creates a temporary directory, configures the
// backend, runs `init` + `output -json`, and returns the non-sensitive values.
func FetchSharedOutputs(ctx context.Context, sharedCfg map[string]interface{}) (map[string]interface{}, error) {
	bucket, _ := sharedCfg["state_bucket"].(string)
	prefix, _ := sharedCfg["state_prefix"].(string)
	backend, _ := sharedCfg["state_backend"].(string)

	if backend != "gcs" || bucket == "" || prefix == "" {
		return nil, fmt.Errorf("shared backend config requires state_backend=gcs, state_bucket, and state_prefix")
	}

	tmpDir, err := os.MkdirTemp("", "grove-shared-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir for shared outputs: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	overrideContent := map[string]interface{}{
		"terraform": map[string]interface{}{
			"backend": map[string]interface{}{
				"gcs": map[string]interface{}{},
			},
		},
	}
	overrideBytes, err := json.MarshalIndent(overrideContent, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal shared backend override: %w", err)
	}
	overridePath := filepath.Join(tmpDir, "_grove_backend_override.tf.json")
	if err := os.WriteFile(overridePath, overrideBytes, 0o644); err != nil { //nolint:gosec // terraform override is not sensitive
		return nil, fmt.Errorf("failed to write shared backend override: %w", err)
	}

	initArgs := []string{
		"init", "-input=false", "-reconfigure",
		"-backend-config=bucket=" + bucket,
		"-backend-config=prefix=" + prefix,
	}
	initCmd := exec.CommandContext(ctx, "terraform", initArgs...)
	initCmd.Dir = tmpDir
	if output, err := initCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("terraform init for shared outputs failed: %w\nOutput: %s", err, string(output))
	}

	outputCmd := exec.CommandContext(ctx, "terraform", "output", "-json")
	outputCmd.Dir = tmpDir
	outputBytes, err := outputCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("terraform output for shared infra failed: %w", err)
	}

	var rawOutputs map[string]TfOutput
	if err := json.Unmarshal(outputBytes, &rawOutputs); err != nil {
		return nil, fmt.Errorf("failed to parse shared terraform outputs: %w", err)
	}

	result := make(map[string]interface{})
	for name, out := range rawOutputs {
		if !out.Sensitive {
			result[name] = out.Value
		}
	}

	return result, nil
}
