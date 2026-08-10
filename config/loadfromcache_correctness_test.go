package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// setupLoadFromHierarchy isolates the full cascade under temp dirs: a global
// config dir (reached via GROVE_HOME) and a project directory with its own
// grove.toml. Files are aged with writeConfigAt so later same-size rewrites
// are visible to the (mtime, size) stamps regardless of filesystem timestamp
// resolution.
func setupLoadFromHierarchy(t *testing.T) (projectDir, globalDir string) {
	t.Helper()
	ResetLoadCache()
	t.Cleanup(ResetLoadCache)

	groveHome := t.TempDir()
	t.Setenv("GROVE_HOME", groveHome)
	globalDir = filepath.Join(groveHome, "config", "grove")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("mkdir global config dir: %v", err)
	}
	writeConfigAt(t, filepath.Join(globalDir, "grove.toml"), "build_cmd = \"global\"\n", -2*time.Second)

	projectDir = t.TempDir()
	writeConfigAt(t, filepath.Join(projectDir, "grove.toml"), "name = \"alpha\"\n", -2*time.Second)
	return projectDir, globalDir
}

func TestLoadFromServesCachedConfigForUnchangedHierarchy(t *testing.T) {
	projectDir, _ := setupLoadFromHierarchy(t)

	first, err := LoadFrom(projectDir)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	second, err := LoadFrom(projectDir)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}

	if first != second {
		t.Errorf("unchanged hierarchy re-loaded: got a different *Config on the second LoadFrom")
	}
	if first.Name != "alpha" {
		t.Errorf("Name = %q, want %q", first.Name, "alpha")
	}
	if first.BuildCmd != "global" {
		t.Errorf("BuildCmd = %q, want %q — global layer missing from merge", first.BuildCmd, "global")
	}
}

func TestLoadFromRereadsAfterProjectEdit(t *testing.T) {
	projectDir, _ := setupLoadFromHierarchy(t)

	if cfg, err := LoadFrom(projectDir); err != nil || cfg.Name != "alpha" {
		t.Fatalf("first load: cfg=%v err=%v", cfg, err)
	}

	// Same byte count, different content: only the mtime distinguishes them.
	writeConfigAt(t, filepath.Join(projectDir, "grove.toml"), "name = \"bravo\"\n", -1*time.Second)

	cfg, err := LoadFrom(projectDir)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if cfg.Name != "bravo" {
		t.Errorf("Name = %q after rewrite, want %q — stale entry served", cfg.Name, "bravo")
	}
}

func TestLoadFromRereadsAfterGlobalEdit(t *testing.T) {
	projectDir, globalDir := setupLoadFromHierarchy(t)

	if cfg, err := LoadFrom(projectDir); err != nil || cfg.BuildCmd != "global" {
		t.Fatalf("first load: cfg=%v err=%v", cfg, err)
	}

	writeConfigAt(t, filepath.Join(globalDir, "grove.toml"), "build_cmd = \"edited\"\n", -1*time.Second)

	cfg, err := LoadFrom(projectDir)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if cfg.BuildCmd != "edited" {
		t.Errorf("BuildCmd = %q after global edit, want %q — parent layer change did not invalidate", cfg.BuildCmd, "edited")
	}
}

func TestLoadFromRereadsAfterEcosystemEdit(t *testing.T) {
	_, _ = setupLoadFromHierarchy(t)

	ecoDir := t.TempDir()
	writeConfigAt(t, filepath.Join(ecoDir, "grove.toml"), "build_cmd = \"eco-one\"\nworkspaces = [\"ws\"]\n", -2*time.Second)
	wsDir := filepath.Join(ecoDir, "ws")
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	writeConfigAt(t, filepath.Join(wsDir, "grove.toml"), "name = \"ws\"\n", -2*time.Second)

	if cfg, err := LoadFrom(wsDir); err != nil || cfg.BuildCmd != "eco-one" {
		t.Fatalf("first load: cfg=%v err=%v", cfg, err)
	}

	writeConfigAt(t, filepath.Join(ecoDir, "grove.toml"), "build_cmd = \"eco-two\"\nworkspaces = [\"ws\"]\n", -1*time.Second)

	cfg, err := LoadFrom(wsDir)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if cfg.BuildCmd != "eco-two" {
		t.Errorf("BuildCmd = %q after ecosystem edit, want %q", cfg.BuildCmd, "eco-two")
	}
}

func TestLoadFromRereadsAfterProjectConfigCreated(t *testing.T) {
	_, _ = setupLoadFromHierarchy(t)

	// No config in this directory yet: the search falls through to the global
	// config, so the cascade is anchored there.
	bareDir := t.TempDir()

	cfg, err := LoadFrom(bareDir)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	if cfg.Name != "" {
		t.Fatalf("Name = %q before any project config exists, want empty", cfg.Name)
	}

	// A grove.toml appearing in the directory re-anchors the whole cascade.
	writeConfigAt(t, filepath.Join(bareDir, "grove.toml"), "name = \"appeared\"\n", -1*time.Second)

	cfg, err = LoadFrom(bareDir)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if cfg.Name != "appeared" {
		t.Errorf("Name = %q, want %q — newly created project config not picked up", cfg.Name, "appeared")
	}
}

func TestLoadFromRereadsAfterOverrideCreated(t *testing.T) {
	projectDir, _ := setupLoadFromHierarchy(t)

	if cfg, err := LoadFrom(projectDir); err != nil || cfg.Name != "alpha" {
		t.Fatalf("first load: cfg=%v err=%v", cfg, err)
	}

	// The override candidates were checked and absent on the first load; one
	// appearing must invalidate the entry.
	writeConfigAt(t, filepath.Join(projectDir, "grove.override.toml"), "name = \"override\"\n", -1*time.Second)

	cfg, err := LoadFrom(projectDir)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if cfg.Name != "override" {
		t.Errorf("Name = %q, want %q — newly created override not picked up", cfg.Name, "override")
	}
}

