package prune

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Runner is the exec abstraction used by detection and deletion paths.
// Each method returns combined stdout + error; fixtures implement this
// directly in tests. Keeping the surface this narrow avoids dragging
// the flow exec package (which doesn't expose output) into core.
type Runner interface {
	Run(name string, args ...string) ([]byte, error)
}

// SlugIndex holds the membership sets the detectors consult. Active are
// the slugs we must never flag; Known is the full universe (active +
// inactive) used as a dictionary to extract slugs from opaque resource
// names. When Known omits a slug, resources for that slug fall through
// as "unrecognized" and are skipped as shared infra.
type SlugIndex struct {
	Active map[string]struct{}
	Known  []string
}

// NewSlugIndex builds a SlugIndex with Known sorted longest-first so
// lookup prefers the most specific match (e.g. "tier1-tf-rerun" over
// "tier1"). Duplicates between active and inactive are harmless.
func NewSlugIndex(active, inactive []string) SlugIndex {
	idx := SlugIndex{Active: make(map[string]struct{}, len(active))}
	seen := make(map[string]struct{}, len(active)+len(inactive))
	add := func(s string) {
		if s == "" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		idx.Known = append(idx.Known, s)
	}
	for _, s := range active {
		idx.Active[s] = struct{}{}
		add(s)
	}
	for _, s := range inactive {
		add(s)
	}
	sort.Slice(idx.Known, func(i, j int) bool {
		if len(idx.Known[i]) != len(idx.Known[j]) {
			return len(idx.Known[i]) > len(idx.Known[j])
		}
		return idx.Known[i] < idx.Known[j]
	})
	return idx
}

// ExtractSlug returns the known slug the resource belongs to, or "" if
// name does not look like a per-worktree grove resource. Match order:
// strip optional "grove-" prefix, then scan idx.Known (longest-first)
// for a prefix match followed by "-" or end-of-string. This deliberately
// ignores unknown slugs so shared-infra names (kitchen-env-vpc,
// kitchen-vpc-conn, kitchen-env-cloudbuild) are skipped.
func ExtractSlug(name string, idx SlugIndex) string {
	core := strings.TrimPrefix(name, "grove-")
	for _, slug := range idx.Known {
		if core == slug {
			return slug
		}
		if strings.HasPrefix(core, slug+"-") {
			return slug
		}
	}
	return ""
}

// IsActiveSlug reports whether slug is in the active set.
func IsActiveSlug(slug string, idx SlugIndex) bool {
	if slug == "" {
		return false
	}
	_, ok := idx.Active[slug]
	return ok
}

// DetectDockerImages shells out to `docker images` and returns orphan
// entries for every image whose extracted slug is not active. Images
// that don't map to a known slug (i.e. they match the grove- prefix
// but not any known worktree) are still flagged as orphans with the
// slug field left blank, so one-off `grove-*` leaks don't hide.
func DetectDockerImages(runner Runner, idx SlugIndex) ([]Orphan, error) {
	out, err := runner.Run("docker", "images", "--format", "{{.Repository}}:{{.Tag}}", "--filter", "reference=grove-*")
	if err != nil {
		return nil, fmt.Errorf("docker images: %w", err)
	}
	var orphans []Orphan
	for _, line := range splitNonEmpty(string(out)) {
		// Skip dangling <none>:<none> entries — they're garbage
		// that `docker image prune` already handles and don't
		// carry a worktree slug to match against.
		if strings.Contains(line, "<none>") {
			continue
		}
		name := strings.TrimSpace(line)
		slug := ExtractSlug(repoPart(name), idx)
		if slug == "" || IsActiveSlug(slug, idx) {
			continue
		}
		orphans = append(orphans, Orphan{Category: CatDockerImage, Name: name, Worktree: slug})
	}
	return orphans, nil
}

// DetectDockerVolumes mirrors DetectDockerImages for `docker volume ls`.
func DetectDockerVolumes(runner Runner, idx SlugIndex) ([]Orphan, error) {
	out, err := runner.Run("docker", "volume", "ls", "--format", "{{.Name}}", "--filter", "name=grove-*")
	if err != nil {
		return nil, fmt.Errorf("docker volume ls: %w", err)
	}
	var orphans []Orphan
	for _, line := range splitNonEmpty(string(out)) {
		name := strings.TrimSpace(line)
		slug := ExtractSlug(name, idx)
		if slug == "" || IsActiveSlug(slug, idx) {
			continue
		}
		orphans = append(orphans, Orphan{Category: CatDockerVolume, Name: name, Worktree: slug})
	}
	return orphans, nil
}

