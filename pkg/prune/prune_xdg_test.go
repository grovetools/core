package prune

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/grovetools/core/pkg/paths"
	"github.com/grovetools/core/pkg/workspace"
)

// sandboxXDG isolates a test from the host grove data dir. GROVE_HOME must
// be cleared explicitly — it beats XDG_DATA_HOME in paths.getDataHome().
func sandboxXDG(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("GROVE_HOME", "")
}

func TestDetectHostWorktreeDirs_XDGOrphans(t *testing.T) {
	sandboxXDG(t)

	gitRoot := t.TempDir()
	xdgDead := workspace.ResolveNewWorktreePath(gitRoot, "dead-xdg", true)
	xdgAlive := workspace.ResolveNewWorktreePath(gitRoot, "alive", true)
	legacyDead := filepath.Join(gitRoot, ".grove-worktrees", "dead-legacy")
	for _, d := range []string{xdgDead, xdgAlive, legacyDead} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	idx := NewSlugIndex([]string{"alive"}, nil)
	orphans, err := DetectHostWorktreeDirs(gitRoot, idx, workspace.WorktreeBases(gitRoot)...)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, o := range orphans {
		found[o.Name] = true
	}
	if len(orphans) != 2 || !found[xdgDead] || !found[legacyDead] {
		t.Fatalf("expected XDG + legacy orphans, got %+v", orphans)
	}
}

func TestRun_DeletesXDGOrphan(t *testing.T) {
	sandboxXDG(t)

	gitRoot := t.TempDir()
	dead := workspace.ResolveNewWorktreePath(gitRoot, "dead", true)
	if err := os.MkdirAll(dead, 0o755); err != nil {
		t.Fatal(err)
	}

	in := Inputs{GitRoot: gitRoot, Active: []string{"alive"}}
	res, err := Run(in, Options{DryRun: false})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Deleted) != 1 || res.Deleted[0].Name != dead {
		t.Fatalf("expected XDG orphan deleted, got deleted=%+v failed=%+v", res.Deleted, res.Failed)
	}
	if _, err := os.Stat(dead); !os.IsNotExist(err) {
		t.Errorf("expected %s removed, stat err=%v", dead, err)
	}
	// The identifier dir and the XDG base must survive the deletion.
	if _, err := os.Stat(filepath.Dir(dead)); err != nil {
		t.Errorf("identifier dir removed: %v", err)
	}
	if _, err := os.Stat(paths.WorktreesDir()); err != nil {
		t.Errorf("XDG worktree base removed: %v", err)
	}
}

// TestRemoveHostPath_XDGRefusalRows pins the destructive-path guard: only
// strict children of the gitRoot and its worktree bases are deletable —
// never the XDG base, the identifier dirs, the grove data dir, or the cx
// checkout area.
func TestRemoveHostPath_XDGRefusalRows(t *testing.T) {
	sandboxXDG(t)

	gitRoot := t.TempDir()
	allowed := append([]string{gitRoot}, workspace.WorktreeBases(gitRoot)...)
	identifierDir := filepath.Join(paths.WorktreesDir(), workspace.DirIdentifier(gitRoot))
	otherIdentifierDir := filepath.Join(paths.WorktreesDir(), workspace.DirIdentifier("/some/other/repo"))

	refuse := []struct {
		name string
		path string
	}{
		{"XDG worktree base", paths.WorktreesDir()},
		{"identifier dir", identifierDir},
		{"another repo's identifier dir child", filepath.Join(otherIdentifierDir, "wt")},
		{"DataDir", paths.DataDir()},
		{"DataDir/cx", filepath.Join(paths.DataDir(), "cx")},
		{"cx checkout", filepath.Join(paths.DataDir(), "cx", "repos", "r", ".grove-worktrees", "abc")},
	}
	for _, tt := range refuse {
		if err := removeHostPath(tt.path, allowed); err == nil {
			t.Errorf("removeHostPath(%s: %q) succeeded, want refusal", tt.name, tt.path)
		}
	}

	// A strict child of the identifier dir IS deletable.
	dead := filepath.Join(identifierDir, "dead")
	if err := os.MkdirAll(dead, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := removeHostPath(dead, allowed); err != nil {
		t.Errorf("removeHostPath(identifier child) = %v, want deletion", err)
	}
	if _, err := os.Stat(dead); !os.IsNotExist(err) {
		t.Errorf("expected %s removed, stat err=%v", dead, err)
	}
}
