package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func satelliteSeed() ConfigSeed {
	return ConfigSeed{
		Provenance:  "Written by `grove satellite up` for satellite \"vm1\".",
		MachineName: "vm1",
		Ecosystems: map[string]MachineEcosystem{
			"grovetools": {
				Path:        "~/code/grovetools",
				Notebook:    "grovetools",
				Description: "Grove ecosystem (satellite)",
			},
		},
		Notebooks:    map[string]string{"grovetools": "~/notebooks/grovetools"},
		DaemonSSH:    true,
		LegacyGroves: true,
		Sync: &SyncSeed{
			Server:       "http://127.0.0.1:8788",
			TokenCommand: "cat ~/.config/grove/sync.token",
			Workspaces: []SyncWorkspace{
				{Name: "cloud", Role: SyncRolePeer, Pull: true},
				{Name: "grovetools", Role: SyncRolePeer, Pull: true},
			},
		},
		Dirs: []string{"notebooks/grovetools/workspaces/cloud/inbox"},
	}
}

func TestConfigSeedRendersTheThreeConfigFiles(t *testing.T) {
	files, err := satelliteSeed().Files()
	if err != nil {
		t.Fatalf("Files: %v", err)
	}
	got := map[string]SeedFile{}
	for _, f := range files {
		got[f.Name] = f
	}
	for _, name := range []string{SeedFileGroveTOML, SeedFileMachineTOML, SeedFileSyncTOML} {
		if _, ok := got[name]; !ok {
			t.Fatalf("seed did not render %s (rendered %v)", name, files)
		}
	}
	if mode := got[SeedFileSyncTOML].Mode.Perm(); mode != 0o600 {
		t.Errorf("sync.toml mode = %o, want 600 (it names how to get a token)", mode)
	}
	if mode := got[SeedFileGroveTOML].Mode.Perm(); mode != 0o644 {
		t.Errorf("grove.toml mode = %o, want 644", mode)
	}

	// machine.toml carries the intent; grove.toml carries host topology plus
	// the migration-window [groves.*] mirror.
	if !strings.Contains(got[SeedFileMachineTOML].Content, "[machine.ecosystems.grovetools]") {
		t.Errorf("machine.toml missing the ecosystem subscription:\n%s", got[SeedFileMachineTOML].Content)
	}
	if !strings.Contains(got[SeedFileGroveTOML].Content, "[groves.grovetools]") {
		t.Errorf("grove.toml missing the LegacyGroves mirror:\n%s", got[SeedFileGroveTOML].Content)
	}
	if !strings.Contains(got[SeedFileGroveTOML].Content, "[notebooks.definitions.grovetools]") {
		t.Errorf("grove.toml missing the notebook definition:\n%s", got[SeedFileGroveTOML].Content)
	}
	if !strings.Contains(got[SeedFileGroveTOML].Content, "[daemon.ssh]") {
		t.Errorf("grove.toml missing [daemon.ssh]:\n%s", got[SeedFileGroveTOML].Content)
	}
}

func TestConfigSeedOmitsTheLegacyGrovesMirrorWhenNotAsked(t *testing.T) {
	seed := satelliteSeed()
	seed.LegacyGroves = false
	files, err := seed.Files()
	if err != nil {
		t.Fatalf("Files: %v", err)
	}
	for _, f := range files {
		if f.Name == SeedFileGroveTOML && strings.Contains(f.Content, "[groves.") {
			t.Fatalf("LegacyGroves=false still emitted a [groves.*] mirror:\n%s", f.Content)
		}
	}
}

// The whole point of routing sync.toml through the shared editor: a seed
// cannot smuggle a pull-enabled entry past the push-only invariant, which the
// bootstrap's old `printf` heredoc could and did.
func TestConfigSeedRefusesPullUnderAPushOnlyRole(t *testing.T) {
	for _, role := range []string{"", SyncRoleSatellite} {
		seed := satelliteSeed()
		seed.Sync.Workspaces = []SyncWorkspace{{Name: "cloud", Role: role, Pull: true}}
		if _, err := seed.Files(); err == nil {
			t.Fatalf("role %q with pull=true was accepted; the push-only invariant must refuse it", role)
		}
	}
}

func TestConfigSeedRendersDeterministically(t *testing.T) {
	first, err := satelliteSeed().Bundle()
	if err != nil {
		t.Fatalf("Bundle: %v", err)
	}
	for i := 0; i < 5; i++ {
		again, err := satelliteSeed().Bundle()
		if err != nil {
			t.Fatalf("Bundle: %v", err)
		}
		if again != first {
			t.Fatalf("bundle is not deterministic:\n--- first ---\n%s\n--- again ---\n%s", first, again)
		}
	}
}

