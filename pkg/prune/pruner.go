package prune

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Options carries prune runtime flags. Mirrors the CLI surface so
// programmatic callers (flow plan_finish, daemon RPC) don't have to
// reach into cobra.
type Options struct {
	DryRun       bool
	IncludeCloud bool
	Worktree     string // scope to a single slug when non-empty
}

// Inputs bundles the discovery-phase results the pruner needs: which
// slugs are active, which slugs are known-inactive (dictionary), the
// git root for host detection, and the cloud config for gcloud shell-
// outs. Zero-valued fields disable the relevant detectors.
type Inputs struct {
	GitRoot       string
	Active        []string
	Inactive      []string
	Cloud         CloudConfig
	DockerRunner  Runner
	GcloudRunner  Runner
}

// Detect runs every enabled detector and returns a flat orphan slice
// honoring Options.Worktree / IncludeCloud filtering. Detection errors
// for one category are non-fatal — logged to stderr so partial results
// still surface — except for the active-slug bail described on Run.
func Detect(in Inputs, opts Options) ([]Orphan, error) {
	if len(in.Active) == 0 {
		// Safety rail: refuse to prune when the active list is
		// empty. An empty list means discovery failed or this
		// isn't an ecosystem — either way, flagging everything
		// as orphan would be catastrophic.
		return nil, fmt.Errorf("prune: active worktree list is empty; refusing to run (bailing out for safety)")
	}
	// When --worktree scopes to a specific slug, treat it as inactive
	// even if it still appears in the active set — that's the whole
	// point of the flag: "this worktree is definitively dead, clean up
	// its resources." Without this demotion, the caller would see an
	// empty result because every detector skips active slugs first.
	activeList := in.Active
	inactiveList := in.Inactive
	if opts.Worktree != "" {
		filteredActive := activeList[:0:0]
		for _, s := range in.Active {
			if s == opts.Worktree {
				continue
			}
			filteredActive = append(filteredActive, s)
		}
		activeList = filteredActive
		if !containsString(inactiveList, opts.Worktree) {
			inactiveList = append(append([]string{}, inactiveList...), opts.Worktree)
		}
	}
	idx := NewSlugIndex(activeList, inactiveList)

	var all []Orphan
	collect := func(orphans []Orphan, err error) {
		if err != nil {
			fmt.Fprintf(os.Stderr, "prune: detect warning: %v\n", err)
			return
		}
		all = append(all, orphans...)
	}

	if in.DockerRunner != nil {
		collect(DetectDockerImages(in.DockerRunner, idx))
		collect(DetectDockerVolumes(in.DockerRunner, idx))
	}
	collect(DetectHostWorktreeDirs(in.GitRoot, idx))
	collect(DetectHostVolumeDirs(in.GitRoot, idx))
	if in.GcloudRunner != nil && opts.IncludeCloud {
		collect(DetectCloudRun(in.GcloudRunner, in.Cloud, idx))
		collect(DetectCloudGCE(in.GcloudRunner, in.Cloud, idx))
		collect(DetectCloudAR(in.GcloudRunner, in.Cloud, idx))
		collect(DetectCloudGCS(in.GcloudRunner, in.Cloud, idx))
	}

	if opts.Worktree != "" {
		filtered := all[:0]
		for _, o := range all {
			if o.Worktree == opts.Worktree {
				filtered = append(filtered, o)
			}
		}
		all = filtered
	}
	return all, nil
}

// Run detects orphans and, unless DryRun is set, deletes them. Cloud
// categories are only deleted when IncludeCloud is true (the CLI gates
// this behind an explicit --include=cloud flag; callers that forward
// it must gate accordingly). Local categories delete under --yes alone.
func Run(in Inputs, opts Options) (*PruneResult, error) {
	orphans, err := Detect(in, opts)
	if err != nil {
		return nil, err
	}
	result := &PruneResult{
		Orphans:      orphans,
		DryRun:       opts.DryRun,
		IncludeCloud: opts.IncludeCloud,
		ScopedTo:     opts.Worktree,
	}
	if opts.DryRun {
		return result, nil
	}
	for _, o := range orphans {
		if o.Category.IsCloud() && !opts.IncludeCloud {
			continue
		}
		if err := deleteOrphan(in, o); err != nil {
			result.Failed = append(result.Failed, FailedDelete{Orphan: o, Error: err.Error()})
			continue
		}
		result.Deleted = append(result.Deleted, o)
	}
	return result, nil
}

// deleteOrphan dispatches by Category. Deletion is best-effort: errors
// propagate back as FailedDelete entries without aborting the run.
func deleteOrphan(in Inputs, o Orphan) error {
	switch o.Category {
	case CatDockerImage:
		if in.DockerRunner == nil {
			return fmt.Errorf("docker runner not configured")
		}
		_, err := in.DockerRunner.Run("docker", "rmi", "-f", o.Name)
		return err
	case CatDockerVolume:
		if in.DockerRunner == nil {
			return fmt.Errorf("docker runner not configured")
		}
		_, err := in.DockerRunner.Run("docker", "volume", "rm", "-f", o.Name)
		return err
	case CatHostWorktree, CatHostVolume:
		return removeHostPath(o.Name, in.GitRoot)
	case CatCloudRun:
		_, err := in.GcloudRunner.Run("gcloud", "run", "services", "delete", o.Name,
			"--project="+in.Cloud.Project, "--region="+in.Cloud.Region, "--quiet")
		return err
	case CatCloudGCE:
		zone := ""
		if o.Metadata != nil {
			zone = o.Metadata["zone"]
		}
		args := []string{"compute", "instances", "delete", o.Name, "--project=" + in.Cloud.Project, "--quiet"}
		if zone != "" {
			args = append(args, "--zone="+zone)
		}
		_, err := in.GcloudRunner.Run("gcloud", args...)
		return err
	case CatCloudAR:
		_, err := in.GcloudRunner.Run("gcloud", "artifacts", "docker", "images", "delete", o.Name, "--delete-tags", "--quiet")
		return err
	case CatCloudGCS:
		_, err := in.GcloudRunner.Run("gcloud", "storage", "rm", "-r", o.Name)
		return err
	}
	return fmt.Errorf("unknown category %s", o.Category)
}

// removeHostPath deletes path after confirming it lives strictly under
// gitRoot — guards against ever descending outside the ecosystem.
func removeHostPath(path, gitRoot string) error {
	if gitRoot == "" {
		return fmt.Errorf("gitRoot empty; refusing to remove %s", path)
	}
	clean := filepath.Clean(path)
	rootClean := filepath.Clean(gitRoot) + string(filepath.Separator)
	if !strings.HasPrefix(clean, rootClean) {
		return fmt.Errorf("refusing to remove %s: outside git root %s", path, gitRoot)
	}
	return os.RemoveAll(clean)
}

func containsString(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

// ExecRunner is the default Runner: shells out to a real binary via
// os/exec and returns combined stdout. Stderr is dropped when the
// command succeeds so chatter (e.g. `gcloud` update notices) doesn't
// pollute JSON parsing; on error, stderr is folded into err for the
// caller.
type ExecRunner struct{}

func (ExecRunner) Run(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}
