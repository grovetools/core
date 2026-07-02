package config

import (
	"os"
	"path/filepath"
	"testing"
)

// setupAuditEnv isolates the config cascade for audit tests: a fake HOME for
// the global layer and a scrubbed GROVE_CONFIG_OVERLAY so the host machine's
// real config can't leak into findings.
func setupAuditEnv(t *testing.T) (globalConfigDir, projectDir string) {
	t.Helper()
	tmpDir := t.TempDir()

	fakeHome := filepath.Join(tmpDir, "home")
	globalConfigDir = filepath.Join(fakeHome, ".config", "grove")
	if err := os.MkdirAll(globalConfigDir, 0o755); err != nil {
		t.Fatal(err)
	}

	projectDir = filepath.Join(tmpDir, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", fakeHome)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("GROVE_CONFIG_OVERLAY", "")
	return globalConfigDir, projectDir
}

// findFinding returns the first finding matching the key, or nil.
func findFinding(findings []AuditFinding, key string) *AuditFinding {
	for i := range findings {
		if findings[i].Key == key {
			return &findings[i]
		}
	}
	return nil
}

// requireClass asserts that a key was found and classified as expected.
func requireClass(t *testing.T, findings []AuditFinding, key string, class AuditClass) *AuditFinding {
	t.Helper()
	f := findFinding(findings, key)
	if f == nil {
		t.Fatalf("expected a finding for key %q, got none (findings: %+v)", key, findings)
	}
	if f.Class != class {
		t.Errorf("key %q: expected class %q, got %q", key, class, f.Class)
	}
	return f
}

// TestAuditClassifications covers each classification bucket across the
// global and project layers.
func TestAuditClassifications(t *testing.T) {
	globalConfigDir, projectDir := setupAuditEnv(t)

	// Global layer: a known core key, an unknown nested key under [tui], a
	// known extension namespace, a deprecated key, and an orphan.
	globalConfig := `
[tui]
theme = "kanagawa"
totally_bogus = "nothing reads this"

[logging]
level = "debug"

[search_paths.main]
path = "/tmp/projects"

[some_orphan]
leftover = true
`
	globalPath := filepath.Join(globalConfigDir, "grove.toml")
	if err := os.WriteFile(globalPath, []byte(globalConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	// Project layer: core scalars plus a known extension.
	projectConfig := `
name = "audit-test"
version = "1.0"

[flow]
default_model = "gemini"
`
	projectPath := filepath.Join(projectDir, "grove.toml")
	if err := os.WriteFile(projectPath, []byte(projectConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	findings, err := Audit(projectDir)
	if err != nil {
		t.Fatalf("Audit failed: %v", err)
	}

	// Global layer classifications.
	f := requireClass(t, findings, "tui.theme", AuditKnownCore)
	if f.Layer != SourceGlobal {
		t.Errorf("tui.theme: expected layer %q, got %q", SourceGlobal, f.Layer)
	}
	if f.File != globalPath {
		t.Errorf("tui.theme: expected file %q, got %q", globalPath, f.File)
	}
	requireClass(t, findings, "tui.totally_bogus", AuditUnknownNested)
	requireClass(t, findings, "logging", AuditKnownExtension)
	requireClass(t, findings, "search_paths", AuditDeprecated)
	requireClass(t, findings, "some_orphan", AuditOrphan)

	// The deprecated and extension namespaces must not be descended into.
	if f := findFinding(findings, "search_paths.main"); f != nil {
		t.Errorf("expected no descent under deprecated search_paths, got %+v", f)
	}
	if f := findFinding(findings, "logging.level"); f != nil {
		t.Errorf("expected no descent under extension logging, got %+v", f)
	}

	// Project layer classifications.
	f = requireClass(t, findings, "name", AuditKnownCore)
	if f.Layer != SourceProject {
		t.Errorf("name: expected layer %q, got %q", SourceProject, f.Layer)
	}
	if f.File != projectPath {
		t.Errorf("name: expected file %q, got %q", projectPath, f.File)
	}
	flowFinding := requireClass(t, findings, "flow", AuditKnownExtension)
	if flowFinding.Layer != SourceProject {
		t.Errorf("flow: expected layer %q, got %q", SourceProject, flowFinding.Layer)
	}
}

// TestAuditNestedCoreStructs verifies descent through nested core structs and
// typed maps: matched leaves are known-core, mismatches are unknown-nested,
// and user-defined map keys (grove names) are not flagged.
func TestAuditNestedCoreStructs(t *testing.T) {
	_, projectDir := setupAuditEnv(t)

	projectConfig := `
version = "1.0"

[groves.personal]
path = "~/code"
description = "personal projects"

[groves.work]
path = "~/work"
not_a_field = true

[daemon]
bogus_daemon_key = 1
`
	if err := os.WriteFile(filepath.Join(projectDir, "grove.toml"), []byte(projectConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	findings, err := Audit(projectDir)
	if err != nil {
		t.Fatalf("Audit failed: %v", err)
	}

	requireClass(t, findings, "groves.personal.path", AuditKnownCore)
	requireClass(t, findings, "groves.personal.description", AuditKnownCore)
	requireClass(t, findings, "groves.work.not_a_field", AuditUnknownNested)
	requireClass(t, findings, "daemon.bogus_daemon_key", AuditUnknownNested)
}

// TestAuditKeybindingsFreeForm verifies that arbitrary package names under
// [tui.keybindings] (consumed by postProcessTOMLKeybindings) are not flagged.
func TestAuditKeybindingsFreeForm(t *testing.T) {
	_, projectDir := setupAuditEnv(t)

	projectConfig := `
version = "1.0"

[tui.keybindings.nb.browser]
create_note = ["n"]
`
	if err := os.WriteFile(filepath.Join(projectDir, "grove.toml"), []byte(projectConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	findings, err := Audit(projectDir)
	if err != nil {
		t.Fatalf("Audit failed: %v", err)
	}

	requireClass(t, findings, "tui.keybindings", AuditKnownCore)
	for _, f := range findings {
		if f.Class == AuditUnknownNested {
			t.Errorf("expected no unknown-nested findings for keybindings, got %+v", f)
		}
	}
}

// TestAuditYAMLLayer verifies raw YAML parsing takes the same classification
// path as TOML (exercised directly against auditFile).
func TestAuditYAMLLayer(t *testing.T) {
	tmpDir := t.TempDir()
	yamlConfig := `
version: "1.0"
tui:
  theme: gruvbox
  totally_bogus: true
logging:
  level: debug
some_orphan: 42
`
	path := filepath.Join(tmpDir, "grove.yml")
	if err := os.WriteFile(path, []byte(yamlConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	findings, err := auditFile(path, SourceProject)
	if err != nil {
		t.Fatalf("auditFile failed: %v", err)
	}

	requireClass(t, findings, "tui.theme", AuditKnownCore)
	requireClass(t, findings, "tui.totally_bogus", AuditUnknownNested)
	requireClass(t, findings, "logging", AuditKnownExtension)
	requireClass(t, findings, "some_orphan", AuditOrphan)
}

// TestAuditRegisterExtension verifies that self-registered extension keys are
// recognized instead of being reported as orphans.
func TestAuditRegisterExtension(t *testing.T) {
	const key = "audit_test_extension"
	if _, exists := KnownExtension(key); exists {
		t.Fatalf("test extension key %q unexpectedly pre-registered", key)
	}
	RegisterExtension(ExtensionInfo{Key: key, Repo: "test", Description: "audit test"})
	defer delete(knownExtensions, key)

	info, ok := KnownExtension(key)
	if !ok {
		t.Fatal("registered extension not found")
	}
	if info.Repo != "test" {
		t.Errorf("expected repo %q, got %q", "test", info.Repo)
	}

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "grove.toml")
	if err := os.WriteFile(path, []byte("[audit_test_extension]\nenabled = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	findings, err := auditFile(path, SourceProject)
	if err != nil {
		t.Fatalf("auditFile failed: %v", err)
	}
	requireClass(t, findings, key, AuditKnownExtension)
}
