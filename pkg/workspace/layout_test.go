package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grovetools/core/pkg/paths"
)

// sandboxXDG isolates a test from the host grove data dir. GROVE_HOME must
// be cleared explicitly — it beats XDG_DATA_HOME in paths.getDataHome().
func sandboxXDG(t *testing.T) string {
	t.Helper()
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("GROVE_HOME", "")
	return dataHome
}

// TestWorktreeBases pins the Phase-3 contract: ordered legacy-first pair of
// identifier-level base directories.
func TestWorktreeBases(t *testing.T) {
	sandboxXDG(t)

	gitRoot := "/path/to/my-ecosystem"
	got := WorktreeBases(gitRoot)
	want := []string{
		filepath.Join(gitRoot, ".grove-worktrees"),
		filepath.Join(paths.WorktreesDir(), DirIdentifier(gitRoot)),
	}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("WorktreeBases(%q) = %v, want %v", gitRoot, got, want)
	}
}

// TestIsWorktreePath_Legacy pins the anchored legacy semantics: a full
// .grove-worktrees path component, not a substring.
func TestIsWorktreePath_Legacy(t *testing.T) {
	sandboxXDG(t)

	tests := []struct {
		path string
		want bool
	}{
		{"/path/to/eco/.grove-worktrees/feature", true},
		{"/path/to/eco/.grove-worktrees/feature/sub/dir", true},
		{"/path/to/eco/.grove-worktrees", true}, // base dir keeps the legacy substring behavior
		{"/path/to/eco", false},
		{"/path/to/.grove-worktreesX/feature", false}, // anchored: near-miss components don't match
		{"/path/to/X.grove-worktrees/feature", false},
		{"", false},
		{"/", false},
	}
	for _, tt := range tests {
		if got := IsWorktreePath(tt.path); got != tt.want {
			t.Errorf("IsWorktreePath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

// TestIsWorktreePath_XDG pins the XDG semantics: strictly under
// WorktreesDir()/<identifier>/; the base and identifier dirs are containers.
func TestIsWorktreePath_XDG(t *testing.T) {
	sandboxXDG(t)

	base := paths.WorktreesDir()
	id := DirIdentifier("/path/to/eco")
	tests := []struct {
		path string
		want bool
	}{
		{filepath.Join(base, id, "feature"), true},
		{filepath.Join(base, id, "feature", "sub", "dir"), true},
		{filepath.Join(base, id, "fix", "deep"), true}, // branch-style nesting
		{base, false},                    // the XDG base is a container, not a worktree
		{filepath.Join(base, id), false}, // identifier dirs are containers, not worktrees
		{filepath.Dir(base), false},      // DataDir()
	}
	for _, tt := range tests {
		if got := IsWorktreePath(tt.path); got != tt.want {
			t.Errorf("IsWorktreePath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

// TestIsWorktreePath_CxCarveOut pins the DataDir()/cx carve-out: cx-internal
// commit-keyed checkouts contain the legacy literal AND live under the data
// root, but are NOT grove worktrees.
func TestIsWorktreePath_CxCarveOut(t *testing.T) {
	sandboxXDG(t)

	cxRoot := filepath.Join(paths.DataDir(), "cx")
	tests := []struct {
		path string
		want bool
	}{
		{filepath.Join(cxRoot, "repos", "my-repo", ".grove-worktrees", "abc123def456"), false},
		{filepath.Join(cxRoot, "repos", "my-repo", ".grove-worktrees", "abc123def456", "src"), false},
		{cxRoot, false},
		// The same shape outside DataDir()/cx is a worktree.
		{"/somewhere/else/repos/my-repo/.grove-worktrees/abc123def456", true},
	}
	for _, tt := range tests {
		if got := IsWorktreePath(tt.path); got != tt.want {
			t.Errorf("IsWorktreePath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

// TestWorktreeOwner_LegacyFallback pins legacy compatibility: for
// legacy-shaped paths without a parsable .git file (including zombie
// worktrees), the owner is exactly Dir(Dir(path)).
func TestWorktreeOwner_LegacyFallback(t *testing.T) {
	sandboxXDG(t)

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

// TestWorktreeOwner_GitdirParse pins the primary mechanism: the worktree's
// .git file gitdir pointer names the owner in any layout.
func TestWorktreeOwner_GitdirParse(t *testing.T) {
	sandboxXDG(t)

	owner := t.TempDir()

	t.Run("legacy relative gitdir", func(t *testing.T) {
		wt := filepath.Join(owner, ".grove-worktrees", "feature")
		if err := os.MkdirAll(wt, 0o755); err != nil {
			t.Fatal(err)
		}
		gitFile := "gitdir: " + filepath.Join("..", "..", ".git", "worktrees", "feature") + "\n"
		if err := os.WriteFile(filepath.Join(wt, ".git"), []byte(gitFile), 0o644); err != nil {
			t.Fatal(err)
		}
		got, ok := WorktreeOwner(wt)
		if !ok || got != owner {
			t.Errorf("WorktreeOwner(legacy) = %q, %v; want %q, true", got, ok, owner)
		}
	})

	t.Run("xdg absolute gitdir", func(t *testing.T) {
		wt := ResolveNewWorktreePath(owner, "feature", true)
		if err := os.MkdirAll(wt, 0o755); err != nil {
			t.Fatal(err)
		}
		gitFile := "gitdir: " + filepath.Join(owner, ".git", "worktrees", "feature") + "\n"
		if err := os.WriteFile(filepath.Join(wt, ".git"), []byte(gitFile), 0o644); err != nil {
			t.Fatal(err)
		}
		got, ok := WorktreeOwner(wt)
		if !ok || got != owner {
			t.Errorf("WorktreeOwner(xdg) = %q, %v; want %q, true", got, ok, owner)
		}
		// A path deep inside the worktree resolves the same owner.
		deep := filepath.Join(wt, "src", "pkg")
		if err := os.MkdirAll(deep, 0o755); err != nil {
			t.Fatal(err)
		}
		got, ok = WorktreeOwner(deep)
		if !ok || got != owner {
			t.Errorf("WorktreeOwner(xdg deep) = %q, %v; want %q, true", got, ok, owner)
		}
	})

	t.Run("xdg bare-owner gitdir", func(t *testing.T) {
		bare := filepath.Join(owner, "bare-repo")
		wt := ResolveNewWorktreePath(bare, "wt1", true)
		if err := os.MkdirAll(wt, 0o755); err != nil {
			t.Fatal(err)
		}
		gitFile := "gitdir: " + filepath.Join(bare, "worktrees", "wt1") + "\n"
		if err := os.WriteFile(filepath.Join(wt, ".git"), []byte(gitFile), 0o644); err != nil {
			t.Fatal(err)
		}
		got, ok := WorktreeOwner(wt)
		if !ok || got != bare {
			t.Errorf("WorktreeOwner(bare) = %q, %v; want %q, true", got, ok, bare)
		}
	})

	t.Run("xdg zombie resolves via marker owner", func(t *testing.T) {
		wt := ResolveNewWorktreePath(owner, "zombie", true)
		if err := os.MkdirAll(filepath.Join(wt, ".grove"), 0o755); err != nil {
			t.Fatal(err)
		}
		// No .git file and no legacy shape: the .grove/workspace marker's
		// owner: key (written at creation since Phase 4) names the owner.
		marker := "branch: zombie\nplan: \nowner: " + owner + "\necosystem: true\nrepos:\n  - sub\n"
		if err := os.WriteFile(filepath.Join(wt, ".grove", "workspace"), []byte(marker), 0o644); err != nil {
			t.Fatal(err)
		}
		got, ok := WorktreeOwner(wt)
		if !ok || got != owner {
			t.Errorf("WorktreeOwner(xdg zombie with marker) = %q, %v; want %q, true", got, ok, owner)
		}
	})

	t.Run("xdg zombie without marker has no owner", func(t *testing.T) {
		wt := ResolveNewWorktreePath(owner, "zombie-bare", true)
		if err := os.MkdirAll(wt, 0o755); err != nil {
			t.Fatal(err)
		}
		// No .git file, no marker, no legacy shape: resolution fails.
		if got, ok := WorktreeOwner(wt); ok {
			t.Errorf("WorktreeOwner(xdg zombie) = %q, true; want miss", got)
		}
	})
}

// TestFindWorktreePath probes existing worktrees in both layouts, legacy
// base first.
func TestFindWorktreePath(t *testing.T) {
	sandboxXDG(t)

	gitRoot := t.TempDir()
	legacyWT := filepath.Join(gitRoot, ".grove-worktrees", "feature")
	nested := filepath.Join(gitRoot, ".grove-worktrees", "fix", "deep")
	xdgWT := ResolveNewWorktreePath(gitRoot, "xdg-only", true)
	for _, d := range []string{legacyWT, nested, xdgWT} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if got, ok := FindWorktreePath(gitRoot, "feature"); !ok || got != legacyWT {
		t.Errorf("FindWorktreePath(feature) = %q, %v; want %q, true", got, ok, legacyWT)
	}
	// Branch-style names with '/' nest via Join, same as today.
	if got, ok := FindWorktreePath(gitRoot, "fix/deep"); !ok || got != nested {
		t.Errorf("FindWorktreePath(fix/deep) = %q, %v; want %q, true", got, ok, nested)
	}
	// XDG-located worktrees are found when the legacy candidate misses.
	if got, ok := FindWorktreePath(gitRoot, "xdg-only"); !ok || got != xdgWT {
		t.Errorf("FindWorktreePath(xdg-only) = %q, %v; want %q, true", got, ok, xdgWT)
	}
	// Legacy wins when a name exists in both layouts.
	bothLegacy := filepath.Join(gitRoot, ".grove-worktrees", "both")
	bothXDG := ResolveNewWorktreePath(gitRoot, "both", true)
	for _, d := range []string{bothLegacy, bothXDG} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if got, ok := FindWorktreePath(gitRoot, "both"); !ok || got != bothLegacy {
		t.Errorf("FindWorktreePath(both) = %q, %v; want legacy %q", got, ok, bothLegacy)
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
	sandboxXDG(t)

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
	sandboxXDG(t)

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
	// Two same-basename roots must get distinct identifiers — a shared XDG
	// subdir would let prune delete the wrong clone's worktrees.
	other := DirIdentifier("/other/clone/my-ecosystem")
	if other == id {
		t.Errorf("DirIdentifier collision: %q for both clones", id)
	}
}

// TestWorktreeRootForPath pins the extraction used by the zombie guard, in
// both layouts.
func TestWorktreeRootForPath(t *testing.T) {
	sandboxXDG(t)

	xdgBase := paths.WorktreesDir()
	id := DirIdentifier("/repo")
	tests := []struct {
		path   string
		want   string
		wantOK bool
	}{
		{"/repo/.grove-worktrees/feat", "/repo/.grove-worktrees/feat", true},
		{"/repo/.grove-worktrees/feat/.grove/rules", "/repo/.grove-worktrees/feat", true},
		{"/repo/.grove-worktrees", "", false},
		{"/repo/src", "", false},
		{filepath.Join(xdgBase, id, "feat"), filepath.Join(xdgBase, id, "feat"), true},
		{filepath.Join(xdgBase, id, "feat", ".grove", "rules"), filepath.Join(xdgBase, id, "feat"), true},
		{filepath.Join(xdgBase, id), "", false},
		{xdgBase, "", false},
	}
	for _, tt := range tests {
		got, ok := worktreeRootForPath(tt.path)
		if ok != tt.wantOK || got != tt.want {
			t.Errorf("worktreeRootForPath(%q) = %q, %v; want %q, %v", tt.path, got, ok, tt.want, tt.wantOK)
		}
	}
}

// TestGetWorktreeName_LegacyOutputsPreserved pins every current legacy
// output of GetWorktreeName across the rewrite that fixed the
// filepath.SplitList misuse.
func TestGetWorktreeName_LegacyOutputsPreserved(t *testing.T) {
	sandboxXDG(t)

	tests := []struct {
		name string
		node WorkspaceNode
		want string
	}{
		{
			name: "non-worktree returns empty",
			node: WorkspaceNode{Kind: KindStandaloneProject, Path: "/p/repo"},
			want: "",
		},
		{
			name: "standalone project worktree",
			node: WorkspaceNode{
				Kind:              KindStandaloneProjectWorktree,
				Path:              "/p/repo/.grove-worktrees/feature",
				ParentProjectPath: "/p/repo",
			},
			want: "feature",
		},
		{
			name: "nested branch-style name returns first component",
			node: WorkspaceNode{
				Kind:              KindStandaloneProjectWorktree,
				Path:              "/p/repo/.grove-worktrees/fix/deep",
				ParentProjectPath: "/p/repo",
			},
			want: "fix",
		},
		{
			name: "ecosystem sub-project worktree",
			node: WorkspaceNode{
				Kind:                KindEcosystemSubProjectWorktree,
				Path:                "/p/eco/sub/.grove-worktrees/wt1",
				ParentProjectPath:   "/p/eco/sub",
				ParentEcosystemPath: "/p/eco",
			},
			want: "wt1",
		},
		{
			name: "ecosystem worktree uses ParentEcosystemPath base",
			node: WorkspaceNode{
				Kind:                KindEcosystemWorktree,
				Path:                "/p/eco/.grove-worktrees/eco-feat",
				ParentProjectPath:   "/p/eco",
				ParentEcosystemPath: "/p/eco",
			},
			want: "eco", // historical behavior: Base of ParentEcosystemPath
		},
		{
			name: "ecosystem worktree without parent uses own path base",
			node: WorkspaceNode{
				Kind: KindEcosystemWorktree,
				Path: "/p/eco/.grove-worktrees/eco-feat",
			},
			want: "eco-feat",
		},
		{
			name: "eco worktree sub-project worktree uses ParentEcosystemPath base",
			node: WorkspaceNode{
				Kind:                KindEcosystemWorktreeSubProjectWorktree,
				Path:                "/p/eco/.grove-worktrees/eco-feat/sub",
				ParentProjectPath:   "/p/eco/sub",
				ParentEcosystemPath: "/p/eco/.grove-worktrees/eco-feat",
			},
			want: "eco-feat",
		},
		{
			name: "path outside any base returns empty",
			node: WorkspaceNode{
				Kind:              KindStandaloneProjectWorktree,
				Path:              "/elsewhere/feature",
				ParentProjectPath: "/p/repo",
			},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.node.GetWorktreeName(); got != tt.want {
				t.Errorf("GetWorktreeName() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestGetWorktreeName_XDG covers worktree nodes whose Path is XDG-located.
func TestGetWorktreeName_XDG(t *testing.T) {
	sandboxXDG(t)

	repo := "/p/repo"
	node := WorkspaceNode{
		Kind:              KindStandaloneProjectWorktree,
		Path:              ResolveNewWorktreePath(repo, "feature", true),
		ParentProjectPath: repo,
	}
	if got := node.GetWorktreeName(); got != "feature" {
		t.Errorf("GetWorktreeName(xdg) = %q, want %q", got, "feature")
	}

	// Branch-style nesting returns the first component, like legacy.
	node.Path = ResolveNewWorktreePath(repo, "fix/deep", true)
	if got := node.GetWorktreeName(); got != "fix" {
		t.Errorf("GetWorktreeName(xdg nested) = %q, want %q", got, "fix")
	}

	// XDG ecosystem worktree: ParentEcosystemPath points at the ORIGINAL
	// checkout (node identity contract), so the historical Base() behavior
	// yields the ecosystem basename, same as legacy.
	eco := WorkspaceNode{
		Kind:                KindEcosystemWorktree,
		Path:                ResolveNewWorktreePath("/p/eco", "eco-feat", true),
		ParentProjectPath:   "/p/eco",
		ParentEcosystemPath: "/p/eco",
	}
	if got := eco.GetWorktreeName(); got != "eco" {
		t.Errorf("GetWorktreeName(xdg eco wt) = %q, want %q", got, "eco")
	}
}
