package config

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/grovetools/core/errors"
)

// writeConfigAt writes content and stamps a distinct mtime, so a same-size
// rewrite is still visible to a (mtime, size) cache regardless of how coarse
// the filesystem's timestamps are. Real editors move mtime forward too; the
// explicit Chtimes only removes the test's dependence on wall-clock
// resolution.
func writeConfigAt(t *testing.T, path, content string, age time.Duration) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	stamp := time.Now().Add(age)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
}

func TestLoadServesCachedConfigForUnchangedFile(t *testing.T) {
	ResetLoadCache()
	t.Cleanup(ResetLoadCache)

	path := filepath.Join(t.TempDir(), "grove.toml")
	writeConfigAt(t, path, "name = \"alpha\"\n", -2*time.Second)

	first, err := Load(path)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	second, err := Load(path)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}

	if first != second {
		t.Errorf("unchanged file re-parsed: got a different *Config on the second Load")
	}
	if first.Name != "alpha" {
		t.Errorf("Name = %q, want %q", first.Name, "alpha")
	}
}

func TestLoadRereadsAfterContentChange(t *testing.T) {
	ResetLoadCache()
	t.Cleanup(ResetLoadCache)

	path := filepath.Join(t.TempDir(), "grove.toml")
	writeConfigAt(t, path, "name = \"alpha\"\n", -2*time.Second)

	if cfg, err := Load(path); err != nil || cfg.Name != "alpha" {
		t.Fatalf("first load: cfg=%v err=%v", cfg, err)
	}

	// Same byte count, different content: only the mtime distinguishes them.
	writeConfigAt(t, path, "name = \"bravo\"\n", -1*time.Second)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if cfg.Name != "bravo" {
		t.Errorf("Name = %q after rewrite, want %q — stale cache entry served", cfg.Name, "bravo")
	}
}

func TestLoadRereadsAfterSizeChangeAtSameMtime(t *testing.T) {
	ResetLoadCache()
	t.Cleanup(ResetLoadCache)

	path := filepath.Join(t.TempDir(), "grove.toml")
	writeConfigAt(t, path, "name = \"alpha\"\n", -2*time.Second)
	if _, err := Load(path); err != nil {
		t.Fatalf("first load: %v", err)
	}

	// Pin the mtime and change only the size — the other half of the stamp
	// has to carry it.
	writeConfigAt(t, path, "name = \"alpha-with-a-longer-name\"\n", -2*time.Second)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if cfg.Name != "alpha-with-a-longer-name" {
		t.Errorf("Name = %q, want the rewritten value — size change did not invalidate", cfg.Name)
	}
}

func TestLoadRereadsAfterEnvChange(t *testing.T) {
	ResetLoadCache()
	t.Cleanup(ResetLoadCache)

	path := filepath.Join(t.TempDir(), "grove.toml")
	writeConfigAt(t, path, "name = \"${GROVE_TEST_CACHE_NAME}\"\n", -2*time.Second)

	t.Setenv("GROVE_TEST_CACHE_NAME", "first")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	if cfg.Name != "first" {
		t.Fatalf("Name = %q, want %q", cfg.Name, "first")
	}

	t.Setenv("GROVE_TEST_CACHE_NAME", "second")
	cfg, err = Load(path)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if cfg.Name != "second" {
		t.Errorf("Name = %q, want %q — env change did not invalidate the entry", cfg.Name, "second")
	}
}

func TestLoadRereadsAfterEnvChangeWithDefaultForm(t *testing.T) {
	ResetLoadCache()
	t.Cleanup(ResetLoadCache)

	path := filepath.Join(t.TempDir(), "grove.toml")
	writeConfigAt(t, path, "name = \"${GROVE_TEST_CACHE_DEFAULTED:-fallback}\"\n", -2*time.Second)

	os.Unsetenv("GROVE_TEST_CACHE_DEFAULTED")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	if cfg.Name != "fallback" {
		t.Fatalf("Name = %q, want %q", cfg.Name, "fallback")
	}

	t.Setenv("GROVE_TEST_CACHE_DEFAULTED", "explicit")
	cfg, err = Load(path)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if cfg.Name != "explicit" {
		t.Errorf("Name = %q, want %q — ${VAR:-default} reference not tracked", cfg.Name, "explicit")
	}
}

