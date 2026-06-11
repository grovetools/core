package sessions

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveClaudeSessionDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	const sessionID = "3c7166b2-aaaa-bbbb-cccc-000000000000"
	projects := filepath.Join(home, ".claude", "projects")

	// The same session id fragments across two project-slug dirs.
	slugA := filepath.Join(projects, "-Users-solair-Code-worktree", sessionID)
	slugB := filepath.Join(projects, "-Users-solair-Code-worktree-flow", sessionID)
	for _, dir := range []string{slugA, slugB} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) failed: %v", dir, err)
		}
	}

	// A different session id under a third slug must not match.
	other := filepath.Join(projects, "-Users-solair-Code-other", "deadbeef-1111-2222-3333-444444444444")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) failed: %v", other, err)
	}

	// A plain file named after the session id must be skipped (directories only).
	fileSlug := filepath.Join(projects, "-Users-solair-Code-filecase")
	if err := os.MkdirAll(fileSlug, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) failed: %v", fileSlug, err)
	}
	if err := os.WriteFile(filepath.Join(fileSlug, sessionID), []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	dirs, err := ResolveClaudeSessionDirs(sessionID)
	if err != nil {
		t.Fatalf("ResolveClaudeSessionDirs returned error: %v", err)
	}

	// Lexicographic order of full paths: '-' sorts before '/', so the
	// "...-worktree-flow/<id>" path precedes "...-worktree/<id>".
	want := []string{slugB, slugA}
	if len(dirs) != len(want) {
		t.Fatalf("got %d dirs %v, want %d %v", len(dirs), dirs, len(want), want)
	}
	for i := range want {
		if dirs[i] != want[i] {
			t.Errorf("dirs[%d] = %q, want %q", i, dirs[i], want[i])
		}
	}
}

func TestResolveClaudeSessionDirsNoMatches(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dirs, err := ResolveClaudeSessionDirs("no-such-session")
	if err != nil {
		t.Fatalf("ResolveClaudeSessionDirs returned error: %v", err)
	}
	if len(dirs) != 0 {
		t.Errorf("got %v, want empty", dirs)
	}
}

func TestResolveClaudeSessionDirsEmptyID(t *testing.T) {
	if _, err := ResolveClaudeSessionDirs(""); err == nil {
		t.Error("expected error for empty session ID, got nil")
	}
}
