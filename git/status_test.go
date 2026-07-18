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

func TestParseNumstatZ(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want map[string][2]int
	}{
		{
			name: "single file",
			in:   "12\t3\tfile.go\x00",
			want: map[string][2]int{"file.go": {12, 3}},
		},
		{
			name: "multiple files",
			in:   "12\t3\ta.go\x005\t0\tb.go\x00",
			want: map[string][2]int{"a.go": {12, 3}, "b.go": {5, 0}},
		},
		{
			name: "binary file dashes",
			in:   "-\t-\timg.png\x00",
			want: map[string][2]int{"img.png": {0, 0}},
		},
		{
			name: "rename keeps new path",
			in:   "4\t2\t\x00old.go\x00new.go\x00",
			want: map[string][2]int{"new.go": {4, 2}},
		},
		{
			name: "empty",
			in:   "",
			want: map[string][2]int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseNumstatZ(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseBlobNumstat(t *testing.T) {
	tests := []struct {
		name        string
		in          string
		wantAdded   int
		wantDeleted int
	}{
		{
			name:        "normal blob pair",
			in:          "5\t2\tdeadbeef => cafef00d\n",
			wantAdded:   5,
			wantDeleted: 2,
		},
		{
			name:        "binary sentinel",
			in:          "-\t-\tdeadbeef => cafef00d\n",
			wantAdded:   0,
			wantDeleted: 0,
		},
		{
			name:        "missing path column",
			in:          "7\t3\n",
			wantAdded:   7,
			wantDeleted: 3,
		},
		{
			name:        "no trailing newline",
			in:          "1\t1",
			wantAdded:   1,
			wantDeleted: 1,
		},
		{
			name:        "empty output",
			in:          "",
			wantAdded:   0,
			wantDeleted: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			added, deleted := parseBlobNumstat(tt.in)
			assert.Equal(t, tt.wantAdded, added, "added")
			assert.Equal(t, tt.wantDeleted, deleted, "deleted")
		})
	}
}

// TestGetBlobDiffNumstat exercises the helper end-to-end against two committed
// blobs (both present in the object database) and confirms a bad object errors.
func TestGetBlobDiffNumstat(t *testing.T) {
	tempDir := t.TempDir()
	setupGitRepo(t, tempDir)

	revParse := func(rev string) string {
		cmd := exec.Command("git", "rev-parse", rev)
		cmd.Dir = tempDir
		out, err := cmd.Output()
		require.NoError(t, err)
		return strings.TrimSpace(string(out))
	}

	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "f.txt"), []byte("a\nb\nc\n"), 0o644))
	runGitCommand(t, tempDir, "add", "f.txt")
	runGitCommand(t, tempDir, "commit", "-m", "v1")
	oldBlob := revParse("HEAD:f.txt")

	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "f.txt"), []byte("a\nB\nc\nd\n"), 0o644))
	runGitCommand(t, tempDir, "add", "f.txt")
	runGitCommand(t, tempDir, "commit", "-m", "v2")
	newBlob := revParse("HEAD:f.txt")

	added, deleted, err := GetBlobDiffNumstat(tempDir, oldBlob, newBlob)
	assert.NoError(t, err)
	assert.Equal(t, 2, added)
	assert.Equal(t, 1, deleted)

	// An unknown blob (e.g. a never-staged working-tree hash) errors rather than
	// silently returning zero churn.
	_, _, err = GetBlobDiffNumstat(tempDir, oldBlob, "0000000000000000000000000000000000000000")
	assert.Error(t, err)
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

