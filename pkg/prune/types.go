// Package prune detects and removes orphaned grove environment resources —
// docker images/volumes, host worktree/volume directories, and cloud
// resources (Cloud Run, GCE, Artifact Registry, GCS state) that were
// created for a grove worktree but are no longer mapped to an active one.
//
// The package is exec-driven: callers inject a Runner for shelling out to
// docker/gcloud, which keeps the package testable with fixture-based
// stand-ins. Safety rails (dry-run default, cloud opt-in, active-slug
// bail) live in pruner.go.
package prune

// Category is the stable tag for a kind of resource the pruner knows how
// to detect. The strings are part of the JSON surface, keep them stable.
type Category string

const (
	CatDockerImage  Category = "docker.image"
	CatDockerVolume Category = "docker.volume"
	CatHostWorktree Category = "host.worktree_dir"
	CatHostVolume   Category = "host.volume_dir"
	CatCloudRun     Category = "cloud.cloud_run"
	CatCloudGCE     Category = "cloud.gce_instance"
	CatCloudAR      Category = "cloud.artifact_registry_tag"
	CatCloudGCS     Category = "cloud.gcs_state_prefix"
)

// IsCloud reports whether c represents a billable cloud resource; cloud
// categories require explicit --include=cloud opt-in before deletion.
func (c Category) IsCloud() bool {
	switch c {
	case CatCloudRun, CatCloudGCE, CatCloudAR, CatCloudGCS:
		return true
	}
	return false
}

// Orphan is a single resource that prune has flagged. Name is a
// category-specific identifier the corresponding delete path knows how
// to consume (docker image ref, volume name, absolute host path, gcloud
// resource URL, gs:// prefix). Worktree is the extracted slug when the
// name is recognizable; empty when the resource matched on a broader
// rule (rare).
type Orphan struct {
	Category Category          `json:"category"`
	Name     string            `json:"name"`
	Worktree string            `json:"worktree,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// FailedDelete carries one per-orphan delete error without losing the
// original orphan payload, so callers can retry/report without a second
// detection pass.
type FailedDelete struct {
	Orphan Orphan `json:"orphan"`
	Error  string `json:"error"`
}

// PruneResult is the top-level return shape. DryRun/IncludeCloud/ScopedTo
// mirror the CLI flags so JSON consumers can sanity-check the run.
type PruneResult struct {
	Orphans      []Orphan       `json:"orphans"`
	Deleted      []Orphan       `json:"deleted,omitempty"`
	Failed       []FailedDelete `json:"failed,omitempty"`
	DryRun       bool           `json:"dry_run"`
	IncludeCloud bool           `json:"include_cloud"`
	ScopedTo     string         `json:"scoped_to,omitempty"`
}

// CloudConfig carries the cloud-detection parameters resolved from
// grove.toml. Empty fields disable the corresponding detector (e.g. no
// Project disables every cloud category; no StateBucket disables GCS).
type CloudConfig struct {
	Project     string
	Region      string
	StateBucket string
	Ecosystem   string
	// ARRepos holds Artifact Registry repo paths in the canonical
	// "<region>-docker.pkg.dev/<project>/<repo>" form. One entry per
	// grove-owned AR repo from grove.toml image registries.
	ARRepos []string
}
