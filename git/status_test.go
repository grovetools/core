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

// runGitCommand is a test helper to execute git commands.
func runGitCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s failed with output: %s", strings.Join(args, " "), string(output))
}

// setupGitRepo creates a test git repository.
func setupGitRepo(t *testing.T, dir string) {
	t.Helper()
	runGitCommand(t, dir, "init")
	runGitCommand(t, dir, "config", "user.email", "test@example.com")
	runGitCommand(t, dir, "config", "user.name", "Test User")
}

func TestParseChangedFiles(t *testing.T) {
	// Records are NUL-delimited, mirroring `git status --porcelain=v2 -z`.
	// Rename (2) records are immediately followed by a NUL-delimited original
	// path that must be consumed rather than parsed as its own record.
	rec := func(parts ...string) string { return strings.Join(parts, "\x00") + "\x00" }

	tests := []struct {
		name string
		in   string
		want []FileStatus
	}{
		{
			name: "modified working tree",
			in:   rec("1 .M N... 100644 100644 100644 aaa bbb file.go"),
			want: []FileStatus{{Path: "file.go", Staged: '.', Working: 'M'}},
		},
		{
			name: "staged added",
			in:   rec("1 A. N... 000000 100644 100644 000 ccc added.go"),
			want: []FileStatus{{Path: "added.go", Staged: 'A', Working: '.'}},
		},
		{
			name: "deleted working tree",
			in:   rec("1 .D N... 100644 100644 000000 ddd ddd gone.go"),
			want: []FileStatus{{Path: "gone.go", Staged: '.', Working: 'D'}},
		},
		{
			name: "staged modified, working modified",
			in:   rec("1 MM N... 100644 100644 100644 eee fff both.go"),
			want: []FileStatus{{Path: "both.go", Staged: 'M', Working: 'M'}},
		},
		{
			name: "untracked",
			in:   rec("? newfile.go"),
			want: []FileStatus{{Path: "newfile.go", Staged: '?', Working: '?'}},
		},
		{
			name: "renamed consumes original path record",
			in:   rec("2 R. N... 100644 100644 100644 ggg hhh R100 new.go", "old.go"),
			want: []FileStatus{{Path: "new.go", Staged: 'R', Working: '.'}},
		},
		{
			name: "path with a space",
			in:   rec("1 .M N... 100644 100644 100644 iii jjj dir/my file.go"),
			want: []FileStatus{{Path: "dir/my file.go", Staged: '.', Working: 'M'}},
		},
		{
			name: "mixed records",
			in: rec(
				"1 .M N... 100644 100644 100644 a b modified.go",
				"2 R. N... 100644 100644 100644 c d R100 renamed.go", "orig.go",
				"? untracked.go",
				"1 A. N... 000000 100644 100644 e f staged.go",
			),
			want: []FileStatus{
				{Path: "modified.go", Staged: '.', Working: 'M'},
				{Path: "renamed.go", Staged: 'R', Working: '.'},
				{Path: "untracked.go", Staged: '?', Working: '?'},
				{Path: "staged.go", Staged: 'A', Working: '.'},
			},
		},
		{
			name: "empty output",
			in:   "",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseChangedFiles(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseDiffNameStatusZ(t *testing.T) {
	// `git diff --name-status -z` emits the status token and each path as
	// SEPARATE NUL-delimited records. Ordinary changes are "<status>\0<path>";
	// renames/copies are "<status>\0<oldpath>\0<newpath>" and keep the NEW path.
	rec := func(parts ...string) string { return strings.Join(parts, "\x00") + "\x00" }

	tests := []struct {
		name string
		in   string
		want []FileStatus
	}{
		{
			name: "modified",
			in:   rec("M", "file.go"),
			want: []FileStatus{{Path: "file.go", Staged: '.', Working: 'M'}},
		},
		{
			name: "added",
			in:   rec("A", "added.go"),
			want: []FileStatus{{Path: "added.go", Staged: '.', Working: 'A'}},
		},
		{
			name: "deleted",
			in:   rec("D", "gone.go"),
			want: []FileStatus{{Path: "gone.go", Staged: '.', Working: 'D'}},
		},
		{
			name: "renamed keeps new path and consumes old",
			in:   rec("R100", "old.go", "new.go"),
			want: []FileStatus{{Path: "new.go", Staged: '.', Working: 'R'}},
		},
		{
			name: "path with a space",
			in:   rec("M", "dir/my file.go"),
			want: []FileStatus{{Path: "dir/my file.go", Staged: '.', Working: 'M'}},
		},
		{
			name: "mixed records",
			in: rec(
				"M", "modified.go",
				"R100", "orig.go", "renamed.go",
				"A", "added.go",
				"D", "deleted.go",
			),
			want: []FileStatus{
				{Path: "modified.go", Staged: '.', Working: 'M'},
				{Path: "renamed.go", Staged: '.', Working: 'R'},
				{Path: "added.go", Staged: '.', Working: 'A'},
				{Path: "deleted.go", Staged: '.', Working: 'D'},
			},
		},
		{
			name: "empty output",
			in:   "",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDiffNameStatusZ(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetChangedFiles(t *testing.T) {
	tempDir := t.TempDir()
	setupGitRepo(t, tempDir)

	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "initial.txt"), []byte("initial"), 0o644))
	runGitCommand(t, tempDir, "add", "initial.txt")
	runGitCommand(t, tempDir, "commit", "-m", "initial commit")

	// Modify tracked, stage a new file, leave one untracked.
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "initial.txt"), []byte("changed"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "staged.txt"), []byte("staged"), 0o644))
	runGitCommand(t, tempDir, "add", "staged.txt")
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "untracked.txt"), []byte("untracked"), 0o644))

	files, err := GetChangedFiles(tempDir)
	require.NoError(t, err)

	byPath := make(map[string]FileStatus)
	for _, f := range files {
		byPath[f.Path] = f
	}

	require.Contains(t, byPath, "initial.txt")
	assert.Equal(t, 'M', byPath["initial.txt"].Working)
	require.Contains(t, byPath, "staged.txt")
	assert.Equal(t, 'A', byPath["staged.txt"].Staged)
	require.Contains(t, byPath, "untracked.txt")
	assert.Equal(t, '?', byPath["untracked.txt"].Working)
}