// TestGetChangedFilesSinceRef diffs the working tree against a historical
// commit hash and asserts both the name-status parse and the numstat churn.
func TestGetChangedFilesSinceRef(t *testing.T) {
	tempDir := t.TempDir()
	setupGitRepo(t, tempDir)

	// Commit a base revision, capture its hash, then mutate the tree.
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "keep.txt"), []byte("one\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "gone.txt"), []byte("delete me\n"), 0o644))
	runGitCommand(t, tempDir, "add", "keep.txt", "gone.txt")
	runGitCommand(t, tempDir, "commit", "-m", "base")

	revCmd := exec.Command("git", "rev-parse", "HEAD")
	revCmd.Dir = tempDir
	out, err := revCmd.Output()
	require.NoError(t, err)
	base := strings.TrimSpace(string(out))

	// Modify a tracked file, add a brand-new tracked file, delete another.
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "keep.txt"), []byte("one\ntwo\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "added.txt"), []byte("new\n"), 0o644))
	runGitCommand(t, tempDir, "rm", "gone.txt")
	runGitCommand(t, tempDir, "add", "keep.txt", "added.txt")

	files, err := GetChangedFilesSinceRef(tempDir, base)
	require.NoError(t, err)

	byPath := make(map[string]FileStatus)
	for _, f := range files {
		byPath[f.Path] = f
	}

	require.Contains(t, byPath, "keep.txt")
	assert.Equal(t, 'M', byPath["keep.txt"].Working)
	assert.Equal(t, 1, byPath["keep.txt"].LinesAdded)

	require.Contains(t, byPath, "added.txt")
	assert.Equal(t, 'A', byPath["added.txt"].Working)
	assert.Equal(t, 1, byPath["added.txt"].LinesAdded)

	require.Contains(t, byPath, "gone.txt")
	assert.Equal(t, 'D', byPath["gone.txt"].Working)
}

// TestGetChangedFilesSinceRefEmptyRef guards the caller-bug rule: an empty ref
// must error rather than silently diffing against HEAD.
func TestGetChangedFilesSinceRefEmptyRef(t *testing.T) {
	_, err := GetChangedFilesSinceRef(t.TempDir(), "")
	require.Error(t, err)
}

// TestGetChangedFilesInRange covers the commit-to-commit diff used by the
// per-job review scope: only what the range's commits changed is listed —
// working-tree edits made after the end commit must not appear — and the
// numstat churn is merged per file.
func TestGetChangedFilesInRange(t *testing.T) {
	tempDir := t.TempDir()
	setupGitRepo(t, tempDir)

	revParse := func() string {
		cmd := exec.Command("git", "rev-parse", "HEAD")
		cmd.Dir = tempDir
		out, err := cmd.Output()
		require.NoError(t, err)
		return strings.TrimSpace(string(out))
	}

	// Commit 1: the range start.
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "keep.txt"), []byte("one\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "gone.txt"), []byte("delete me\n"), 0o644))
	runGitCommand(t, tempDir, "add", "keep.txt", "gone.txt")
	runGitCommand(t, tempDir, "commit", "-m", "base")
	start := revParse()

	// Commit 2: modify, add, delete — the range end.
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "keep.txt"), []byte("one\ntwo\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "added.txt"), []byte("new\n"), 0o644))
	runGitCommand(t, tempDir, "rm", "gone.txt")
	runGitCommand(t, tempDir, "add", "keep.txt", "added.txt")
	runGitCommand(t, tempDir, "commit", "-m", "job work")
	end := revParse()

	// A working-tree edit AFTER the end commit must not leak into the range.
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "keep.txt"), []byte("one\ntwo\nthree\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "untracked.txt"), []byte("later\n"), 0o644))

	files, err := GetChangedFilesInRange(tempDir, start, end)
	require.NoError(t, err)

	byPath := make(map[string]FileStatus)
	for _, f := range files {
		byPath[f.Path] = f
	}

	require.Len(t, files, 3)
	require.Contains(t, byPath, "keep.txt")
	assert.Equal(t, 'M', byPath["keep.txt"].Working)
	assert.Equal(t, 1, byPath["keep.txt"].LinesAdded)
	assert.Equal(t, 0, byPath["keep.txt"].LinesDeleted)

	require.Contains(t, byPath, "added.txt")
	assert.Equal(t, 'A', byPath["added.txt"].Working)
	assert.Equal(t, 1, byPath["added.txt"].LinesAdded)

	require.Contains(t, byPath, "gone.txt")
	assert.Equal(t, 'D', byPath["gone.txt"].Working)
	assert.Equal(t, 1, byPath["gone.txt"].LinesDeleted)

	assert.NotContains(t, byPath, "untracked.txt")
}

