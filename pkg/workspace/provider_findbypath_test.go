package workspace

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// legacyContainmentScan is the containment fallback FindByPath used before the
// ancestor walk: a full scan of pathMap that allocates `key + separator` for
// every entry it tests, keeping the longest match. It is preserved verbatim
// here as the oracle the new implementation must agree with byte-for-byte —
// FindByPath has callers all over the ecosystem, and "longest containing
// workspace wins" is load-bearing for every one of them.
func legacyContainmentScan(pathMap map[string]*WorkspaceNode, normalizedPath string) *WorkspaceNode {
	var bestMatch *WorkspaceNode
	var bestMatchLen int
	for normalizedNodePath, node := range pathMap {
		if strings.HasPrefix(normalizedPath, normalizedNodePath+string(filepath.Separator)) {
			if bestMatch == nil || len(normalizedNodePath) > bestMatchLen {
				bestMatch = node
				bestMatchLen = len(normalizedNodePath)
			}
		}
	}
	return bestMatch
}

// realisticPathMap builds a pathMap shaped like a real developer machine: a
// handful of ecosystems, sub-projects under each, per-sub-project worktrees,
// ecosystem-wide worktrees with their own sub-project copies, plus standalone
// projects and a couple of deliberately adversarial neighbours (a path that is
// a string prefix of another without being a directory ancestor). Keys are
// lowercase so NormalizeForLookup is the identity on case-insensitive hosts.
func realisticPathMap() map[string]*WorkspaceNode {
	m := map[string]*WorkspaceNode{}
	add := func(path string) {
		m[path] = &WorkspaceNode{Name: filepath.Base(path), Path: path}
	}

	for e := 0; e < 6; e++ {
		eco := fmt.Sprintf("/users/dev/code/eco%d", e)
		add(eco)
		for s := 0; s < 12; s++ {
			sub := fmt.Sprintf("%s/proj%d", eco, s)
			add(sub)
			for w := 0; w < 3; w++ {
				add(fmt.Sprintf("%s/.grove-worktrees/feat%d", sub, w))
			}
		}
		for w := 0; w < 4; w++ {
			ecoWt := fmt.Sprintf("%s/.grove-worktrees/plan%d", eco, w)
			add(ecoWt)
			for s := 0; s < 12; s++ {
				add(fmt.Sprintf("%s/proj%d", ecoWt, s))
			}
		}
	}
	for i := 0; i < 40; i++ {
		add(fmt.Sprintf("/users/dev/code/standalone%d", i))
	}

	// Adversarial neighbours: string prefixes that are NOT directory ancestors.
	add("/users/dev/code/eco0-archive")
	add("/users/dev/code/standalone1-old")
	// A shallow key that contains many deeper ones, so "longest wins" has teeth.
	add("/users/dev/code")

	return m
}

// queryCorpus is the set of lookups the two implementations must agree on:
// deep source files (the drawer's actual traffic), directories, near-misses,
// and the degenerate shapes.
func queryCorpus(pathMap map[string]*WorkspaceNode) []string {
	queries := []string{
		"/users/dev/code/eco0/proj3/internal/app/app.go",
		"/users/dev/code/eco0/proj3/.grove-worktrees/feat1/internal/app/app.go",
		"/users/dev/code/eco2/.grove-worktrees/plan1/proj7/pkg/x/y/z.go",
		"/users/dev/code/eco2/.grove-worktrees/plan1/README.md",
		"/users/dev/code/eco0-archive/notes.md",
		"/users/dev/code/standalone1-old/main.go",
		"/users/dev/code/loose-file.txt",
		"/users/dev/notes/plan.md",
		"/etc/hosts",
		"/",
		"",
		"/users",
		"/users/dev/code",
		"/users/dev/code/eco0",
		"/users/dev/code/eco0/proj3",
		// A key with a sibling whose name extends it: must not match proj1 for proj12.
		"/users/dev/code/eco0/proj1x/file.go",
		// Trailing separator, which normalization would normally have cleaned away.
		"/users/dev/code/eco0/proj3/",
	}
	// Every key, plus a file directly inside it, so the strict-prefix boundary
	// is exercised against the whole map rather than a hand-picked corner.
	for k := range pathMap {
		queries = append(queries, k, k+"/file.go", k+"x/file.go")
	}
	return queries
}

