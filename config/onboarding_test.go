package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestOnboardingTOMLRoundTrip: the [onboarding] section parses from TOML with
// both fields typed, and a config without the section reads as not-completed
// (nil pointer — all consumers are nil-safe by contract).
func TestOnboardingTOMLRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "grove.toml")
	content := `[onboarding]
completed = true
last_step = "keys"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Onboarding == nil {
		t.Fatal("Onboarding section not parsed")
	}
	if !cfg.Onboarding.Completed {
		t.Error("Completed = false, want true")
	}
	if cfg.Onboarding.LastStep != "keys" {
		t.Errorf("LastStep = %q, want keys", cfg.Onboarding.LastStep)
	}

	// No section at all: nil pointer, which readers treat as not-completed.
	bare := filepath.Join(dir, "bare.toml")
	if err := os.WriteFile(bare, []byte("[tui]\nicons = \"nerd\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bareCfg, err := Load(bare)
	if err != nil {
		t.Fatalf("Load bare: %v", err)
	}
	if bareCfg.Onboarding != nil {
		t.Errorf("Onboarding = %+v, want nil for absent section", bareCfg.Onboarding)
	}
}

// TestOnboardingYAMLParse: the custom Config.UnmarshalYAML mirrors the
// section (a missed rawConfig field would silently drop it).
func TestOnboardingYAMLParse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "grove.yml")
	content := "onboarding:\n  completed: true\n  last_step: theme\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Onboarding == nil || !cfg.Onboarding.Completed || cfg.Onboarding.LastStep != "theme" {
		t.Errorf("Onboarding = %+v, want {Completed:true LastStep:theme}", cfg.Onboarding)
	}
}

// TestOnboardingMerge: a set Completed/LastStep in an overlay layer wins;
// a nil or zero-value overlay never un-completes the base.
func TestOnboardingMerge(t *testing.T) {
	// Overlay sets both fields over an empty base.
	base := &Config{}
	override := &Config{Onboarding: &OnboardingConfig{Completed: true, LastStep: "keys"}}
	merged := mergeConfigs(base, override)
	if merged.Onboarding == nil || !merged.Onboarding.Completed || merged.Onboarding.LastStep != "keys" {
		t.Errorf("merged.Onboarding = %+v, want {Completed:true LastStep:keys}", merged.Onboarding)
	}

	// A zero-value overlay section must not clear the base's state.
	base = &Config{Onboarding: &OnboardingConfig{Completed: true, LastStep: "theme"}}
	override = &Config{Onboarding: &OnboardingConfig{}}
	merged = mergeConfigs(base, override)
	if !merged.Onboarding.Completed || merged.Onboarding.LastStep != "theme" {
		t.Errorf("zero overlay clobbered base: %+v", merged.Onboarding)
	}

	// An overlay without the section leaves the base untouched.
	merged = mergeConfigs(base, &Config{})
	if merged.Onboarding == nil || !merged.Onboarding.Completed {
		t.Errorf("nil overlay dropped base section: %+v", merged.Onboarding)
	}

	// A later LastStep overrides an earlier one.
	base = &Config{Onboarding: &OnboardingConfig{LastStep: "welcome"}}
	override = &Config{Onboarding: &OnboardingConfig{LastStep: "done"}}
	merged = mergeConfigs(base, override)
	if merged.Onboarding.LastStep != "done" {
		t.Errorf("LastStep = %q, want done", merged.Onboarding.LastStep)
	}
}
