package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

// sandboxConfig points paths.ConfigDir() at a temp dir via GROVE_HOME.
func sandboxConfig(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GROVE_HOME", home)
	dir := filepath.Join(home, "config", "grove")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	ResetLoadCache()
	t.Cleanup(ResetLoadCache)
	return dir
}

func TestLoadMachineConfigMissingIsNotAnError(t *testing.T) {
	sandboxConfig(t)

	cfg, err := LoadMachineConfig()
	if err != nil {
		t.Fatalf("LoadMachineConfig with no file: %v", err)
	}
	if cfg != nil {
		t.Fatalf("LoadMachineConfig with no file returned %+v, want nil", cfg)
	}
	if got, want := ResolveMachineName(), DefaultMachineName(); got != want {
		t.Fatalf("ResolveMachineName fallback = %q, want hostname %q", got, want)
	}
}

func TestLoadMachineConfigReadsName(t *testing.T) {
	dir := sandboxConfig(t)
	writeFile(t, filepath.Join(dir, "machine.toml"), "[machine]\nname = \"mbp\"\n")

	cfg, err := LoadMachineConfig()
	if err != nil {
		t.Fatalf("LoadMachineConfig: %v", err)
	}
	if cfg == nil || cfg.Machine.Name != "mbp" {
		t.Fatalf("LoadMachineConfig = %+v, want name mbp", cfg)
	}
	if got := ResolveMachineName(); got != "mbp" {
		t.Fatalf("ResolveMachineName = %q, want mbp", got)
	}
}

// The machine-config phase adds subscriptions and bare roots to machine.toml.
// Today's minimal loader must tolerate them rather than fail, so a config
// written for the later phase does not brick this one.
// F5 containment: machine.toml must NOT be picked up by the global `*.toml`
// fragment glob, or its [machine] table would land in Config.Extensions and
// leak into the whole cascade.
func TestMachineTOMLIsExcludedFromTheFragmentGlob(t *testing.T) {
	dir := sandboxConfig(t)
	writeFile(t, filepath.Join(dir, "grove.toml"), "name = \"test\"\n")
	writeFile(t, filepath.Join(dir, "machine.toml"), "[machine]\nname = \"mbp\"\n")

	cfg, err := LoadFromWithLogger(t.TempDir(), logrus.New())
	if err != nil {
		t.Fatalf("LoadFromWithLogger: %v", err)
	}
	if _, leaked := cfg.Extensions["machine"]; leaked {
		t.Fatalf("machine.toml leaked into Config.Extensions: %v", cfg.Extensions)
	}

	ResetLoadCache()
	layered, err := LoadLayered(t.TempDir())
	if err != nil {
		t.Fatalf("LoadLayered: %v", err)
	}
	for _, frag := range layered.GlobalFragments {
		if filepath.Base(frag.Path) == "machine.toml" {
			t.Fatalf("LoadLayered loaded machine.toml as a fragment: %s", frag.Path)
		}
	}
}

// The dead ~/.config/grove/machines/ directory is reported by QUERY, so the
// surfaces an operator asks (`grove machine`, `grove doctor`) can name it
// without any config load logging anything.
func TestLegacyMachinesDirReportsTheDeadDirectory(t *testing.T) {
	dir := sandboxConfig(t)
	writeFile(t, filepath.Join(dir, "grove.toml"), "name = \"test\"\n")
	machinesDir := filepath.Join(dir, LegacyMachinesDirName)
	if err := os.MkdirAll(machinesDir, 0o755); err != nil {
		t.Fatalf("mkdir machines: %v", err)
	}
	writeFile(t, filepath.Join(machinesDir, "old.toml"), "name = \"leaked\"\n")

	if got := LegacyMachinesDir(); got != machinesDir {
		t.Fatalf("LegacyMachinesDir() = %q, want %q", got, machinesDir)
	}

	// Reporting it is ALL that happens — nothing under machines/ is loaded.
	cfg, err := LoadFromWithLogger(t.TempDir(), logrus.New())
	if err != nil {
		t.Fatalf("LoadFromWithLogger: %v", err)
	}
	if cfg.Name == "leaked" {
		t.Fatal("machines/ contents were loaded into the config")
	}
}

