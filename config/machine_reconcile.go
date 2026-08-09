package config

import (
	"os"
	"path/filepath"
)

// Machine ecosystem reconciliation states.
const (
	// MachineEcosystemPresent: the subscription's path is a directory that
	// carries a grove manifest — the ecosystem is materialized here.
	MachineEcosystemPresent = "present"
	// MachineEcosystemDeclaredMissing: subscribed, but nothing is on disk.
	// This is precisely the materialization verb's input, and the state the
	// machine note records so other machines can see the gap.
	MachineEcosystemDeclaredMissing = "declared-missing"
	// MachineEcosystemUnmanifested: the directory exists but carries no grove
	// manifest — a half-materialized clone, or a path that was never an
	// ecosystem. Distinguished from missing because the repair is different:
	// nothing should clone over a non-empty directory.
	MachineEcosystemUnmanifested = "unmanifested"
)

// MachineEcosystemState is one subscription reconciled against the disk. It is
// intent (config) compared with observed state (the filesystem) — the diff §0
// of the design calls derived, and the only thing that makes "declared but
// missing" answerable without a registry.
type MachineEcosystemState struct {
	Name string `json:"name"`
	// Path is the subscription's path, expanded (~ and env vars resolved).
	Path string `json:"path"`
	// Notebook is the machine-side override, empty when the ecosystem's own
	// card decides.
	Notebook string `json:"notebook,omitempty"`
	// State is one of the MachineEcosystem* constants above.
	State string `json:"state"`
	// Manifest is the grove manifest found at Path, empty unless present.
	Manifest string `json:"manifest,omitempty"`
	// Enabled reports whether the subscription is active. A disabled entry is
	// still reconciled — the operator asked for it to exist, just not to be
	// scanned — but no surface should nag about it.
	Enabled bool `json:"enabled"`
	// Repos and Exclude retain the subscriber's member intent. Reconciliation
	// is ecosystem-level: omitted members are intentional and therefore never
	// turn an otherwise-present partial ecosystem into a missing state.
	Repos   []string `json:"repos,omitempty"`
	Exclude []string `json:"exclude,omitempty"`
}

// Missing reports whether this subscription needs materializing.
func (s MachineEcosystemState) Missing() bool {
	return s.State == MachineEcosystemDeclaredMissing || s.State == MachineEcosystemUnmanifested
}

// ReconcileMachineEcosystems compares every [machine.ecosystems.*]
// subscription with the disk, in deterministic (sorted) order.
//
// Bare roots are deliberately NOT reconciled: nothing can materialize
// ~/code/chickens, so reporting it as "declared but missing" would be noise
// pointing at no action.
func ReconcileMachineEcosystems(m *MachineConfig) []MachineEcosystemState {
	if m == nil || len(m.Machine.Ecosystems) == 0 {
		return nil
	}
	out := make([]MachineEcosystemState, 0, len(m.Machine.Ecosystems))
	for _, name := range sortedKeys(m.Machine.Ecosystems) {
		eco := m.Machine.Ecosystems[name]
		state := MachineEcosystemState{
			Name:     name,
			Path:     expandPath(eco.Path),
			Notebook: eco.Notebook,
			Enabled:  eco.Enabled == nil || *eco.Enabled,
			Repos:    append([]string(nil), eco.Repos...),
			Exclude:  append([]string(nil), eco.Exclude...),
		}
		if abs, err := filepath.Abs(state.Path); err == nil {
			state.Path = abs
		}
		info, err := os.Stat(state.Path)
		switch {
		case err != nil || !info.IsDir():
			state.State = MachineEcosystemDeclaredMissing
		default:
			if manifest := FindEcosystemManifest(state.Path); manifest != "" {
				state.State = MachineEcosystemPresent
				state.Manifest = manifest
			} else {
				state.State = MachineEcosystemUnmanifested
			}
		}
		out = append(out, state)
	}
	return out
}
