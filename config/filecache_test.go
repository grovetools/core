package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// benchConfigTOML is a realistic repo-level grove.toml — roughly the shape of
// the file every workspace classification loads (core/grove.toml at the time
// of writing).
const benchConfigTOML = `
name = "core"
description = "Core libraries and debugging tools for the Grove ecosystem"
managed = true

[binary]
name = "core"
path = "./bin/core"

[context]
default_rules = "dev-no-tests"

[tui.nvim_embed]
user_config = true

[[hooks.on_stop]]
name = "Auto-format"
command = "make fmt"
run_if = "changes"

[[hooks.on_stop]]
name = "Validation & Smart E2E"
command = "make check"
run_if = "changes"
cancel_previous = true
timeout = 300

[[test_scopes]]
name = "notebook-resolution"
rules = ".cx/notebook-resolution.rules"
scenarios = ["xdg-worktree-notebook-inheritance"]

[logging]
level = "info"
`

func writeBenchConfig(tb testing.TB) string {
	tb.Helper()
	dir := tb.TempDir()
	path := filepath.Join(dir, "grove.toml")
	if err := os.WriteFile(path, []byte(benchConfigTOML), 0o644); err != nil {
		tb.Fatalf("write config: %v", err)
	}
	return path
}

// BenchmarkLoadCached measures the steady-state cost of Load on an unchanged
// file: the case that dominates in groved, where the same handful of
// grove.toml files are classified thousands of times a day.
func BenchmarkLoadCached(b *testing.B) {
	path := writeBenchConfig(b)
	if _, err := Load(path); err != nil {
		b.Fatalf("warm load: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Load(path); err != nil {
			b.Fatalf("load: %v", err)
		}
	}
}

// BenchmarkLoadCold measures the parse+validate path — what every Load cost
// before the cache existed, and what a genuinely changed file still costs.
func BenchmarkLoadCold(b *testing.B) {
	path := writeBenchConfig(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ResetLoadCache()
		if _, err := Load(path); err != nil {
			b.Fatalf("load: %v", err)
		}
	}
}

// BenchmarkLoadCachedParallel is the groved shape: many goroutines classifying
// workspaces at once. Per-entry locking must not serialize them across
// different files.
func BenchmarkLoadCachedParallel(b *testing.B) {
	const files = 16
	paths := make([]string, files)
	dir := b.TempDir()
	for i := range paths {
		sub := filepath.Join(dir, string(rune('a'+i)))
		if err := os.MkdirAll(sub, 0o755); err != nil {
			b.Fatalf("mkdir: %v", err)
		}
		paths[i] = filepath.Join(sub, "grove.toml")
		if err := os.WriteFile(paths[i], []byte(benchConfigTOML), 0o644); err != nil {
			b.Fatalf("write config: %v", err)
		}
		if _, err := Load(paths[i]); err != nil {
			b.Fatalf("warm load: %v", err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	var counter int64
	var mu sync.Mutex
	b.RunParallel(func(pb *testing.PB) {
		mu.Lock()
		n := counter
		counter++
		mu.Unlock()
		p := paths[n%files]
		for pb.Next() {
			if _, err := Load(p); err != nil {
				b.Fatalf("load: %v", err)
			}
		}
	})
}