// TestFindContainingNodeMatchesLegacyScan is the guard on the optimization: the
// ancestor walk must return the identical node the old full-map scan returned,
// for every query shape, over a pathMap the size of a real machine's.
func TestFindContainingNodeMatchesLegacyScan(t *testing.T) {
	pathMap := realisticPathMap()
	p := &Provider{pathMap: pathMap}

	for _, q := range queryCorpus(pathMap) {
		want := legacyContainmentScan(pathMap, q)
		got := p.findContainingNode(q)
		if got != want {
			t.Errorf("findContainingNode(%q) = %v, legacy scan = %v",
				q, nodePath(got), nodePath(want))
		}
	}
}

// TestFindContainingNodeRootKey pins the one edge the old scan decided by
// accident and the walk decides on purpose: a pathMap key of "/" never matched,
// because `HasPrefix(query, "/"+"/")` is false for any cleaned path. Callers
// depend on a stray root entry NOT swallowing every lookup, so the behavior is
// preserved rather than "fixed".
func TestFindContainingNodeRootKey(t *testing.T) {
	root := &WorkspaceNode{Name: "root", Path: "/"}
	pathMap := map[string]*WorkspaceNode{"/": root}
	p := &Provider{pathMap: pathMap}

	for _, q := range []string{"/etc/hosts", "/users/dev/code/x.go", "/tmp"} {
		if got := p.findContainingNode(q); got != nil {
			t.Errorf("findContainingNode(%q) = %v, want nil (root key must not match)", q, nodePath(got))
		}
		if want := legacyContainmentScan(pathMap, q); want != nil {
			t.Errorf("legacy scan disagrees: %q matched %v", q, nodePath(want))
		}
	}
}

// TestFindContainingNodeLongestWins states the containment rule directly, so a
// future rewrite that drops the leaf-upward walk fails on the RULE and not only
// on the differential comparison above.
func TestFindContainingNodeLongestWins(t *testing.T) {
	shallow := &WorkspaceNode{Name: "eco", Path: "/code/eco"}
	sub := &WorkspaceNode{Name: "proj", Path: "/code/eco/proj"}
	worktree := &WorkspaceNode{Name: "proj", Path: "/code/eco/proj/.grove-worktrees/feat"}
	p := &Provider{pathMap: map[string]*WorkspaceNode{
		shallow.Path:  shallow,
		sub.Path:      sub,
		worktree.Path: worktree,
	}}

	cases := map[string]*WorkspaceNode{
		"/code/eco/proj/.grove-worktrees/feat/pkg/x.go": worktree,
		"/code/eco/proj/pkg/x.go":                       sub,
		"/code/eco/other/pkg/x.go":                      shallow,
		"/code/other/pkg/x.go":                          nil,
		"/code/eco/projx/pkg/x.go":                      shallow,
	}
	for q, want := range cases {
		if got := p.findContainingNode(q); got != want {
			t.Errorf("findContainingNode(%q) = %v, want %v", q, nodePath(got), nodePath(want))
		}
	}
}

// TestFindContainingNodeIsAllocFree is the point of the change: the drawer calls
// this per visible row per frame, and the old scan's `key + separator` made it
// 38% of treemux's CPU (95% of that runtime.concatstring2). A regression here is
// a performance bug even while every correctness test still passes.
func TestFindContainingNodeIsAllocFree(t *testing.T) {
	pathMap := realisticPathMap()
	p := &Provider{pathMap: pathMap}
	const query = "/users/dev/code/eco2/.grove-worktrees/plan1/proj7/pkg/x/y/z.go"

	if got := testing.AllocsPerRun(100, func() { p.findContainingNode(query) }); got != 0 {
		t.Errorf("findContainingNode allocated %v times per call, want 0", got)
	}
}

func nodePath(n *WorkspaceNode) string {
	if n == nil {
		return "<nil>"
	}
	return n.Path
}

func BenchmarkFindContainingNode(b *testing.B) {
	pathMap := realisticPathMap()
	p := &Provider{pathMap: pathMap}
	const query = "/users/dev/code/eco2/.grove-worktrees/plan1/proj7/pkg/x/y/z.go"

	b.Run("walk", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = p.findContainingNode(query)
		}
	})
	b.Run("legacy-scan", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = legacyContainmentScan(pathMap, query)
		}
	})
}
