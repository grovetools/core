package prune

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

// fakeRunner returns canned output per command key. Keys are "name arg0
// arg1 …" joined with spaces; matches are exact to keep tests legible
// when diagnosing failures.
type fakeRunner struct {
	responses map[string]fakeResp
	calls     []string
}

type fakeResp struct {
	out []byte
	err error
}

func (f *fakeRunner) Run(name string, args ...string) ([]byte, error) {
	key := name
	for _, a := range args {
		key += " " + a
	}
	f.calls = append(f.calls, key)
	r, ok := f.responses[key]
	if !ok {
		return nil, os.ErrNotExist
	}
	return r.out, r.err
}

func TestExtractSlug_PrefersLongestKnown(t *testing.T) {
	idx := NewSlugIndex([]string{"env-continued"}, []string{"tier1-tf-rerun", "tier1"})
	cases := map[string]string{
		"grove-tier1-tf-rerun-api":    "tier1-tf-rerun",
		"tier1-tf-rerun-web":          "tier1-tf-rerun",
		"grove-tier1-api":             "tier1",
		"grove-env-continued-web":     "env-continued",
		"kitchen-env-vpc":             "",
		"kitchen-env-cloudbuild":      "",
		"kitchen-vpc-conn":            "",
		"grove-unknown-slug-api":      "",
	}
	for name, want := range cases {
		got := ExtractSlug(name, idx)
		if got != want {
			t.Errorf("ExtractSlug(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestDetectDockerImages(t *testing.T) {
	idx := NewSlugIndex([]string{"env-continued"}, []string{"tier1-tf-rerun"})
	runner := &fakeRunner{responses: map[string]fakeResp{
		"docker images --format {{.Repository}}:{{.Tag}} --filter reference=grove-*": {out: []byte(
			"grove-env-continued-api:latest\n" + // active, skip
				"grove-tier1-tf-rerun-api:latest\n" + // orphan
				"grove-tier1-tf-rerun-web:latest\n" + // orphan
				"<none>:<none>\n" + // dangling, skip
				"",
		)},
	}}
	got, err := DetectDockerImages(runner, idx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 orphans, got %d: %+v", len(got), got)
	}
	for _, o := range got {
		if o.Worktree != "tier1-tf-rerun" {
			t.Errorf("unexpected worktree: %q", o.Worktree)
		}
		if o.Category != CatDockerImage {
			t.Errorf("unexpected category: %q", o.Category)
		}
	}
}

func TestDetectDockerVolumes_FiltersActive(t *testing.T) {
	idx := NewSlugIndex([]string{"alive"}, []string{"dead"})
	runner := &fakeRunner{responses: map[string]fakeResp{
		"docker volume ls --format {{.Name}} --filter name=grove-*": {out: []byte(
			"grove-alive-data\ngrove-dead-data\ngrove-unknown-thing\n",
		)},
	}}
	got, _ := DetectDockerVolumes(runner, idx)
	if len(got) != 1 || got[0].Name != "grove-dead-data" {
		t.Fatalf("want single dead volume, got %+v", got)
	}
}

func TestDetectHostWorktreeDirs(t *testing.T) {
	tmp := t.TempDir()
	base := filepath.Join(tmp, ".grove-worktrees")
	for _, d := range []string{"alive", "dead", "tier1-tf-rerun"} {
		if err := os.MkdirAll(filepath.Join(base, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	idx := NewSlugIndex([]string{"alive"}, nil)
	got, err := DetectHostWorktreeDirs(tmp, idx)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(got))
	for _, o := range got {
		names = append(names, filepath.Base(o.Name))
	}
	sort.Strings(names)
	want := []string{"dead", "tier1-tf-rerun"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("got %v want %v", names, want)
	}
}

func TestDetectCloudRun_RequiresProject(t *testing.T) {
	idx := NewSlugIndex([]string{"alive"}, nil)
	orphans, err := DetectCloudRun(&fakeRunner{}, CloudConfig{}, idx)
	if err != nil || orphans != nil {
		t.Fatalf("expected nil/nil with empty project, got %v / %v", orphans, err)
	}
}

func TestDetectCloudRun_ParsesAndFilters(t *testing.T) {
	idx := NewSlugIndex([]string{"alive"}, []string{"tier1-tf-rerun"})
	payload := `[
		{"metadata":{"name":"alive-api"}},
		{"metadata":{"name":"tier1-tf-rerun-api"}},
		{"metadata":{"name":"tier1-tf-rerun-web"}},
		{"metadata":{"name":"kitchen-env-cloudbuild"}}
	]`
	runner := &fakeRunner{responses: map[string]fakeResp{
		"gcloud run services list --format=json --project=p --region=r": {out: []byte(payload)},
	}}
	cfg := CloudConfig{Project: "p", Region: "r"}
	got, err := DetectCloudRun(runner, cfg, idx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 orphans, got %d: %+v", len(got), got)
	}
	for _, o := range got {
		if o.Worktree != "tier1-tf-rerun" {
			t.Errorf("unexpected slug: %q", o.Worktree)
		}
	}
}
