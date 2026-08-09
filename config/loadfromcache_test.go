package config

import (
	"os"
	"path/filepath"
	"testing"
)

// setupBenchHierarchy builds the cascade shape the daemon's ResolveTarget
// fan-out pays for: a global config with a fragment, and a project directory
// with a realistic repo-level grove.toml.
func setupBenchHierarchy(b *testing.B) string {
	b.Helper()
	ResetLoadCache()
	b.Cleanup(ResetLoadCache)

	groveHome := b.TempDir()
	b.Setenv("GROVE_HOME", groveHome)
	globalDir := filepath.Join(groveHome, "config", "grove")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		b.Fatalf("mkdir global config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "grove.toml"), []byte("[logging]\nlevel = \"info\"\n"), 0o644); err != nil {
		b.Fatalf("write global config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "extra.toml"), []byte("[context]\ndefault_rules = \"dev\"\n"), 0o644); err != nil {
		b.Fatalf("write global fragment: %v", err)
	}

	projectDir := b.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "grove.toml"), []byte(benchConfigTOML), 0o644); err != nil {
		b.Fatalf("write project config: %v", err)
	}
	return projectDir
}

// BenchmarkLoadFromMemoized measures the steady-state cost of LoadFrom on an
// unchanged hierarchy: revalidating the trace (a few dozen stats plus the
// discovery replays) instead of re-reading, re-parsing, and re-validating the
// whole cascade. This is what groved's per-registry-entry plan-binding
// resolution pays on every refresh — the old 2s TTL cache always missed there,
// because refreshes are ~30s apart.
func BenchmarkLoadFromMemoized(b *testing.B) {
	projectDir := setupBenchHierarchy(b)
	if _, err := LoadFrom(projectDir); err != nil {
		b.Fatalf("warm load: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := LoadFrom(projectDir); err != nil {
			b.Fatalf("load: %v", err)
		}
	}
}

// BenchmarkLoadFromCold measures the full hierarchical load — what every call
// spaced beyond the old TTL paid before the memo existed, and what a genuinely
// changed cascade still costs.
func BenchmarkLoadFromCold(b *testing.B) {
	projectDir := setupBenchHierarchy(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ResetLoadCache()
		if _, err := LoadFrom(projectDir); err != nil {
			b.Fatalf("load: %v", err)
		}
	}
}
