package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCreateRepoMakesGitAndReadme(t *testing.T) {
	parent := t.TempDir()
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")

	path, committed, err := CreateRepo(parent, "widgets")
	if err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}
	if path != filepath.Join(parent, "widgets") {
		t.Errorf("path = %q", path)
	}
	if info, err := os.Stat(filepath.Join(path, ".git")); err != nil || !info.IsDir() {
		t.Errorf(".git missing: %v", err)
	}
	readme, err := os.ReadFile(filepath.Join(path, ReadmeFileName))
	if err != nil {
		t.Fatalf("README missing: %v", err)
	}
	if len(readme) != 0 {
		t.Errorf("README is not empty: %q", readme)
	}
	if !committed {
		t.Error("initial commit was not made despite a configured identity")
	}
	// The repo must be clean, or every plan created in it starts with a dirty
	// working tree warning.
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = path
	if out, err := cmd.Output(); err != nil || len(out) != 0 {
		t.Errorf("fresh repo is not clean: %q (err=%v)", out, err)
	}
}

func TestCreateRepoRejectsBadNamesAndCollisions(t *testing.T) {
	parent := t.TempDir()
	for _, name := range []string{"", ".", "..", "a/b", "../escape", ".hidden", "-flag"} {
		if _, _, err := CreateRepo(parent, name); err == nil {
			t.Errorf("name %q accepted", name)
		}
	}
	if _, _, err := CreateRepo(parent, "taken"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := CreateRepo(parent, "taken"); err == nil {
		t.Error("existing directory silently reused")
	}
	if entries, err := os.ReadDir(parent); err != nil || len(entries) != 1 {
		t.Errorf("rejected creations left residue: %v (err=%v)", entries, err)
	}
}