func TestLoadInvalidatesWhenLegacyMachineConfigAppears(t *testing.T) {
	ResetLoadCache()
	t.Cleanup(ResetLoadCache)
	groveHome := t.TempDir()
	t.Setenv("GROVE_HOME", groveHome)
	configDir := filepath.Join(groveHome, "config", "grove")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "grove.toml")
	writeConfigAt(t, path, "name = \"cached\"\n", -2*time.Second)
	if _, err := Load(path); err != nil {
		t.Fatalf("first load: %v", err)
	}

	machinePath := filepath.Join(configDir, MachineConfigFileName)
	writeConfigAt(t, machinePath, "[machine.ecosystems.old]\npath = \"/code\"\n", -1*time.Second)
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), machinePath) || !strings.Contains(err.Error(), "grove migrate") {
		t.Fatalf("legacy machine appearance remained cached: %v", err)
	}
}

func TestLoadTracksRecordedRoutingPair(t *testing.T) {
	ResetLoadCache()
	t.Cleanup(ResetLoadCache)
	groveHome := t.TempDir()
	t.Setenv("GROVE_HOME", groveHome)
	configDir := filepath.Join(groveHome, "config", "grove")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "grove.toml")
	writeConfigAt(t, path, "name = \"cached\"\n", -4*time.Second)
	cfg, err := Load(path)
	if err != nil || len(cfg.Groves) != 0 {
		t.Fatalf("initial load = %+v, %v", cfg, err)
	}

	np, rp := filepath.Join(configDir, "notebooks.toml"), filepath.Join(configDir, "roots.toml")
	writeConfigAt(t, np, "default = \"nb\"\n[notebooks.nb]\nroot = \"/n1\"\n", -3*time.Second)
	writeConfigAt(t, rp, "[roots.code]\npath = \"/code\"\n", -3*time.Second)
	cfg, err = Load(path)
	if err != nil || cfg.Groves["code"].NotebookRoot != "/n1" {
		t.Fatalf("appearance not observed: %+v, %v", cfg, err)
	}

	// Stamps intentionally use mtime+size: a same-size rewrite at the exact
	// same mtime remains cached until ResetLoadCache, matching self-file cache
	// semantics.
	stamp := stampFile(np)
	writeConfigAt(t, np, "default = \"nb\"\n[notebooks.nb]\nroot = \"/n2\"\n", 0)
	if err := os.Chtimes(np, stamp.modTime, stamp.modTime); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(path)
	if err != nil || cfg.Groves["code"].NotebookRoot != "/n1" {
		t.Fatalf("same-stamp dependency unexpectedly invalidated: %+v, %v", cfg, err)
	}
	ResetLoadCache()
	cfg, err = Load(path)
	if err != nil || cfg.Groves["code"].NotebookRoot != "/n2" {
		t.Fatalf("notebook modification not observed: %+v, %v", cfg, err)
	}

	if err := os.Remove(rp); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(path)
	if err != nil || len(cfg.Groves) != 0 {
		t.Fatalf("roots deletion not observed: %+v, %v", cfg, err)
	}
	if err := os.Remove(np); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(path)
	if err != nil || cfg.Notebooks != nil {
		t.Fatalf("notebooks deletion not observed = %+v, %v", cfg, err)
	}
}