// DetectHostWorktreeDirs lists <gitRoot>/.grove-worktrees/ and reports
// every entry whose name is not in the active set. This is the
// filesystem analogue of "what worktrees does the ecosystem know
// about". The caller is responsible for gating this behind --force /
// known-dead checks — a stale directory might still hold uncommitted
// work. Prune reports, `flow plan finish` owns actual removal.
func DetectHostWorktreeDirs(gitRoot string, idx SlugIndex) ([]Orphan, error) {
	if gitRoot == "" {
		return nil, nil
	}
	base := filepath.Join(gitRoot, ".grove-worktrees")
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var orphans []Orphan
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if _, ok := idx.Active[name]; ok {
			continue
		}
		orphans = append(orphans, Orphan{
			Category: CatHostWorktree,
			Name:     filepath.Join(base, name),
			Worktree: name,
		})
	}
	return orphans, nil
}

// DetectHostVolumeDirs walks <gitRoot>/.grove/volumes/ the same way.
// These carry docker bind-mount payloads (clickhouse data etc.) owned
// by a specific worktree's plan lifecycle.
func DetectHostVolumeDirs(gitRoot string, idx SlugIndex) ([]Orphan, error) {
	if gitRoot == "" {
		return nil, nil
	}
	base := filepath.Join(gitRoot, ".grove", "volumes")
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var orphans []Orphan
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		slug := ExtractSlug(name, idx)
		if IsActiveSlug(slug, idx) {
			continue
		}
		// When the dir name doesn't match any known slug, skip
		// rather than flag — shared volumes (e.g. a plain
		// `clickhouse/` dir from a native profile) don't belong
		// to any worktree and must survive prune.
		if slug == "" && !looksLikeWorktreeDir(name, idx) {
			continue
		}
		orphans = append(orphans, Orphan{
			Category: CatHostVolume,
			Name:     filepath.Join(base, name),
			Worktree: slug,
		})
	}
	return orphans, nil
}

// looksLikeWorktreeDir reports whether dir exactly matches an inactive
// known slug. Used by DetectHostVolumeDirs to distinguish per-worktree
// dirs from shared service dirs when ExtractSlug comes up empty.
func looksLikeWorktreeDir(dir string, idx SlugIndex) bool {
	for _, s := range idx.Known {
		if dir == s {
			return true
		}
	}
	return false
}

// DetectCloudRun queries `gcloud run services list` for cfg.Project in
// cfg.Region and returns services whose extracted slug is not active.
// Returns nil if cfg.Project is empty (cloud detection disabled).
func DetectCloudRun(runner Runner, cfg CloudConfig, idx SlugIndex) ([]Orphan, error) {
	if cfg.Project == "" {
		return nil, nil
	}
	args := []string{"run", "services", "list", "--format=json", "--project=" + cfg.Project}
	if cfg.Region != "" {
		args = append(args, "--region="+cfg.Region)
	}
	out, err := runner.Run("gcloud", args...)
	if err != nil {
		return nil, fmt.Errorf("gcloud run services list: %w", err)
	}
	var services []struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(out, &services); err != nil {
		return nil, fmt.Errorf("parse gcloud run output: %w", err)
	}
	var orphans []Orphan
	for _, svc := range services {
		name := svc.Metadata.Name
		slug := ExtractSlug(name, idx)
		if slug == "" || IsActiveSlug(slug, idx) {
			continue
		}
		orphans = append(orphans, Orphan{Category: CatCloudRun, Name: name, Worktree: slug})
	}
	return orphans, nil
}

// DetectCloudGCE mirrors DetectCloudRun for Compute Engine instances.
func DetectCloudGCE(runner Runner, cfg CloudConfig, idx SlugIndex) ([]Orphan, error) {
	if cfg.Project == "" {
		return nil, nil
	}
	out, err := runner.Run("gcloud", "compute", "instances", "list", "--format=json", "--project="+cfg.Project)
	if err != nil {
		return nil, fmt.Errorf("gcloud compute instances list: %w", err)
	}
	var instances []struct {
		Name string `json:"name"`
		Zone string `json:"zone"`
	}
	if err := json.Unmarshal(out, &instances); err != nil {
		return nil, fmt.Errorf("parse gcloud compute output: %w", err)
	}
	var orphans []Orphan
	for _, inst := range instances {
		slug := ExtractSlug(inst.Name, idx)
		if slug == "" || IsActiveSlug(slug, idx) {
			continue
		}
		o := Orphan{Category: CatCloudGCE, Name: inst.Name, Worktree: slug}
		if inst.Zone != "" {
			o.Metadata = map[string]string{"zone": filepath.Base(inst.Zone)}
		}
		orphans = append(orphans, o)
	}
	return orphans, nil
}

