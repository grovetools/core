package git

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLog(t *testing.T) {
	// Records: <hash>\x1f<author>\x1f<email>\x1f<date>\x1f<refs>\x1f<rel>\x1f<parents>\x1f<subject>, NUL-joined.
	rec := func(fields ...string) string {
		out := ""
		for i, f := range fields {
			if i > 0 {
				out += "\x1f"
			}
			out += f
		}
		return out
	}
	output := rec("abc123", "Ada", "ada@example.com", "2026-06-23T10:00:00-07:00", "HEAD -> main, origin/main", "2 hours ago", "def456 999aaa", "feat: first") + "\x00" +
		rec("def456", "Bob", "bob@example.com", "2026-06-22T09:00:00-07:00", "", "1 day ago", "", "fix: second\nwith newline") + "\x00"

	entries := parseLog(output)
	require.Len(t, entries, 2)

	assert.Equal(t, "abc123", entries[0].Hash)
	assert.Equal(t, "Ada", entries[0].Author)
	assert.Equal(t, "ada@example.com", entries[0].Email)
	assert.Equal(t, "HEAD -> main, origin/main", entries[0].Refs)
	assert.Equal(t, "2 hours ago", entries[0].RelativeDate)
	assert.Equal(t, "feat: first", entries[0].Subject)
	assert.False(t, entries[0].Date.IsZero())
	// A merge commit's two parents are split on whitespace.
	assert.Equal(t, []string{"def456", "999aaa"}, entries[0].Parents)

	// A commit with no refs decoration parses with an empty Refs field.
	assert.Equal(t, "", entries[1].Refs)
	// An empty parents field (root commit) yields no parents.
	assert.Empty(t, entries[1].Parents)
	// A subject containing a newline survives because records are NUL-delimited.
	assert.Equal(t, "fix: second\nwith newline", entries[1].Subject)
}

func TestParseLogSkipsMalformed(t *testing.T) {
	// Trailing empty record and a short (5-field) record are both skipped; only
	// the well-formed 8-field record parses.
	output := "onlyhash\x00" +
		"h2\x1fa\x1fe\x1f2026-06-23T10:00:00Z\x1fshort\x00" +
		"h\x1fa\x1fe\x1f2026-06-23T10:00:00Z\x1f\x1f3 days ago\x1fparent1\x1fsubject\x00"
	entries := parseLog(output)
	require.Len(t, entries, 1)
	assert.Equal(t, "h", entries[0].Hash)
	assert.Equal(t, "subject", entries[0].Subject)
	assert.Equal(t, "3 days ago", entries[0].RelativeDate)
	assert.Equal(t, []string{"parent1"}, entries[0].Parents)
}

func TestGetLog(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\n"), 0o644))
	runGitCommand(t, dir, "add", "a.go")
	runGitCommand(t, dir, "commit", "-m", "feat: add a")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.go"), []byte("package main\n"), 0o644))
	runGitCommand(t, dir, "add", "b.go")
	runGitCommand(t, dir, "commit", "-m", "feat: add b")

	entries, err := GetLog(dir, 10)
	require.NoError(t, err)
	require.Len(t, entries, 2)

	// Most recent first.
	assert.Equal(t, "feat: add b", entries[0].Subject)
	assert.Equal(t, "feat: add a", entries[1].Subject)
	assert.Equal(t, "Test User", entries[0].Author)
	assert.Equal(t, "test@example.com", entries[0].Email)
	assert.Regexp(t, `^[0-9a-f]{40}$`, entries[0].Hash)
	assert.WithinDuration(t, time.Now(), entries[0].Date, 5*time.Minute)
	// The tip commit carries a HEAD ref decoration, and a humanized date.
	assert.Contains(t, entries[0].Refs, "HEAD")
	assert.NotEmpty(t, entries[0].RelativeDate)
	// The tip commit's sole parent is the earlier commit; the root has none.
	require.Len(t, entries[0].Parents, 1)
	assert.Equal(t, entries[1].Hash, entries[0].Parents[0])
	assert.Empty(t, entries[1].Parents)
}

func TestGetLogRespectsLimit(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)

	for _, name := range []string{"a", "b", "c"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name+".go"), []byte("package main\n"), 0o644))
		runGitCommand(t, dir, "add", name+".go")
		runGitCommand(t, dir, "commit", "-m", "add "+name)
	}

	entries, err := GetLog(dir, 2)
	require.NoError(t, err)
	assert.Len(t, entries, 2)
}

func TestGetLogEmptyRepo(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir) // init + config, but no commits

	entries, err := GetLog(dir, 10)
	require.NoError(t, err, "empty repo should not be an error")
	assert.Nil(t, entries)
}
