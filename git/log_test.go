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
	// Two records: <hash>\x1f<author>\x1f<email>\x1f<date>\x1f<subject>, NUL-joined.
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
	output := rec("abc123", "Ada", "ada@example.com", "2026-06-23T10:00:00-07:00", "feat: first") + "\x00" +
		rec("def456", "Bob", "bob@example.com", "2026-06-22T09:00:00-07:00", "fix: second\nwith newline") + "\x00"

	entries := parseLog(output)
	require.Len(t, entries, 2)

	assert.Equal(t, "abc123", entries[0].Hash)
	assert.Equal(t, "Ada", entries[0].Author)
	assert.Equal(t, "ada@example.com", entries[0].Email)
	assert.Equal(t, "feat: first", entries[0].Subject)
	assert.False(t, entries[0].Date.IsZero())

	// A subject containing a newline survives because records are NUL-delimited.
	assert.Equal(t, "fix: second\nwith newline", entries[1].Subject)
}

func TestParseLogSkipsMalformed(t *testing.T) {
	// Trailing empty record and a short record are both skipped.
	output := "onlyhash\x00" + "h\x1fa\x1fe\x1f2026-06-23T10:00:00Z\x1fsubject\x00"
	entries := parseLog(output)
	require.Len(t, entries, 1)
	assert.Equal(t, "h", entries[0].Hash)
	assert.Equal(t, "subject", entries[0].Subject)
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