func TestLoadRereadsAfterRootsEnvChange(t *testing.T) {
	ResetLoadCache()
	t.Cleanup(ResetLoadCache)
	groveHome := t.TempDir()
	t.Setenv("GROVE_HOME", groveHome)
	configDir := filepath.Join(groveHome, "config", "grove")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "grove.toml")
	writeConfigAt(t, path, "name = \"cached\"\n", -3*time.Second)
	writeConfigAt(t, filepath.Join(configDir, "notebooks.toml"), "default = \"nb\"\n[notebooks.nb]\nroot = \"/notes\"\n", -2*time.Second)
	writeConfigAt(t, filepath.Join(configDir, "roots.toml"), "[roots.code]\npath = \"${GROVE_TEST_RECORDED_CODE_ROOT}\"\n", -2*time.Second)

	t.Setenv("GROVE_TEST_RECORDED_CODE_ROOT", "/code-one")
	cfg, err := Load(path)
	if err != nil || cfg.Groves["code"].Path != "/code-one" {
		t.Fatalf("first load = %+v, %v", cfg, err)
	}
	t.Setenv("GROVE_TEST_RECORDED_CODE_ROOT", "/code-two")
	cfg, err = Load(path)
	if err != nil || cfg.Groves["code"].Path != "/code-two" {
		t.Fatalf("roots env change remained cached: %+v, %v", cfg, err)
	}
}

func TestLoadRereadsAfterNotebooksEnvChange(t *testing.T) {
	ResetLoadCache()
	t.Cleanup(ResetLoadCache)
	groveHome := t.TempDir()
	t.Setenv("GROVE_HOME", groveHome)
	configDir := filepath.Join(groveHome, "config", "grove")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "grove.toml")
	writeConfigAt(t, path, "name = \"cached\"\n", -3*time.Second)
	writeConfigAt(t, filepath.Join(configDir, "notebooks.toml"), "default = \"nb\"\n[notebooks.nb]\nroot = \"${GROVE_TEST_RECORDED_NOTEBOOK_ROOT}\"\n", -2*time.Second)
	writeConfigAt(t, filepath.Join(configDir, "roots.toml"), "[roots.code]\npath = \"/code\"\n", -2*time.Second)

	t.Setenv("GROVE_TEST_RECORDED_NOTEBOOK_ROOT", "/notes-one")
	cfg, err := Load(path)
	if err != nil || cfg.Groves["code"].NotebookRoot != "/notes-one" {
		t.Fatalf("first load = %+v, %v", cfg, err)
	}
	t.Setenv("GROVE_TEST_RECORDED_NOTEBOOK_ROOT", "/notes-two")
	cfg, err = Load(path)
	if err != nil || cfg.Groves["code"].NotebookRoot != "/notes-two" {
		t.Fatalf("notebooks env change remained cached: %+v, %v", cfg, err)
	}
}

func TestLoadMissingFileErrorIsUnchanged(t *testing.T) {
	ResetLoadCache()
	t.Cleanup(ResetLoadCache)

	path := filepath.Join(t.TempDir(), "absent.toml")

	// Twice: a missing file must not be memoized as anything, and the second
	// call must produce the same error as the first.
	for i := 0; i < 2; i++ {
		cfg, err := Load(path)
		if err == nil {
			t.Fatalf("attempt %d: expected an error for a missing config, got %v", i, cfg)
		}
		if !errors.Is(err, errors.ErrCodeConfigNotFound) {
			t.Errorf("attempt %d: error code = %v, want ErrCodeConfigNotFound", i, err)
		}
	}

	// And a file appearing at that path must be picked up immediately.
	writeConfigAt(t, path, "name = \"appeared\"\n", -1*time.Second)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load after create: %v", err)
	}
	if cfg.Name != "appeared" {
		t.Errorf("Name = %q, want %q", cfg.Name, "appeared")
	}
}