func TestLoadFromRereadsAfterGlobalFragmentCreated(t *testing.T) {
	projectDir, globalDir := setupLoadFromHierarchy(t)

	if cfg, err := LoadFrom(projectDir); err != nil || cfg.BuildCmd != "global" {
		t.Fatalf("first load: cfg=%v err=%v", cfg, err)
	}

	// Fragment glob membership is part of the trace: a new *.toml in the
	// global config dir must invalidate.
	writeConfigAt(t, filepath.Join(globalDir, "extra.toml"), "build_cmd = \"frag\"\n", -1*time.Second)

	cfg, err := LoadFrom(projectDir)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if cfg.BuildCmd != "frag" {
		t.Errorf("BuildCmd = %q, want %q — new global fragment not picked up", cfg.BuildCmd, "frag")
	}
}

func TestLoadFromRereadsAfterMachineConfigChange(t *testing.T) {
	projectDir, globalDir := setupLoadFromHierarchy(t)

	cfg, err := LoadFrom(projectDir)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	if _, ok := cfg.Groves["subscribed"]; ok {
		t.Fatalf("grove %q present before machine.toml was written", "subscribed")
	}

	// compileMachineGroves folds machine.toml's subscriptions into every
	// loaded config, so its appearance has to invalidate the entry.
	writeConfigAt(t, filepath.Join(globalDir, "machine.toml"), "[machine]\n\n[machine.ecosystems.subscribed]\npath = \"/tmp/subscribed\"\n", -1*time.Second)

	cfg, err = LoadFrom(projectDir)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if _, ok := cfg.Groves["subscribed"]; !ok {
		t.Errorf("grove %q missing after machine.toml appeared; groves=%v", "subscribed", cfg.Groves)
	}
}

func TestLoadFromTracksRecordedRoutingPair(t *testing.T) {
	projectDir, globalDir := setupLoadFromHierarchy(t)
	cfg, err := LoadFrom(projectDir)
	if err != nil || len(cfg.Groves) != 0 {
		t.Fatalf("initial load = %+v, %v", cfg, err)
	}
	np, rp := filepath.Join(globalDir, "notebooks.toml"), filepath.Join(globalDir, "roots.toml")
	writeConfigAt(t, np, "default = \"nb\"\n[notebooks.nb]\nroot = \"/notes-a\"\n", -2*time.Second)
	writeConfigAt(t, rp, "[roots.code]\npath = \"/code\"\n", -2*time.Second)
	cfg, err = LoadFrom(projectDir)
	if err != nil || cfg.Groves["code"].NotebookRoot != "/notes-a" {
		t.Fatalf("recorded appearance not observed: %+v, %v", cfg, err)
	}
	writeConfigAt(t, np, "default = \"nb\"\n[notebooks.nb]\nroot = \"/notes-bb\"\n", -1*time.Second)
	cfg, err = LoadFrom(projectDir)
	if err != nil || cfg.Groves["code"].NotebookRoot != "/notes-bb" {
		t.Fatalf("recorded modification not observed: %+v, %v", cfg, err)
	}
	if err := os.Remove(rp); err != nil {
		t.Fatal(err)
	}
	cfg, err = LoadFrom(projectDir)
	if err != nil || len(cfg.Groves) != 0 {
		t.Fatalf("recorded deletion not observed: %+v, %v", cfg, err)
	}
}

func TestLoadFromRereadsAfterEnvChange(t *testing.T) {
	projectDir, _ := setupLoadFromHierarchy(t)
	writeConfigAt(t, filepath.Join(projectDir, "grove.toml"), "name = \"${GROVE_TEST_LOADFROM_NAME}\"\n", -2*time.Second)

	t.Setenv("GROVE_TEST_LOADFROM_NAME", "first")
	cfg, err := LoadFrom(projectDir)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	if cfg.Name != "first" {
		t.Fatalf("Name = %q, want %q", cfg.Name, "first")
	}

	t.Setenv("GROVE_TEST_LOADFROM_NAME", "second")
	cfg, err = LoadFrom(projectDir)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if cfg.Name != "second" {
		t.Errorf("Name = %q, want %q — env change did not invalidate the entry", cfg.Name, "second")
	}
}

func TestResetLoadCacheDropsHierarchyEntries(t *testing.T) {
	projectDir, _ := setupLoadFromHierarchy(t)

	first, err := LoadFrom(projectDir)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}

	ResetLoadCache()

	second, err := LoadFrom(projectDir)
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

// TestLoadFromConcurrent is the -race guard for the per-entry locking: many
// goroutines loading the same and different directories, which is exactly the
// daemon's per-registry-entry ResolveTarget fan-out.
func TestLoadFromConcurrent(t *testing.T) {
	projectDir, _ := setupLoadFromHierarchy(t)

	otherDir := t.TempDir()
	writeConfigAt(t, filepath.Join(otherDir, "grove.toml"), "name = \"other\"\n", -2*time.Second)

	dirs := []string{projectDir, otherDir}
	wants := []string{"alpha", "other"}

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				cfg, err := LoadFrom(dirs[i%2])
				if err != nil {
					t.Errorf("load %s: %v", dirs[i%2], err)
					return
				}
				if cfg.Name != wants[i%2] {
					t.Errorf("Name = %q, want %q", cfg.Name, wants[i%2])
					return
				}
			}
		}(i)
	}
	wg.Wait()
}