func TestLegacyMachinesDirIgnoresAbsentDirAndPlainFile(t *testing.T) {
	dir := sandboxConfig(t)

	if got := LegacyMachinesDir(); got != "" {
		t.Fatalf("LegacyMachinesDir() with no machines/ = %q, want empty", got)
	}

	// A FILE named machines (not a directory) is not the dead directory.
	writeFile(t, filepath.Join(dir, LegacyMachinesDirName), "")
	if got := LegacyMachinesDir(); got != "" {
		t.Fatalf("a plain file named %q reported as the dead dir: %q", LegacyMachinesDirName, got)
	}
}

// The regression this whole seam exists for: the dead directory is a standing
// condition, so config load — which every grove binary performs, many times a
// run — must not report it at all. Warning once per PROCESS was still one log
// line per invocation, which spammed the workspace logs with hundreds of
// identical WARNING lines.
func TestConfigLoadNeverReportsLegacyMachinesDir(t *testing.T) {
	dir := sandboxConfig(t)
	writeFile(t, filepath.Join(dir, "grove.toml"), "name = \"test\"\n")
	machinesDir := filepath.Join(dir, LegacyMachinesDirName)
	if err := os.MkdirAll(machinesDir, 0o755); err != nil {
		t.Fatalf("mkdir machines: %v", err)
	}

	resetWarningsForTest()
	t.Cleanup(resetWarningsForTest)
	var sunk []Warning
	SetWarningSink(func(w Warning) { sunk = append(sunk, w) })

	logger, warnings := loggerCapturingWarnings()
	for i := 0; i < 3; i++ {
		if _, err := LoadFromWithLogger(t.TempDir(), logger); err != nil {
			t.Fatalf("LoadFromWithLogger (load %d): %v", i+1, err)
		}
		ResetLoadCache()
	}

	for _, w := range sunk {
		if strings.Contains(w.Message, LegacyMachinesDirName) {
			t.Fatalf("config load reported the standing condition on the warning channel: %q", w.Message)
		}
	}
	for _, w := range *warnings {
		if strings.Contains(w, LegacyMachinesDirName) {
			t.Fatalf("config load reported the standing condition on the console: %q", w)
		}
	}
}

func TestWriteMachineNameRefusesInvalidCandidateWithoutChangingFile(t *testing.T) {
	dir := sandboxConfig(t)
	path := filepath.Join(dir, "machine.toml")
	original := "[machine]\nname = \"old\"\n\n[broken\n"
	writeFile(t, path, original)

	changed, err := WriteMachineName(path, "new")
	if err == nil || !strings.Contains(err.Error(), "result would not reload") {
		t.Fatalf("changed=%t err=%v, want pre-persist reload refusal", changed, err)
	}
	if changed {
		t.Fatal("a refused atomic write must report no change")
	}
	if got := readFile(t, path); got != original {
		t.Fatalf("invalid candidate was persisted:\n%s", got)
	}
}

func TestMachineConfigValidateRejectsBadNames(t *testing.T) {
	for _, name := range []string{" padded", "trailing ", "two\nlines"} {
		cfg := MachineConfig{Machine: MachineSettings{Name: name}}
		if err := cfg.Validate(); err == nil {
			t.Errorf("Validate accepted %q", name)
		}
	}
	ok := MachineConfig{Machine: MachineSettings{Name: "mbp-2"}}
	if err := ok.Validate(); err != nil {
		t.Errorf("Validate rejected a good name: %v", err)
	}
}

func loggerCapturingWarnings() (*logrus.Logger, *[]string) {
	logger := logrus.New()
	logger.SetOutput(os.NewFile(0, os.DevNull))
	hook := &warnHook{}
	logger.AddHook(hook)
	return logger, &hook.messages
}

type warnHook struct{ messages []string }

func (h *warnHook) Levels() []logrus.Level { return logrus.AllLevels }

func (h *warnHook) Fire(e *logrus.Entry) error {
	if e.Level <= logrus.WarnLevel {
		h.messages = append(h.messages, e.Message)
	}
	return nil
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