func TestGetStatus(t *testing.T) {
	t.Run("invalid path", func(t *testing.T) {
		_, err := GetStatus("/non/existent/path")
		assert.Error(t, err)
	})

	t.Run("non-git directory", func(t *testing.T) {
		tempDir := t.TempDir()
		_, err := GetStatus(tempDir)
		assert.Error(t, err)
	})

	t.Run("clean repo", func(t *testing.T) {
		tempDir := t.TempDir()
		setupGitRepo(t, tempDir)
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "file.txt"), []byte("content"), 0o644))
		runGitCommand(t, tempDir, "add", "file.txt")
		runGitCommand(t, tempDir, "commit", "-m", "initial commit")

		status, err := GetStatus(tempDir)
		require.NoError(t, err)
		assert.False(t, status.IsDirty)
		assert.Equal(t, 0, status.ModifiedCount)
		assert.Equal(t, 0, status.StagedCount)
		assert.Equal(t, 0, status.UntrackedCount)
		assert.Equal(t, 0, status.AheadCount)
		assert.Equal(t, 0, status.BehindCount)
		assert.False(t, status.HasUpstream)
		assert.NotEmpty(t, status.Branch)
	})

	t.Run("with changes", func(t *testing.T) {
		tempDir := t.TempDir()
		setupGitRepo(t, tempDir)

		// Create initial commit first
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "initial.txt"), []byte("initial"), 0o644))
		runGitCommand(t, tempDir, "add", "initial.txt")
		runGitCommand(t, tempDir, "commit", "-m", "initial commit")

		// Staged file (new file that's staged but not committed)
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "staged.txt"), []byte("staged"), 0o644))
		runGitCommand(t, tempDir, "add", "staged.txt")

		// Modified file (modify the initial file)
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "initial.txt"), []byte("modified"), 0o644))

		// Untracked file
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "untracked.txt"), []byte("untracked"), 0o644))

		status, err := GetStatus(tempDir)
		require.NoError(t, err)
		assert.True(t, status.IsDirty)
		assert.Equal(t, 1, status.StagedCount, "staged.txt should be staged")
		assert.Equal(t, 1, status.ModifiedCount, "initial.txt should be modified")
		assert.Equal(t, 1, status.UntrackedCount, "untracked.txt should be untracked")
	})

	t.Run("with upstream", func(t *testing.T) {
		// Setup remote and local repos
		baseDir := t.TempDir()
		remoteDir := filepath.Join(baseDir, "remote.git")
		localDir := filepath.Join(baseDir, "local")

		// Init bare remote with main branch
		require.NoError(t, os.Mkdir(remoteDir, 0o755))
		runGitCommand(t, remoteDir, "init", "--bare", "--initial-branch=main")

		// Clone local
		runGitCommand(t, baseDir, "clone", "remote.git", "local")
		setupGitRepo(t, localDir) // to set user config

		// Initial commit and push
		require.NoError(t, os.WriteFile(filepath.Join(localDir, "file.txt"), []byte("1"), 0o644))
		runGitCommand(t, localDir, "add", ".")
		runGitCommand(t, localDir, "commit", "-m", "c1")
		runGitCommand(t, localDir, "push", "origin", "main")

		// Test ahead
		require.NoError(t, os.WriteFile(filepath.Join(localDir, "file2.txt"), []byte("2"), 0o644))
		runGitCommand(t, localDir, "add", ".")
		runGitCommand(t, localDir, "commit", "-m", "c2")

		status, err := GetStatus(localDir)
		require.NoError(t, err)
		assert.True(t, status.HasUpstream)
		assert.Equal(t, 1, status.AheadCount)
		assert.Equal(t, 0, status.BehindCount)

		// Test behind - push local changes, then make new changes in another clone
		runGitCommand(t, localDir, "push", "origin", "main")

		// In another clone, push a different commit to remote to make local behind
		anotherLocalDir := filepath.Join(baseDir, "another")
		runGitCommand(t, baseDir, "clone", "remote.git", "another")
		setupGitRepo(t, anotherLocalDir)
		require.NoError(t, os.WriteFile(filepath.Join(anotherLocalDir, "another-file.txt"), []byte("remote change"), 0o644))
		runGitCommand(t, anotherLocalDir, "add", ".")
		runGitCommand(t, anotherLocalDir, "commit", "-m", "remote-c3")
		runGitCommand(t, anotherLocalDir, "push", "origin", "main")

		// Now fetch in original local repo
		runGitCommand(t, localDir, "fetch")

		status, err = GetStatus(localDir)
		require.NoError(t, err)
		assert.True(t, status.HasUpstream)
		assert.Equal(t, 0, status.AheadCount, "Should be 0 ahead after pushing")
		assert.Equal(t, 1, status.BehindCount, "Should be 1 behind remote")
	})
}