// TestGetChangedFilesInRangeEmptyRef guards the caller-bug rule shared with
// GetChangedFilesSinceRef: empty endpoints error instead of silently diffing.
func TestGetChangedFilesInRangeEmptyRef(t *testing.T) {
	if _, err := GetChangedFilesInRange(t.TempDir(), "", "HEAD"); err == nil {
		t.Error("empty start ref should error")
	}
	if _, err := GetChangedFilesInRange(t.TempDir(), "HEAD", ""); err == nil {
		t.Error("empty end ref should error")
	}
}

// TestGetChangedFilesUntrackedNewDir guards the -uall behavior: a new file in a
// directory that does not yet contain any tracked files must surface as the file
// itself, not as a collapsed `dir/` record. Without --untracked-files=all git
// reports only the directory, which the change tree renders as an empty folder.
func TestGetChangedFilesUntrackedNewDir(t *testing.T) {
	tempDir := t.TempDir()
	setupGitRepo(t, tempDir)

	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "initial.txt"), []byte("initial"), 0o644))
	runGitCommand(t, tempDir, "add", "initial.txt")
	runGitCommand(t, tempDir, "commit", "-m", "initial commit")

	// A brand-new directory with files, none of which the repo has ever tracked.
	newDir := filepath.Join(tempDir, "newpkg")
	require.NoError(t, os.MkdirAll(newDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(newDir, "a.go"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(newDir, "b.go"), []byte("b"), 0o644))

	files, err := GetChangedFiles(tempDir)
	require.NoError(t, err)

	byPath := make(map[string]FileStatus)
	for _, f := range files {
		byPath[f.Path] = f
	}

	// Each file is listed individually; the collapsed directory record is not.
	require.Contains(t, byPath, "newpkg/a.go")
	require.Contains(t, byPath, "newpkg/b.go")
	assert.Equal(t, '?', byPath["newpkg/a.go"].Working)
	assert.NotContains(t, byPath, "newpkg/")
}

// TestListAllFiles verifies the all-files listing includes tracked AND
// untracked-not-ignored files, deduped and sorted, while honoring .gitignore.
func TestListAllFiles(t *testing.T) {
	tempDir := t.TempDir()
	setupGitRepo(t, tempDir)

	// Tracked file (committed) plus a nested tracked file.
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "tracked.txt"), []byte("a"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "pkg"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "pkg", "mod.go"), []byte("b"), 0o644))
	runGitCommand(t, tempDir, "add", "tracked.txt", "pkg/mod.go")
	runGitCommand(t, tempDir, "commit", "-m", "initial")

	// Modify a tracked file (still tracked, must appear once), add an untracked
	// file, and an ignored one that must NOT appear.
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "tracked.txt"), []byte("changed"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "new.txt"), []byte("new"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, ".gitignore"), []byte("ignored.txt\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "ignored.txt"), []byte("nope"), 0o644))

	files, err := ListAllFiles(tempDir)
	require.NoError(t, err)

	set := make(map[string]bool)
	for _, f := range files {
		set[f] = true
	}

	assert.True(t, set["tracked.txt"], "tracked file should be listed")
	assert.True(t, set["pkg/mod.go"], "nested tracked file should be listed")
	assert.True(t, set["new.txt"], "untracked-not-ignored file should be listed")
	assert.True(t, set[".gitignore"], "the .gitignore itself is an untracked file and should be listed")
	assert.False(t, set["ignored.txt"], "ignored file must be excluded")

	// No duplicates and sorted ascending.
	for i := 1; i < len(files); i++ {
		assert.Less(t, files[i-1], files[i], "files should be sorted with no duplicates")
	}
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