func TestConfigSeedBundleFraming(t *testing.T) {
	bundle, err := satelliteSeed().Bundle()
	if err != nil {
		t.Fatalf("Bundle: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(bundle, "\n"), "\n")
	if lines[0] != BundleVersion {
		t.Fatalf("bundle does not start with the version line: %q", lines[0])
	}
	var sawDir, sawFile bool
	for _, l := range lines[1:] {
		switch {
		case strings.HasPrefix(l, "#!dir "):
			sawDir = true
			if sawFile {
				t.Fatalf("a #!dir line appeared after a #!file line — the unpacker reads dirs first")
			}
		case strings.HasPrefix(l, "#!file "):
			sawFile = true
			parts := strings.Fields(l)
			if len(parts) != 3 {
				t.Fatalf("malformed file header: %q", l)
			}
			if err := validateSeedFileName(parts[1]); err != nil {
				t.Fatalf("bundle names a non-seedable file: %v", err)
			}
			if parts[2] != "600" && parts[2] != "644" {
				t.Fatalf("unexpected mode %q in %q", parts[2], l)
			}
		case strings.HasPrefix(l, "#!"):
			t.Fatalf("unrecognized directive line: %q", l)
		}
	}
	if !sawDir || !sawFile {
		t.Fatalf("bundle carried no dirs (%v) or no files (%v)", sawDir, sawFile)
	}
}

func TestConfigSeedRejectsEscapingDirs(t *testing.T) {
	for _, dir := range []string{"/etc", "~/evil", "notebooks/../../etc", ""} {
		seed := satelliteSeed()
		seed.Dirs = []string{dir}
		if _, err := seed.Bundle(); err == nil {
			t.Errorf("dir %q was accepted; it must be refused", dir)
		}
	}
}

// ApplyConfigSeed is the local half of the same renderer. What it writes must
// load back through the real loaders — that is the property the remote seed
// relies on without being able to check it.
func TestApplyConfigSeedWritesLoadableConfig(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "grove")

	written, err := ApplyConfigSeed(configDir, home, satelliteSeed())
	if err != nil {
		t.Fatalf("ApplyConfigSeed: %v", err)
	}
	if len(written) != 3 {
		t.Fatalf("wrote %d files, want 3: %v", len(written), written)
	}

	mc, err := LoadMachineConfigFrom(filepath.Join(configDir, SeedFileMachineTOML))
	if err != nil {
		t.Fatalf("LoadMachineConfigFrom: %v", err)
	}
	if mc == nil || mc.Machine.Name != "vm1" {
		t.Fatalf("machine.toml did not round-trip: %+v", mc)
	}
	if eco, ok := mc.Machine.Ecosystems["grovetools"]; !ok || eco.Path != "~/code/grovetools" {
		t.Fatalf("ecosystem subscription did not round-trip: %+v", mc.Machine.Ecosystems)
	}

	sc, err := LoadSyncConfigFrom(filepath.Join(configDir, SeedFileSyncTOML))
	if err != nil {
		t.Fatalf("LoadSyncConfigFrom: %v", err)
	}
	if sc == nil || len(sc.Workspaces) != 2 {
		t.Fatalf("sync.toml did not round-trip: %+v", sc)
	}
	for _, ws := range sc.Workspaces {
		if !ws.Pull || ws.Role != SyncRolePeer {
			t.Fatalf("workspace %q lost its role/pull: %+v", ws.Name, ws)
		}
	}

	if _, err := os.Stat(filepath.Join(home, "notebooks/grovetools/workspaces/cloud/inbox")); err != nil {
		t.Fatalf("seeded dir was not created: %v", err)
	}

	info, err := os.Stat(filepath.Join(configDir, SeedFileSyncTOML))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("sync.toml landed with mode %o, want 600", info.Mode().Perm())
	}
}

// A re-seed must converge, including the mode of a file that already exists
// (os.WriteFile honors the mode only on create).
func TestApplyConfigSeedIsIdempotent(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "grove")
	if _, err := ApplyConfigSeed(configDir, home, satelliteSeed()); err != nil {
		t.Fatalf("first: %v", err)
	}
	syncPath := filepath.Join(configDir, SeedFileSyncTOML)
	before, err := os.ReadFile(syncPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(syncPath, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyConfigSeed(configDir, home, satelliteSeed()); err != nil {
		t.Fatalf("second: %v", err)
	}
	after, err := os.ReadFile(syncPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("re-seed changed sync.toml content")
	}
	info, err := os.Stat(syncPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("re-seed did not restore mode 600, got %o", info.Mode().Perm())
	}
}

func TestConfigSeedEmptyRendersNothing(t *testing.T) {
	files, err := ConfigSeed{}.Files()
	if err != nil {
		t.Fatalf("Files: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("empty seed rendered %v", files)
	}
}