// DetectCloudAR lists tags for each cfg.ARRepos entry and returns
// inactive tags. AR tags use the `grove-<slug>-<timestamp>` convention;
// anything else is skipped.
func DetectCloudAR(runner Runner, cfg CloudConfig, idx SlugIndex) ([]Orphan, error) {
	if cfg.Project == "" || len(cfg.ARRepos) == 0 {
		return nil, nil
	}
	var orphans []Orphan
	for _, repo := range cfg.ARRepos {
		out, err := runner.Run("gcloud", "artifacts", "docker", "images", "list", repo, "--include-tags", "--format=json")
		if err != nil {
			return nil, fmt.Errorf("gcloud artifacts list %s: %w", repo, err)
		}
		var items []struct {
			Package string   `json:"package"`
			Tags    []string `json:"tags"`
		}
		if err := json.Unmarshal(out, &items); err != nil {
			return nil, fmt.Errorf("parse gcloud artifacts output: %w", err)
		}
		for _, it := range items {
			for _, tag := range it.Tags {
				// Tag shape: grove-<slug>-<ts> OR bare
				// <slug>-<ts>. ExtractSlug handles both.
				slug := ExtractSlug(tag, idx)
				if slug == "" || IsActiveSlug(slug, idx) {
					continue
				}
				ref := it.Package + ":" + tag
				orphans = append(orphans, Orphan{
					Category: CatCloudAR,
					Name:     ref,
					Worktree: slug,
					Metadata: map[string]string{"repo": repo, "tag": tag},
				})
			}
		}
	}
	return orphans, nil
}

// DetectCloudGCS lists gs://<bucket>/<ecosystem>/<profile>/ prefixes
// and reports any <slug>/ under it that is not in the active set.
// Multiple profile sub-prefixes are scanned — callers pass every
// terraform/hybrid-api profile's config.path leading segment as the
// profile dimension via cfg.ARRepos? No: we probe two common profile
// segments ("kitchen-app", "kitchen-infra" style) via a direct listing
// of gs://<bucket>/<ecosystem>/.
func DetectCloudGCS(runner Runner, cfg CloudConfig, idx SlugIndex) ([]Orphan, error) {
	if cfg.StateBucket == "" || cfg.Ecosystem == "" {
		return nil, nil
	}
	ecoPrefix := fmt.Sprintf("gs://%s/%s/", cfg.StateBucket, cfg.Ecosystem)
	// Step 1: list profile segments beneath the ecosystem prefix.
	out, err := runner.Run("gcloud", "storage", "ls", ecoPrefix)
	if err != nil {
		// If the bucket or ecosystem prefix doesn't exist, treat
		// as "no orphans" rather than failing the whole prune.
		return nil, nil
	}
	var orphans []Orphan
	for _, profileURL := range splitNonEmpty(string(out)) {
		profileURL = strings.TrimSpace(profileURL)
		if !strings.HasSuffix(profileURL, "/") {
			continue
		}
		slugOut, err := runner.Run("gcloud", "storage", "ls", profileURL)
		if err != nil {
			continue
		}
		for _, slugURL := range splitNonEmpty(string(slugOut)) {
			slugURL = strings.TrimSpace(slugURL)
			if !strings.HasSuffix(slugURL, "/") {
				continue
			}
			slug := strings.TrimSuffix(strings.TrimPrefix(slugURL, profileURL), "/")
			if slug == "" {
				continue
			}
			if _, ok := idx.Active[slug]; ok {
				continue
			}
			// Only flag slugs we know about (inactive known)
			// so ad-hoc tfstate prefixes from other tools
			// aren't mistaken for grove orphans.
			if ExtractSlug(slug, idx) == "" {
				continue
			}
			orphans = append(orphans, Orphan{
				Category: CatCloudGCS,
				Name:     slugURL,
				Worktree: slug,
				Metadata: map[string]string{"profile_prefix": profileURL},
			})
		}
	}
	return orphans, nil
}

// splitNonEmpty splits s on newlines and drops empty lines.
func splitNonEmpty(s string) []string {
	lines := strings.Split(s, "\n")
	out := lines[:0]
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

// repoPart returns the repository segment of an image ref (before the
// last colon). For "grove-foo-api:latest" this is "grove-foo-api".
func repoPart(ref string) string {
	if i := strings.LastIndex(ref, ":"); i >= 0 {
		return ref[:i]
	}
	return ref
}
