package sessions

import (
	"os"
	"path/filepath"
	"testing"
)

// writeLiveSession registers a session for the current (alive) process under the
// given scope so RecoverSessions can pick it up.
func writeLiveSession(t *testing.T, claudeID, scope string) {
	t.Helper()
	reg, err := NewFileSystemRegistry()
	if err != nil {
		t.Fatalf("NewFileSystemRegistry: %v", err)
	}
	if err := reg.Register(SessionMetadata{
		SessionID:       claudeID,
		ClaudeSessionID: claudeID,
		Provider:        "claude",
		PID:             os.Getpid(),
		Scope:           scope,
	}); err != nil {
		t.Fatalf("Register(%s): %v", claudeID, err)
	}
}

func TestRecoverSessionsForScope(t *testing.T) {
	t.Setenv("GROVE_HOME", t.TempDir())

	writeLiveSession(t, "scoped-a", "/eco/worktree-a")
	writeLiveSession(t, "scoped-b", "/eco/worktree-b")
	writeLiveSession(t, "global-1", "")

	cases := []struct {
		scope string
		want  []string
	}{
		{"/eco/worktree-a", []string{"scoped-a"}},
		{"/eco/worktree-b", []string{"scoped-b"}},
		{"", []string{"global-1"}},
	}
	for _, tc := range cases {
		got, err := RecoverSessionsForScope(tc.scope)
		if err != nil {
			t.Fatalf("RecoverSessionsForScope(%q): %v", tc.scope, err)
		}
		gotIDs := map[string]bool{}
		for _, s := range got {
			gotIDs[s.ClaudeSessionID] = true
		}
		if len(gotIDs) != len(tc.want) {
			t.Fatalf("scope %q: got %v, want %v", tc.scope, gotIDs, tc.want)
		}
		for _, w := range tc.want {
			if !gotIDs[w] {
				t.Errorf("scope %q: missing %q (got %v)", tc.scope, w, gotIDs)
			}
		}
	}

	// The unfiltered sweep still returns everything.
	all, err := RecoverSessions()
	if err != nil {
		t.Fatalf("RecoverSessions: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("RecoverSessions returned %d sessions, want 3", len(all))
	}
}

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
