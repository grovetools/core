package config

import (
	"os"
	"path/filepath"
	"testing"
)

// Reconciliation is the diff between intent and disk: present, declared but
// missing, or a directory with no manifest. Bare roots are never reconciled —
// nothing can materialize one, so reporting it would point at no action.
func TestReconcileMachineEcosystems(t *testing.T) {
	root := t.TempDir()
	present := filepath.Join(root, "present")
	unmanifested := filepath.Join(root, "unmanifested")
	writeFileIn(t, filepath.Join(present, "grove.toml"), "name = \"present\"\n")
	if err := os.MkdirAll(unmanifested, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	dir := sandboxConfig(t)
	path := filepath.Join(dir, "machine.toml")
	writeFile(t, path, `[machine.ecosystems.present]
path = "`+present+`"
notebook = "nb"

[machine.ecosystems.unmanifested]
path = "`+unmanifested+`"

[machine.ecosystems.gone]
path = "`+filepath.Join(root, "gone")+`"
enabled = false

[machine.roots.somewhere]
path = "`+filepath.Join(root, "also-gone")+`"
`)

	cfg, err := LoadMachineConfigFrom(path)
	if err != nil {
		t.Fatalf("LoadMachineConfigFrom: %v", err)
	}
	states := ReconcileMachineEcosystems(cfg)
	if len(states) != 3 {
		t.Fatalf("reconciled %d entries, want 3 (roots must not be reconciled): %+v", len(states), states)
	}
	// Sorted by name: gone, present, unmanifested.
	if states[0].Name != "gone" || states[1].Name != "present" || states[2].Name != "unmanifested" {
		t.Fatalf("states are not sorted by name: %+v", states)
	}

	if states[1].State != MachineEcosystemPresent || states[1].Manifest == "" {
		t.Errorf("present = %+v, want present with a manifest", states[1])
	}
	if states[1].Notebook != "nb" {
		t.Errorf("notebook override lost: %+v", states[1])
	}
	if states[0].State != MachineEcosystemDeclaredMissing || !states[0].Missing() {
		t.Errorf("gone = %+v, want declared-missing", states[0])
	}
	if states[0].Enabled {
		t.Errorf("gone should report Enabled=false: %+v", states[0])
	}
	if states[2].State != MachineEcosystemUnmanifested || !states[2].Missing() {
		t.Errorf("unmanifested = %+v, want unmanifested", states[2])
	}
	if states[1].Missing() {
		t.Errorf("a present ecosystem must not report Missing()")
	}
}

func TestReconcileMachineEcosystemsNilSafe(t *testing.T) {
	if got := ReconcileMachineEcosystems(nil); got != nil {
		t.Fatalf("ReconcileMachineEcosystems(nil) = %+v, want nil", got)
	}
	if got := ReconcileMachineEcosystems(&MachineConfig{}); got != nil {
		t.Fatalf("ReconcileMachineEcosystems(empty) = %+v, want nil", got)
	}
}
