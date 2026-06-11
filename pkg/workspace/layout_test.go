package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grovetools/core/pkg/paths"
)

// TestWorktreeBases_LegacyOnly pins the Phase-1 contract: a single,
// legacy-first, identifier-level base under the git root.
func TestWorktreeBases_LegacyOnly(t *testing.T) {
	gitRoot := "/path/to/my-ecosystem"
	got := WorktreeBases(gitRoot)
	want := []string{filepath.Join(gitRoot, ".grove-worktrees")}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("WorktreeBases(%q) = %v, want %v", gitRoot, got, want)
	}
}

// TestIsWorktreePath_LegacyEquivalence pins the Phase-1 contract: byte-
// identical to strings.Contains(path, ".grove-worktrees").
func TestIsWorktreePath_LegacyEquivalence(t *testing.T) {
	cases := []string{
		"/path/to/eco/.grove-worktrees/feature",
		"/path/to/eco/.grove-worktrees/feature/sub/dir",
		"/path/to/eco/.grove-worktrees",
		"/path/to/eco",
		"/path/to/.grove-worktreesX/feature", // substring match, like today
		"",
		"/",
	}
	for _, path := range cases {
		want := strings.Contains(path, ".grove-worktrees")
		if got := IsWorktreePath(path); got != want {
			t.Errorf("IsWorktreePath(%q) = %v, want legacy-equivalent %v", path, got, want)
		}
	}
}

// TestWorktreeOwner_LegacyEquivalence pins the Phase-1 contract:
// for legacy worktree paths the owner is exactly Dir(Dir(path)).
func TestWorktreeOwner_LegacyEquivalence(t *testing.T) {
	tests := []struct {
		path   string
		wantOK bool
	}{
		{"/path/to/eco/.grove-worktrees/feature", true},
		{"/path/to/eco/sub/.grove-worktrees/fix-1", true},
		{"/path/to/eco/sub", false},
		{"", false},
	}
	for _, tt := range tests {
		got, ok := WorktreeOwner(tt.path)
		if ok != tt.wantOK {
			t.Errorf("WorktreeOwner(%q) ok = %v, want %v", tt.path, ok, tt.wantOK)
			continue
		}
		if ok {
			want := filepath.Dir(filepath.Dir(tt.path))
			if got != want {
				t.Errorf("WorktreeOwner(%q) = %q, want Dir(Dir()) %q", tt.path, got, want)
			}
		}
	}
}

// TestFindWorktreePath probes existing worktrees, legacy base first.
func TestFindWorktreePath(t *testing.T) {
	gitRoot := t.TempDir()
	wt := filepath.Join(gitRoot, ".grove-worktrees", "feature")
	nested := filepath.Join(gitRoot, ".grove-worktrees", "fix", "deep")
	for _, d := range []string{wt, nested} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if got, ok := FindWorktreePath(gitRoot, "feature"); !ok || got != wt {
		t.Errorf("FindWorktreePath(feature) = %q, %v; want %q, true", got, ok, wt)
	}
	// Branch-style names with '/' nest via Join, same as today.
	if got, ok := FindWorktreePath(gitRoot, "fix/deep"); !ok || got != nested {
		t.Errorf("FindWorktreePath(fix/deep) = %q, %v; want %q, true", got, ok, nested)
	}
	if _, ok := FindWorktreePath(gitRoot, "missing"); ok {
		t.Error("FindWorktreePath(missing) = ok, want miss")
	}
}

// TestResolveNewWorktreePath_Legacy pins the legacy join.
func TestResolveNewWorktreePath_Legacy(t *testing.T) {
	gitRoot := "/path/to/eco"
	want := filepath.Join(gitRoot, ".grove-worktrees", "feature")
	if got := ResolveNewWorktreePath(gitRoot, "feature", false); got != want {
		t.Errorf("ResolveNewWorktreePath(legacy) = %q, want %q", got, want)
	}
	// Branch-style names nest.
	want = filepath.Join(gitRoot, ".grove-worktrees", "fix", "deep")
	if got := ResolveNewWorktreePath(gitRoot, "fix/deep", false); got != want {
		t.Errorf("ResolveNewWorktreePath(legacy nested) = %q, want %q", got, want)
	}
}

// TestResolveNewWorktreePath_XDG pins the XDG target shape:
// WorktreesDir()/<DirIdentifier(gitRoot)>/<name>.
func TestResolveNewWorktreePath_XDG(t *testing.T) {
	// Sandbox: GROVE_HOME beats XDG_DATA_HOME in getDataHome, clear it.
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("GROVE_HOME", "")

	gitRoot := "/path/to/eco"
	want := filepath.Join(paths.WorktreesDir(), DirIdentifier(gitRoot), "feature")
	if got := ResolveNewWorktreePath(gitRoot, "feature", true); got != want {
		t.Errorf("ResolveNewWorktreePath(xdg) = %q, want %q", got, want)
	}
	if !strings.HasPrefix(want, os.Getenv("XDG_DATA_HOME")+string(filepath.Separator)) {
		t.Errorf("XDG target %q escaped the sandboxed data home", want)
	}
}

// TestDirIdentifier pins shape, stability, and collision safety.
func TestDirIdentifier(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("GROVE_HOME", "")

	id := DirIdentifier("/path/to/my-ecosystem")
	if !strings.HasPrefix(id, "my-ecosystem-") {
		t.Errorf("DirIdentifier = %q, want sanitized basename prefix", id)
	}
	suffix := strings.TrimPrefix(id, "my-ecosystem-")
	if len(suffix) != 8 {
		t.Errorf("DirIdentifier hash suffix = %q, want 8 hex chars", suffix)
	}
	// Stable across calls.
	if id2 := DirIdentifier("/path/to/my-ecosystem"); id2 != id {
		t.Errorf("DirIdentifier not stable: %q vs %q", id, id2)
	}
	// Two same-basename roots must get distinct identifiers.
	other := DirIdentifier("/other/clone/my-ecosystem")
	if other == id {
		t.Errorf("DirIdentifier collision: %q for both clones", id)
	}
}

// TestWorktreeRootForPath pins the legacy extraction used by the zombie guard.
func TestWorktreeRootForPath(t *testing.T) {
	tests := []struct {
		path   string
		want   string
		wantOK bool
	}{
		{"/repo/.grove-worktrees/feat", "/repo/.grove-worktrees/feat", true},
		{"/repo/.grove-worktrees/feat/.grove/rules", "/repo/.grove-worktrees/feat", true},
		{"/repo/.grove-worktrees", "", false},
		{"/repo/src", "", false},
	}
	for _, tt := range tests {
		got, ok := worktreeRootForPath(tt.path)
		if ok != tt.wantOK || got != tt.want {
			t.Errorf("worktreeRootForPath(%q) = %q, %v; want %q, %v", tt.path, got, ok, tt.want, tt.wantOK)
		}
	}
}