func TestLoadParseErrorIsNotCached(t *testing.T) {
	ResetLoadCache()
	t.Cleanup(ResetLoadCache)

	path := filepath.Join(t.TempDir(), "grove.toml")
	writeConfigAt(t, path, "name = \"unterminated\n", -2*time.Second)

	for i := 0; i < 2; i++ {
		if _, err := Load(path); err == nil {
			t.Fatalf("attempt %d: expected a parse error", i)
		}
	}

	// Fixing the file must take effect without any cache reset.
	writeConfigAt(t, path, "name = \"fixed\"\n", -1*time.Second)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load after fix: %v", err)
	}
	if cfg.Name != "fixed" {
		t.Errorf("Name = %q, want %q", cfg.Name, "fixed")
	}
}

func TestLoadDirectoryPathErrors(t *testing.T) {
	ResetLoadCache()
	t.Cleanup(ResetLoadCache)

	dir := t.TempDir()
	if _, err := Load(dir); err == nil {
		t.Errorf("Load(%q) on a directory returned no error", dir)
	}
}

func TestResetLoadCacheDropsFileEntries(t *testing.T) {
	ResetLoadCache()
	t.Cleanup(ResetLoadCache)

	path := filepath.Join(t.TempDir(), "grove.toml")
	writeConfigAt(t, path, "name = \"alpha\"\n", -2*time.Second)

	first, err := Load(path)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}

	ResetLoadCache()

	second, err := Load(path)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if first == second {
		t.Errorf("ResetLoadCache did not drop the memoized entry: same *Config returned")
	}
	if second.Name != "alpha" {
		t.Errorf("Name = %q, want %q", second.Name, "alpha")
	}
}

// TestLoadWithOverridesDoesNotPoisonCache pins the immutability contract at
// the one place inside the package where a loaded config is fed to a merge.
// mergeConfigs shallow-copies its base and writes through shared nested
// pointers, so a memoized base would let an override file rewrite what every
// other Load caller sees.
func TestLoadWithOverridesDoesNotPoisonCache(t *testing.T) {
	ResetLoadCache()
	t.Cleanup(ResetLoadCache)

	dir := t.TempDir()
	base := filepath.Join(dir, "grove.toml")
	writeConfigAt(t, base, "name = \"alpha\"\n\n[worktree]\nlayout = \"xdg\"\n", -2*time.Second)
	writeConfigAt(t, filepath.Join(dir, "grove.override.toml"), "[worktree]\nlayout = \"legacy\"\n", -2*time.Second)

	cached, err := Load(base)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cached.Worktree == nil || cached.Worktree.Layout != "xdg" {
		t.Fatalf("base layout = %v, want xdg", cached.Worktree)
	}

	merged, err := LoadWithOverrides(base)
	if err != nil {
		t.Fatalf("LoadWithOverrides: %v", err)
	}
	if merged.Worktree == nil || merged.Worktree.Layout != "legacy" {
		t.Errorf("merged layout = %v, want legacy", merged.Worktree)
	}

	after, err := Load(base)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if after.Worktree == nil || after.Worktree.Layout != "xdg" {
		t.Errorf("cached layout = %v after LoadWithOverrides, want xdg — the override leaked into the shared entry", after.Worktree)
	}
}

// TestLoadConcurrent is the -race guard for the per-entry locking: many
// goroutines loading the same and different paths, which is exactly groved's
// workspace-classification fan-out.
func TestLoadConcurrent(t *testing.T) {
	ResetLoadCache()
	t.Cleanup(ResetLoadCache)

	dir := t.TempDir()
	const files = 4
	paths := make([]string, files)
	for i := range paths {
		paths[i] = filepath.Join(dir, string(rune('a'+i))+".toml")
		writeConfigAt(t, paths[i], "name = \""+string(rune('a'+i))+"\"\n", -2*time.Second)
	}

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p := paths[i%files]
			want := string(rune('a' + i%files))
			for j := 0; j < 20; j++ {
				cfg, err := Load(p)
				if err != nil {
					t.Errorf("load %s: %v", p, err)
					return
				}
				if cfg.Name != want {
					t.Errorf("Name = %q, want %q", cfg.Name, want)
					return
				}
			}
		}(i)
	}
	wg.Wait()
}
