package workflows

import (
	"os"
	"path/filepath"
	"testing"
)

const sampleScript = `export const meta = {
  name: 'grovetools-release-survey',
  description: 'Deep survey of grovetools for treemux MVP release readiness',
  phases: [
    { title: 'Survey', detail: 'parallel deep-dives per subsystem' },
    { title: 'Probe', detail: 'build check, CLI quality, provider audit' },
    { title: 'Critique', detail: 'completeness critic on combined findings' },
  ],
}

const MISSION = ` + "`MISSION { braces } inside template literal`" + `
phase('Survey')
`

func TestParseScriptMeta(t *testing.T) {
	meta := ParseScriptMeta([]byte(sampleScript))
	if meta == nil {
		t.Fatal("expected meta, got nil")
	}
	if meta.Name != "grovetools-release-survey" {
		t.Errorf("Name = %q", meta.Name)
	}
	if meta.Description == "" {
		t.Error("Description empty")
	}
	if len(meta.Phases) != 3 {
		t.Fatalf("expected 3 phases, got %d: %+v", len(meta.Phases), meta.Phases)
	}
	if meta.Phases[0].Title != "Survey" || meta.Phases[0].Detail != "parallel deep-dives per subsystem" {
		t.Errorf("phase 0 = %+v", meta.Phases[0])
	}
	if meta.Phases[2].Title != "Critique" {
		t.Errorf("phase 2 = %+v", meta.Phases[2])
	}
}

func TestParseScriptMeta_NoMeta(t *testing.T) {
	if meta := ParseScriptMeta([]byte("const x = 1\n")); meta != nil {
		t.Errorf("expected nil, got %+v", meta)
	}
}

func TestLoadRunMeta(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "grovetools-release-survey-wf_4650c05a-c39.js")
	if err := os.WriteFile(path, []byte(sampleScript), 0o600); err != nil {
		t.Fatal(err)
	}
	meta := LoadRunMeta(dir, "wf_4650c05a-c39")
	if meta == nil || meta.Name != "grovetools-release-survey" {
		t.Fatalf("meta = %+v", meta)
	}
	if LoadRunMeta(dir, "wf_other") != nil {
		t.Error("expected nil for unknown run")
	}
}
