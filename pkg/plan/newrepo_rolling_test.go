package plan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/grovetools/core/pkg/workspace"
)

// TestNewRepoGetsARollingPlanInItsNotebook is the end-to-end claim behind nav's
// "add repo" action: a repo created inside an ecosystem — with nothing but a
// .git and a README, no grove.toml — resolves to its ecosystem's notebook and
// can be given a rolling plan straight away.
func TestNewRepoGetsARollingPlanInItsNotebook(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "xdgconfig"))

	eco := filepath.Join(root, "eco")
	notebook := filepath.Join(root, "nb")
	if err := os.MkdirAll(eco, 0o755); err != nil {
		t.Fatal(err)
	}
	ecoToml := "name = \"eco\"\nworkspaces = [\"*\"]\n\n" +
		"[notebooks.definitions.test]\nroot_dir = \"" + notebook + "\"\n\n" +
		"[notebooks.rules]\ndefault = \"test\"\n"
	if err := os.WriteFile(filepath.Join(eco, "grove.toml"), []byte(ecoToml), 0o600); err != nil {
		t.Fatal(err)
	}

	repo, _, err := workspace.CreateRepo(eco, "widgets")
	if err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}

	dir, created, err := EnsureRollingPlan(repo)
	if err != nil {
		t.Fatalf("EnsureRollingPlan for the new repo: %v", err)
	}
	if !created {
		t.Error("rolling plan reported as pre-existing in a brand-new repo")
	}
	want := filepath.Join(notebook, "notespaces", "widgets", "plans", RollingPlanName)
	if dir != want {
		t.Fatalf("rolling plan at %q, want %q", dir, want)
	}
	if _, err := os.Stat(filepath.Join(dir, ".grove-plan.yml")); err != nil {
		t.Errorf("rolling plan marker missing: %v", err)
	}

	// And the plan is discoverable the way flow's browser finds it.
	if got := ResolvePlanDir(repo, RollingPlanName); got != want {
		t.Errorf("ResolvePlanDir = %q, want %q", got, want)
	}
}
