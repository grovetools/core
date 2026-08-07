package workspace

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const benchWorkspaceTOML = `
name = "core"
description = "Core libraries and debugging tools for the Grove ecosystem"
managed = true

[binary]
name = "core"
path = "./bin/core"

[context]
default_rules = "dev-no-tests"

[[hooks.on_stop]]
name = "Auto-format"
command = "make fmt"
run_if = "changes"
`

// BenchmarkFindGroveConfigHit is the classification path for a directory that
// HAS a config: the stat probe plus config.Load. It is the measurement behind
// the decision not to add a negative-lookup cache — compare it against
// BenchmarkFindGroveConfigMiss.
func BenchmarkFindGroveConfigHit(b *testing.B) {
	dir := b.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "grove.toml"), []byte(benchWorkspaceTOML), 0o644); err != nil {
		b.Fatalf("write config: %v", err)
	}
	if _, _, err := findGroveConfig(dir); err != nil {
		b.Fatalf("warm: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := findGroveConfig(dir); err != nil {
			b.Fatalf("findGroveConfig: %v", err)
		}
	}
}

// BenchmarkFindGroveConfigMiss is the negative case: a directory with no grove
// config, so all six candidate names are statted and none exists.
func BenchmarkFindGroveConfigMiss(b *testing.B) {
	dir := b.TempDir()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := findGroveConfig(dir); err == nil {
			b.Fatal("expected a miss")
		}
	}
}

// BenchmarkClassifyRealWorkspaceRoots replays groved's plan-stats pass against
// a real machine's workspace roster: one iteration classifies every directory
// under GROVE_PERF_ROOTS that holds a grove.toml, which is exactly what
// FetchPlanStatsMap's DiscoverAll does every few seconds.
//
// It is read-only and skipped unless the roster is named, e.g.
//
//	GROVE_PERF_ROOTS=~/.local/share/grove/worktrees:~/Code/grovetools \
//	  go test ./pkg/workspace -run '^$' -bench ClassifyRealWorkspaceRoots \
//	  -benchtime=5x -cpuprofile cpu.out
//
// A CPU profile of that run is the end-to-end check on config.Load's cache:
// before it, the profile bottoms out in jsonschema/v5 validate frames; after,
// those frames are gone and the pass is dominated by stat syscalls.
func BenchmarkClassifyRealWorkspaceRoots(b *testing.B) {
	spec := os.Getenv("GROVE_PERF_ROOTS")
	if spec == "" {
		b.Skip("set GROVE_PERF_ROOTS=<dir>[:<dir>...] to run against a real workspace roster")
	}

	var roots []string
	for _, root := range strings.Split(spec, ":") {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		if strings.HasPrefix(root, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				b.Fatalf("resolve ~: %v", err)
			}
			root = filepath.Join(home, root[2:])
		}
		roots = append(roots, root)
	}

	var dirs []string
	seen := map[string]struct{}{}
	for _, root := range roots {
		base := strings.Count(filepath.Clean(root), string(os.PathSeparator))
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil //nolint:nilerr // unreadable subtrees are simply not part of the roster
			}
			if !d.IsDir() {
				return nil
			}
			name := d.Name()
			if name == "node_modules" || name == ".git" || (strings.HasPrefix(name, ".") && path != root) {
				return fs.SkipDir
			}
			if strings.Count(path, string(os.PathSeparator))-base > 4 {
				return fs.SkipDir
			}
			if _, statErr := os.Stat(filepath.Join(path, "grove.toml")); statErr != nil {
				return nil
			}
			if _, dup := seen[path]; !dup {
				seen[path] = struct{}{}
				dirs = append(dirs, path)
			}
			return nil
		})
	}
	if len(dirs) == 0 {
		b.Skipf("no grove.toml-bearing directories under %v", roots)
	}
	b.Logf("classifying %d real workspace roots per iteration", len(dirs))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, dir := range dirs {
			// Errors are the point of classification, not a failure: a repo
			// with a broken grove.toml is a legitimate outcome here.
			_, _, _ = classifyWorkspaceRoot(dir)
		}
	}
}
