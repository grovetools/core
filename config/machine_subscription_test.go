package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestMachineEcosystemMemberFiltersRoundTrip(t *testing.T) {
	dir := sandboxConfig(t)
	path := filepath.Join(dir, "machine.toml")
	changed, err := WriteMachineSubscriptions(path, MachineSubscriptions{Ecosystems: map[string]MachineEcosystem{
		"grovetools": {Path: "/code/grovetools", Repos: []string{"core", "nav"}},
	}})
	if err != nil || !changed {
		t.Fatalf("WriteMachineSubscriptions: changed=%t err=%v", changed, err)
	}
	cfg, err := LoadMachineConfigFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	eco := cfg.Machine.Ecosystems["grovetools"]
	if !eco.IncludesRepo("core") || eco.IncludesRepo("grove") {
		t.Fatalf("include selection not preserved: %+v", eco)
	}
	body := readFile(t, path)
	if !strings.Contains(body, `repos = ["core", "nav"]`) {
		t.Fatalf("repos were not rendered deterministically:\n%s", body)
	}
}

func TestMachineEcosystemRejectsAmbiguousFilters(t *testing.T) {
	_, err := ParseMachineConfigContent("machine.toml", `[machine.ecosystems.grovetools]
path = "/code/grovetools"
repos = ["core"]
exclude = ["nav"]
`)
	if err == nil || !strings.Contains(err.Error(), "both repos and exclude") {
		t.Fatalf("error = %v, want mutual-exclusion error", err)
	}

	eco := MachineEcosystem{Exclude: []string{"daemon"}}
	if !eco.IncludesRepo("core") || eco.IncludesRepo("daemon") {
		t.Fatalf("exclude selection is wrong: %+v", eco)
	}
}
