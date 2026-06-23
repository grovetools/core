package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gitStatusPorcelain returns the raw `git status --porcelain` output for dir,
// used to assert index/working-tree state after the staging primitives run.
func gitStatusPorcelain(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git status failed: %s", string(out))
	return string(out)
}

func TestStage(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "new.go"), []byte("package main\n"), 0o644))
	// Untracked before staging: "?? new.go".
	require.Contains(t, gitStatusPorcelain(t, dir), "?? new.go")

	require.NoError(t, Stage(dir, "new.go"))

	// After staging an add: index column 'A'.
	status := gitStatusPorcelain(t, dir)
	assert.Contains(t, status, "A  new.go")
	assert.NotContains(t, status, "?? new.go")
}

func TestUnstage(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)
	// git restore --staged needs a HEAD to restore the index from; a real
	// worktree always has one. Establish an initial commit.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "seed.go"), []byte("package main\n"), 0o644))
	runGitCommand(t, dir, "add", "seed.go")
	runGitCommand(t, dir, "commit", "-m", "seed")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "new.go"), []byte("package main\n"), 0o644))
	require.NoError(t, Stage(dir, "new.go"))
	require.Contains(t, gitStatusPorcelain(t, dir), "A  new.go")

	require.NoError(t, Unstage(dir, "new.go"))

	// Back to untracked after unstaging the add.
	status := gitStatusPorcelain(t, dir)
	assert.Contains(t, status, "?? new.go")
	assert.NotContains(t, status, "A  new.go")
}

func TestUnstageModifiedTrackedFile(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)

	path := filepath.Join(dir, "tracked.go")
	require.NoError(t, os.WriteFile(path, []byte("package main\n"), 0o644))
	runGitCommand(t, dir, "add", "tracked.go")
	runGitCommand(t, dir, "commit", "-m", "add tracked")

	require.NoError(t, os.WriteFile(path, []byte("package main\n\nvar X = 1\n"), 0o644))
	require.NoError(t, Stage(dir, "tracked.go"))
	require.Contains(t, gitStatusPorcelain(t, dir), "M  tracked.go") // staged modification

	require.NoError(t, Unstage(dir, "tracked.go"))

	// Modification moves to the working-tree column; file is unchanged on disk.
	assert.Contains(t, gitStatusPorcelain(t, dir), " M tracked.go")
}

func TestDiscard(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)

	path := filepath.Join(dir, "tracked.go")
	original := "package main\n"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o644))
	runGitCommand(t, dir, "add", "tracked.go")
	runGitCommand(t, dir, "commit", "-m", "add tracked")

	// Unstaged modification.
	require.NoError(t, os.WriteFile(path, []byte("package main\n\nvar X = 1\n"), 0o644))
	require.Contains(t, gitStatusPorcelain(t, dir), " M tracked.go")

	require.NoError(t, Discard(dir, "tracked.go"))

	// Working tree is clean and the file is restored from the index.
	assert.Empty(t, strings.TrimSpace(gitStatusPorcelain(t, dir)))
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, original, string(got))
}

func TestStageRejectsUnsafePath(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)

	// Directory traversal is rejected by the shared fileName validator before
	// any git command runs.
	err := Stage(dir, "../escape.go")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid file path")
}

func TestGetBlobHash(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)

	content := "package main\n\nfunc main() {}\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte(content), 0o644))

	hash, err := GetBlobHash(dir, "main.go")
	require.NoError(t, err)
	// A 40-char hex SHA-1 blob id (default object format).
	assert.Regexp(t, `^[0-9a-f]{40}$`, hash)

	// Deterministic: same content -> same hash.
	hash2, err := GetBlobHash(dir, "main.go")
	require.NoError(t, err)
	assert.Equal(t, hash, hash2)
}

func TestGetBlobHashChangesOnEdit(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)

	path := filepath.Join(dir, "main.go")
	require.NoError(t, os.WriteFile(path, []byte("package main\n"), 0o644))
	before, err := GetBlobHash(dir, "main.go")
	require.NoError(t, err)

	// Re-editing the file yields a different content hash — the property the
	// review-state key (path@blob-hash) relies on to un-review on re-edit.
	require.NoError(t, os.WriteFile(path, []byte("package main\n\nvar X = 1\n"), 0o644))
	after, err := GetBlobHash(dir, "main.go")
	require.NoError(t, err)

	assert.NotEqual(t, before, after)
}

func TestGetBlobHashRejectsUnsafePath(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)

	_, err := GetBlobHash(dir, "../escape.go")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid file path")
}
